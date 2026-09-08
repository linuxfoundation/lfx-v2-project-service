// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	nats "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	inviteapi "github.com/linuxfoundation/lfx-v2-invite-service/pkg/api"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	internalnats "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// setupNATS connects to NATS, opens the KV repository, and builds all
// infrastructure dependencies required by ProjectsService. The returned
// ServiceDeps is fully populated and ready to pass to NewProjectsService.
func setupNATS(ctx context.Context, env environment, gracefulCloseWG *sync.WaitGroup, done chan os.Signal) (*nats.Conn, service.ServiceDeps, error) {
	gracefulCloseWG.Add(1)
	slog.With("nats_url", env.NatsURL).Info("attempting to connect to NATS")
	natsConn, err := nats.Connect(
		env.NatsURL,
		nats.DrainTimeout(gracefulShutdownSeconds*time.Second),
		nats.ConnectHandler(func(_ *nats.Conn) {
			slog.With("nats_url", env.NatsURL).Info("NATS connection established")
		}),
		nats.ErrorHandler(func(_ *nats.Conn, s *nats.Subscription, err error) {
			if s != nil {
				slog.With(constants.ErrKey, err, "subject", s.Subject, "queue", s.Queue).Error("async NATS error")
			} else {
				slog.With(constants.ErrKey, err).Error("async NATS error outside subscription")
			}
		}),
		nats.ClosedHandler(func(_ *nats.Conn) {
			if ctx.Err() != nil {
				// If our parent background context has already been canceled, this is
				// a graceful shutdown. Decrement the wait group but do not exit, to
				// allow other graceful shutdown steps to complete.
				slog.With("nats_url", env.NatsURL).Info("NATS connection closed gracefully")
				gracefulCloseWG.Done()
				return
			}
			// Otherwise, this handler means that max reconnect attempts have been
			// exhausted.
			slog.With("nats_url", env.NatsURL).Error("NATS max-reconnects exhausted; connection closed")
			// Send a synthetic interrupt and give any graceful-shutdown tasks 5
			// seconds to clean up.
			done <- os.Interrupt
			time.Sleep(5 * time.Second)
			// Exit with an error instead of decrementing the wait group.
			os.Exit(1)
		}),
	)
	if err != nil {
		slog.With("nats_url", env.NatsURL, constants.ErrKey, err).Error("error creating NATS client")
		return nil, service.ServiceDeps{}, err
	}

	// Open the key-value stores that back all four repository interfaces.
	repo, err := getKeyValueStores(ctx, natsConn)
	if err != nil {
		return natsConn, service.ServiceDeps{}, err
	}

	msgBuilder := &internalnats.MessageBuilder{NatsConn: natsConn}
	userReader := &internalnats.UserReaderNATS{NatsConn: natsConn}
	resolver := service.NewUserResolver(userReader)
	dispatcher := service.NewNotificationDispatcher(msgBuilder, resolver, env.EmailsEnabled, env.InvitesEnabled)

	return natsConn, service.ServiceDeps{
		ProjectRepository:  repo,
		DocumentRepository: repo,
		LinkRepository:     repo,
		FolderRepository:   repo,
		MessageBuilder:     msgBuilder,
		UserReader:         userReader,
		Resolver:           resolver,
		Dispatcher:         dispatcher,
	}, nil
}

// getKeyValueStores creates a JetStream client and opens the project repository stores.
func getKeyValueStores(ctx context.Context, natsConn *nats.Conn) (*internalnats.NatsRepository, error) {
	js, err := jetstream.New(natsConn)
	if err != nil {
		slog.ErrorContext(ctx, "error creating NATS JetStream client", "nats_url", natsConn.ConnectedUrl(), constants.ErrKey, err)
		return nil, err
	}

	repo, err := internalnats.OpenRepository(ctx, js)
	if err != nil {
		slog.ErrorContext(ctx, "error opening NATS repository stores", "nats_url", natsConn.ConnectedUrl(), constants.ErrKey, err)
		return nil, err
	}
	return repo, nil
}

// createNatsSubcriptions creates the NATS subscriptions for the project service.
func createNatsSubcriptions(ctx context.Context, svc *ProjectsAPI, natsConn *nats.Conn) error {
	slog.InfoContext(ctx, "subscribing to NATS subjects", "nats_url", natsConn.ConnectedUrl(), "servers", natsConn.Servers())
	queueName := constants.ProjectsAPIQueue

	for _, subject := range []string{
		// Get project name subscription
		constants.ProjectGetNameSubject,
		// Get project slug subscription
		constants.ProjectGetSlugSubject,
		// Get project logo subscription
		constants.ProjectGetLogoSubject,
		// Get project slug to UID subscription
		constants.ProjectSlugToUIDSubject,
		// Get project parent UID subscription
		constants.ProjectGetParentUIDSubject,
		// Get project writers subscription
		constants.ProjectGetWritersSubject,
	} {
		slog.With("subject", subject, "queue", queueName).Debug("subscribing to NATS subject")
		_, err := natsConn.QueueSubscribe(subject, queueName, func(msg *nats.Msg) {
			msgCtx, end := internalnats.ExtractMsgContext(ctx, msg, subject)
			defer end()
			natsMsg := &internalnats.NatsMsg{Msg: msg}
			svc.service.HandleMessage(msgCtx, natsMsg)
		})
		if err != nil {
			slog.ErrorContext(ctx, "error creating NATS queue subscription", constants.ErrKey, err)
			return err
		}
	}

	type eventHandler struct {
		subject string
		handle  func(ctx context.Context, msg domain.Message) error
	}
	for _, eh := range []eventHandler{
		{constants.ProjectSettingsUpdatedSubject, svc.service.HandleProjectSettingsUpdated},
		{inviteapi.InviteServiceAcceptedSubject, svc.service.HandleInviteAccepted},
		{constants.V1SyncHelperUserDeletedSubject, svc.service.HandleUserDeleted},
		{constants.ProjectDocumentCreatedSubject, svc.service.HandleProjectDocumentCreated},
		{constants.ProjectLinkCreatedSubject, svc.service.HandleProjectLinkCreated},
	} {
		// Inbound lifecycle events use core NATS queue subscriptions, matching
		// lfx-v2-committee-service (LFXV2-2645). v1-sync-helper publishes with core NATS
		// Publish; missed messages during disconnect are not replayed automatically.
		slog.With("subject", eh.subject, "queue", queueName).Debug("subscribing to NATS subject")
		_, err := natsConn.QueueSubscribe(eh.subject, queueName, func(msg *nats.Msg) {
			msgCtx, end := internalnats.ExtractMsgContext(ctx, msg, eh.subject)
			defer end()
			natsMsg := &internalnats.NatsMsg{Msg: msg}
			if handlerErr := eh.handle(msgCtx, natsMsg); handlerErr != nil {
				slog.WarnContext(msgCtx, "event handler failed", constants.ErrKey, handlerErr, "subject", eh.subject)
			}
		})
		if err != nil {
			slog.ErrorContext(ctx, "error creating NATS queue subscription", constants.ErrKey, err)
			return err
		}
	}

	return nil
}

func gracefulShutdown(httpServer *http.Server, natsConn *nats.Conn, gracefulCloseWG *sync.WaitGroup, cancel context.CancelFunc) {
	// Cancel the background context.
	cancel()

	go func() {
		// Run the HTTP shutdown in a goroutine so the NATS draining can also start.
		ctx, cancel := context.WithTimeout(context.Background(), gracefulShutdownSeconds*time.Second)
		defer cancel()

		slog.With("addr", httpServer.Addr).Info("shutting down http server")
		if err := httpServer.Shutdown(ctx); err != nil {
			slog.With(constants.ErrKey, err).Error("http shutdown error")
		}
		// Decrement the wait group.
		gracefulCloseWG.Done()
	}()

	// Drain the NATS connection, which will drain all subscriptions, then close the
	// connection when complete.
	if !natsConn.IsClosed() && !natsConn.IsDraining() {
		slog.Info("draining NATS connections")
		if err := natsConn.Drain(); err != nil {
			slog.With(constants.ErrKey, err).Error("error draining NATS connection")
			// Skip waiting or checking error channel.
			return
		}
	}

	// Wait for the HTTP graceful shutdown and for the NATS connection to be
	// closed (see nats.Connect options for the timeout and the handler that
	// decrements the wait group).
	gracefulCloseWG.Wait()
}
