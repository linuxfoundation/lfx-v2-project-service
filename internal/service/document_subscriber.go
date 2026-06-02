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

// projectContentItem is a unified representation of a file document or link for notification purposes.
type projectContentItem struct {
	projectUID string
	folderUID  string // empty when not in a folder
	itemType   string // "file" | "link"
	itemName   string
	fileName   string // non-empty for files
	url        string // non-empty for links
	createdBy  string // LFID of the uploader/creator
}

// HandleProjectDocumentCreated handles project_document.created events and notifies all
// LFID writers and auditors of the project when a new file document is uploaded.
// Best-effort: send errors are logged but not returned.
func (s *ProjectsService) HandleProjectDocumentCreated(ctx context.Context, rawMsg domain.Message) error {
	if !s.Config.EmailsEnabled {
		slog.DebugContext(ctx, "document_subscriber: skipping notifications — EMAILS_ENABLED is false")
		return nil
	}

	var event events.ProjectDocumentCreatedMessage
	if err := json.Unmarshal(rawMsg.Data(), &event); err != nil {
		slog.WarnContext(ctx, "document_subscriber: failed to unmarshal project_document.created event", constants.ErrKey, err)
		return nil
	}

	s.handleProjectContentCreated(ctx, projectContentItem{
		projectUID: event.ProjectUID,
		folderUID:  event.FolderUID,
		itemType:   "file",
		itemName:   event.Name,
		fileName:   event.FileName,
		createdBy:  event.CreatedBy,
	})
	return nil
}

// HandleProjectLinkCreated handles project_link.created events and notifies all
// LFID writers and auditors of the project when a new link is added.
// Best-effort: send errors are logged but not returned.
func (s *ProjectsService) HandleProjectLinkCreated(ctx context.Context, rawMsg domain.Message) error {
	if !s.Config.EmailsEnabled {
		slog.DebugContext(ctx, "document_subscriber: skipping notifications — EMAILS_ENABLED is false")
		return nil
	}

	var event events.ProjectLinkCreatedMessage
	if err := json.Unmarshal(rawMsg.Data(), &event); err != nil {
		slog.WarnContext(ctx, "document_subscriber: failed to unmarshal project_link.created event", constants.ErrKey, err)
		return nil
	}

	s.handleProjectContentCreated(ctx, projectContentItem{
		projectUID: event.ProjectUID,
		folderUID:  event.FolderUID,
		itemType:   "link",
		itemName:   event.Name,
		url:        event.URL,
		createdBy:  event.CreatedBy,
	})
	return nil
}

// handleProjectContentCreated fans out notification emails to all LFID writers and auditors
// of the project. Individual send failures are logged but never abort the batch.
func (s *ProjectsService) handleProjectContentCreated(ctx context.Context, item projectContentItem) {
	slog.DebugContext(ctx, "document_subscriber: handling content created event",
		"project_uid", item.projectUID,
		"item_type", item.itemType,
		"item_name", item.itemName,
	)

	projectBase, err := s.ProjectRepository.GetProjectBase(ctx, item.projectUID)
	if err != nil {
		slog.WarnContext(ctx, "document_subscriber: failed to load project base", constants.ErrKey, err, "project_uid", item.projectUID)
		return
	}

	settings, err := s.ProjectRepository.GetProjectSettings(ctx, item.projectUID)
	if err != nil {
		slog.WarnContext(ctx, "document_subscriber: failed to load project settings", constants.ErrKey, err, "project_uid", item.projectUID)
		return
	}

	folderName := ""
	if item.folderUID != "" {
		if folder, _, err := s.FolderRepository.GetFolder(ctx, item.projectUID, item.folderUID); err == nil {
			folderName = folder.Name
		} else {
			slog.WarnContext(ctx, "document_subscriber: failed to load folder name — omitting from email",
				constants.ErrKey, err, "project_uid", item.projectUID, "folder_uid", item.folderUID)
		}
	}

	recipients := collectDocumentRecipients(settings, item.createdBy)
	if len(recipients) == 0 {
		slog.DebugContext(ctx, "document_subscriber: no eligible recipients — skipping", "project_uid", item.projectUID)
		return
	}

	projectURL := buildProjectURL(s.Config.LFXSelfServeBaseURL, projectBase.Slug)
	uploaderName := s.resolveActorDisplayName(ctx, events.Actor{Username: item.createdBy})

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(5)

	for _, r := range recipients {
		g.Go(func() error {
			if !s.isRecipientDomainAllowed(r.Email) {
				slog.DebugContext(gctx, "document_subscriber: skipping email — recipient domain not in EMAIL_ALLOWED_DOMAINS",
					"project_uid", item.projectUID)
				return nil
			}

			recipientName := r.Name
			if recipientName == "" {
				recipientName = r.Username
			}
			if recipientName == "" {
				recipientName = r.Email
			}

			subj, html, text, renderErr := email.RenderProjectDocumentUploaded(email.ProjectDocumentUploadedData{
				RecipientName: recipientName,
				ProjectName:   projectBase.Name,
				DocumentName:  item.itemName,
				DocumentType:  item.itemType,
				FileName:      item.fileName,
				URL:           item.url,
				FolderName:    folderName,
				UploaderName:  uploaderName,
				ProjectURL:    projectURL,
			})
			if renderErr != nil {
				slog.WarnContext(gctx, "document_subscriber: failed to render content notification email",
					constants.ErrKey, renderErr, "project_uid", item.projectUID)
				return nil
			}

			sendCtx, cancel := context.WithTimeout(gctx, notificationTimeout)
			defer cancel()

			if sendErr := s.MessageBuilder.SendEmailRequest(sendCtx, emailapi.SendEmailRequest{
				To:      r.Email,
				Subject: subj,
				HTML:    html,
				Text:    text,
			}); sendErr != nil {
				slog.WarnContext(gctx, "document_subscriber: failed to send content notification email",
					constants.ErrKey, sendErr, "project_uid", item.projectUID)
			} else {
				slog.DebugContext(gctx, "document_subscriber: sent content notification email",
					"project_uid", item.projectUID, "item_type", item.itemType)
			}
			return nil
		})
	}

	_ = g.Wait()
}

// collectDocumentRecipients returns the deduplicated set of writers and auditors who should
// receive a document/link notification. It excludes users without a Username (no LFID) or
// without an Email address.
func collectDocumentRecipients(settings *models.ProjectSettings, uploaderUsername string) []models.UserInfo {
	if settings == nil {
		return nil
	}
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
