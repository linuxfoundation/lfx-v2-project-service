// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"encoding/json"
	"log/slog"

	emailapi "github.com/linuxfoundation/lfx-v2-email-service/pkg/api"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service/email"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"
	"golang.org/x/sync/errgroup"
)

// HandleDocumentUploaded handles document.uploaded events and fans out notification emails
// to all LFID writers and auditors on the project (excluding the uploader).
//
// Only runs when EmailsEnabled is true; failures per recipient are logged and never propagated.
func (s *ProjectsService) HandleDocumentUploaded(ctx context.Context, msg domain.Message) error {
	if !s.Config.EmailsEnabled {
		slog.DebugContext(ctx, "document_subscriber: skipping notifications — EMAILS_ENABLED is false")
		return nil
	}

	var event events.DocumentUploadedMessage
	if err := json.Unmarshal(msg.Data(), &event); err != nil {
		slog.WarnContext(ctx, "document_subscriber: failed to unmarshal document uploaded event", constants.ErrKey, err)
		return nil
	}

	slog.DebugContext(ctx, "document_subscriber: received document.uploaded event",
		"project_uid", event.ProjectUID,
		"document_type", event.DocumentType,
		"document_name", event.DocumentName,
	)

	projectBase, err := s.ProjectRepository.GetProjectBase(ctx, event.ProjectUID)
	if err != nil {
		slog.WarnContext(ctx, "document_subscriber: failed to load project base", constants.ErrKey, err, "project_uid", event.ProjectUID)
		return nil
	}

	settings, err := s.ProjectRepository.GetProjectSettings(ctx, event.ProjectUID)
	if err != nil {
		slog.WarnContext(ctx, "document_subscriber: failed to load project settings", constants.ErrKey, err, "project_uid", event.ProjectUID)
		return nil
	}

	projectURL := buildProjectURL(s.Config.LFXSelfServeBaseURL, projectBase.Slug)
	uploaderName := s.resolveActorDisplayName(ctx, event.Actor)

	recipients := collectDocumentRecipients(settings, event.Actor.Username)
	if len(recipients) == 0 {
		slog.DebugContext(ctx, "document_subscriber: no eligible recipients — skipping", "project_uid", event.ProjectUID)
		return nil
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(5)

	for _, r := range recipients {
		g.Go(func() error {
			recipientName := r.Name
			if recipientName == "" {
				recipientName = r.Username
			}
			if recipientName == "" {
				recipientName = r.Email
			}

			subject, html, text, renderErr := email.RenderProjectDocumentUploaded(email.ProjectDocumentUploadedData{
				RecipientName: recipientName,
				ProjectName:   projectBase.Name,
				DocumentName:  event.DocumentName,
				DocumentType:  event.DocumentType,
				FileName:      event.FileName,
				URL:           event.URL,
				UploaderName:  uploaderName,
				ProjectURL:    projectURL,
			})
			if renderErr != nil {
				slog.WarnContext(gctx, "document_subscriber: failed to render document upload email",
					constants.ErrKey, renderErr, "project_uid", event.ProjectUID)
				return nil
			}

			sendCtx, cancel := context.WithTimeout(gctx, notificationTimeout)
			defer cancel()

			if sendErr := s.MessageBuilder.SendEmailRequest(sendCtx, emailapi.SendEmailRequest{
				To:      r.Email,
				Subject: subject,
				HTML:    html,
				Text:    text,
			}); sendErr != nil {
				slog.WarnContext(gctx, "document_subscriber: failed to send document upload notification",
					constants.ErrKey, sendErr, "project_uid", event.ProjectUID)
			} else {
				slog.DebugContext(gctx, "document_subscriber: sent document upload notification", "project_uid", event.ProjectUID)
			}
			return nil
		})
	}

	_ = g.Wait()
	return nil
}

// collectDocumentRecipients returns the deduplicated set of writers and auditors who should
// receive a document upload notification. It excludes:
//   - users without a Username (no LFID)
//   - users without an Email address
//   - the uploader (matched by Username)
func collectDocumentRecipients(settings *models.ProjectSettings, uploaderUsername string) []models.UserInfo {
	seen := make(map[string]struct{})
	var out []models.UserInfo

	add := func(u models.UserInfo) {
		if u.Username == "" || u.Email == "" {
			return
		}
		if uploaderUsername != "" && u.Username == uploaderUsername {
			return
		}
		if _, exists := seen[u.Username]; exists {
			return
		}
		seen[u.Username] = struct{}{}
		out = append(out, u)
	}

	for _, u := range settings.Writers {
		add(u)
	}
	for _, u := range settings.Auditors {
		add(u)
	}
	return out
}
