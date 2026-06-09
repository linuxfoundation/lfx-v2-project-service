// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"log/slog"
	"strings"

	fgaconstants "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
	pkgerrors "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/etag"
)

// Type aliases expose port interfaces under KC-specific names for use in tests.
type (
	MemberStorageReader = port.MemberReader
	PMReader            = port.ProjectMembershipReader
	PublisherForKC      = port.MemberPublisher
	UserReaderForKC     = port.UserReader
)

// KeyContactCreateInput carries the validated, normalized fields for creating a key contact.
type KeyContactCreateInput struct {
	MembershipUID  string
	FirstName      string
	LastName       string
	Email          string
	Title          *string
	Role           string
	Status         *string
	BoardMember    *bool
	PrimaryContact *bool
}

// KeyContactUpdateInput carries the validated, normalized fields for updating a key contact.
// Nil pointer = leave existing unchanged.
type KeyContactUpdateInput struct {
	MembershipUID  string
	UID            string
	FirstName      *string
	LastName       *string
	Email          *string
	Title          *string
	Role           *string
	Status         *string
	BoardMember    *bool
	PrimaryContact *bool
	IfMatch        string // ETag from request; "" = unconditional update
}

// KeyContactDeleteInput carries the parameters for deleting a key contact.
type KeyContactDeleteInput struct {
	MembershipUID string
	UID           string
	IfMatch       string
}

// KeyContactWriter orchestrates Create/Update/Delete for key contacts.
type KeyContactWriter interface {
	Create(ctx context.Context, in KeyContactCreateInput) (*model.KeyContact, error)
	Update(ctx context.Context, in KeyContactUpdateInput) (*model.KeyContact, error)
	Delete(ctx context.Context, in KeyContactDeleteInput) error
}

type keyContactWriterOrchestrator struct {
	storage                 port.MemberReader
	keyContactWriter        port.KeyContactWriter
	projectMembershipReader port.ProjectMembershipReader
	memberPublisher         port.MemberPublisher
	userReader              port.UserReader
}

// KeyContactWriterOption configures a keyContactWriterOrchestrator.
type KeyContactWriterOption func(*keyContactWriterOrchestrator)

func WithKCStorage(r port.MemberReader) KeyContactWriterOption {
	return func(o *keyContactWriterOrchestrator) { o.storage = r }
}

func WithKCWriter(w port.KeyContactWriter) KeyContactWriterOption {
	return func(o *keyContactWriterOrchestrator) { o.keyContactWriter = w }
}

func WithKCProjectMembershipReader(r port.ProjectMembershipReader) KeyContactWriterOption {
	return func(o *keyContactWriterOrchestrator) { o.projectMembershipReader = r }
}

func WithKCPublisher(p port.MemberPublisher) KeyContactWriterOption {
	return func(o *keyContactWriterOrchestrator) { o.memberPublisher = p }
}

func WithKCUserReader(r port.UserReader) KeyContactWriterOption {
	return func(o *keyContactWriterOrchestrator) { o.userReader = r }
}

// NewKeyContactWriter constructs a KeyContactWriter.
func NewKeyContactWriter(opts ...KeyContactWriterOption) KeyContactWriter {
	o := &keyContactWriterOrchestrator{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Create creates a new key contact. Self-heals idempotent re-creates by returning
// an existing record when the same role+email is already active for the membership.
func (o *keyContactWriterOrchestrator) Create(ctx context.Context, in KeyContactCreateInput) (*model.KeyContact, error) {
	existing, err := o.normalizeAndValidateCreate(ctx, &in)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	pm, _, err := o.projectMembershipReader.AssembleProjectMembership(ctx, in.MembershipUID)
	if err != nil {
		return nil, err
	}

	input := model.KeyContactInput{
		Email:          &in.Email,
		FirstName:      in.FirstName,
		LastName:       in.LastName,
		Title:          derefPtrStr(in.Title),
		MembershipUID:  in.MembershipUID,
		ProjectUID:     pm.ProjectUID,
		AccountSFID:    pm.B2BOrgUID,
		Role:           &in.Role,
		Status:         in.Status,
		BoardMember:    in.BoardMember,
		PrimaryContact: in.PrimaryContact,
	}

	kc, err := o.keyContactWriter.CreateKeyContact(ctx, input)
	if err != nil {
		return nil, err
	}

	// PM FGA update_access first so the parent tuple exists before the key_contact put.
	pmMsg := BuildProjectMembershipFGAMessage(pm)
	if pubErr := o.memberPublisher.Access(ctx, constants.FGASyncUpdateAccessSubject, pmMsg, false); pubErr != nil {
		slog.WarnContext(ctx, "project membership FGA publish failed on key contact create",
			"membership_uid", pm.UID, "error", pubErr, "publish_failed_for_backfill_repair", true)
	}

	// Resolve username, publish indexer, then FGA put.
	username := o.resolveUsernameForContact(ctx, "", kc.Email)
	kc.Username = username
	PublishKeyContactIndexer(ctx, o.memberPublisher, kc, indexerConstants.ActionCreated)
	o.publishFGAPut(ctx, kc.MembershipUID, username)

	return kc, nil
}

// Update updates a key contact. Returns the current record unchanged on true no-op
// (no input fields set). Paired FGA publish: put new sub before remove old sub on email change.
func (o *keyContactWriterOrchestrator) Update(ctx context.Context, in KeyContactUpdateInput) (*model.KeyContact, error) {
	current, err := o.storage.GetKeyContact(ctx, in.UID)
	if err != nil {
		return nil, err
	}

	if in.IfMatch != "" {
		currentETag, etagErr := etag.LFXEtag(current)
		if etagErr != nil {
			return nil, pkgerrors.NewUnexpected("failed to compute etag for key contact", etagErr)
		}
		if currentETag != in.IfMatch {
			return nil, pkgerrors.NewPreconditionFailed("key contact has been modified since last read — refresh and retry")
		}
	}

	if !hasAnyKCChange(in) {
		return current, nil
	}

	if err := o.normalizeAndValidateUpdate(ctx, current, &in); err != nil {
		return nil, err
	}

	emailChanging := in.Email != nil && !strings.EqualFold(*in.Email, current.Email)

	input := model.KeyContactInput{
		Email:          in.Email,
		FirstName:      derefOrStr(in.FirstName, current.FirstName),
		LastName:       derefOrStr(in.LastName, current.LastName),
		Title:          derefPtrStr(in.Title),
		Role:           in.Role,
		Status:         in.Status,
		BoardMember:    in.BoardMember,
		PrimaryContact: in.PrimaryContact,
		MembershipUID:  in.MembershipUID,
		AccountSFID:    current.B2BOrgUID,
	}
	if in.IfMatch != "" {
		input.IfUnmodifiedSince = current.UpdatedAt.UTC().Format(constants.HTTPDateFormat)
	}

	newKC, err := o.keyContactWriter.UpdateKeyContact(ctx, in.UID, input)
	if err != nil {
		return nil, err
	}

	if emailChanging {
		// Paired FGA: put new username first (avoid no-access window), then remove old.
		newUsername := o.resolveUsernameForContact(ctx, "", newKC.Email)
		newKC.Username = newUsername
		o.publishFGAPut(ctx, newKC.MembershipUID, newUsername)
		oldUsername := o.resolveUsernameForContact(ctx, current.Username, current.Email)
		if oldUsername != newUsername {
			if pubErr := o.publishFGARemove(ctx, newKC.MembershipUID, oldUsername); pubErr != nil {
				// Log at error severity (dangling permission), but do not propagate — the
				// SF update already succeeded and returning an error would mislead callers.
				slog.ErrorContext(ctx, "key contact FGA remove failed on email change — dangling permission",
					"uid", in.UID, "error", pubErr)
			}
		}
	} else {
		username := o.resolveUsernameForContact(ctx, current.Username, newKC.Email)
		newKC.Username = username
		o.publishFGAPut(ctx, newKC.MembershipUID, username)
	}
	PublishKeyContactIndexer(ctx, o.memberPublisher, newKC, indexerConstants.ActionUpdated)

	return newKC, nil
}

// Delete deletes a key contact. Indexer delete is swallowed; FGA remove is propagated.
func (o *keyContactWriterOrchestrator) Delete(ctx context.Context, in KeyContactDeleteInput) error {
	kc, err := o.storage.GetKeyContact(ctx, in.UID)
	if err != nil {
		return err
	}

	if in.IfMatch != "" {
		currentETag, etagErr := etag.LFXEtag(kc)
		if etagErr != nil {
			return pkgerrors.NewUnexpected("failed to compute etag for key contact", etagErr)
		}
		if currentETag != in.IfMatch {
			return pkgerrors.NewPreconditionFailed("key contact has been modified since last read — refresh and retry")
		}
	}

	if err := o.keyContactWriter.DeleteKeyContact(ctx, in.UID, kc.MembershipUID); err != nil {
		return err
	}

	// Indexer delete: swallow (reindexable via /admin/reindex).
	PublishKeyContactIndexer(ctx, o.memberPublisher, kc, indexerConstants.ActionDeleted)

	// FGA remove: propagate — dangling permissions are not auto-repairable.
	username := o.resolveUsernameForContact(ctx, kc.Username, kc.Email)
	if pubErr := o.publishFGARemove(ctx, kc.MembershipUID, username); pubErr != nil {
		slog.ErrorContext(ctx, "key contact FGA remove failed on delete — dangling permission",
			"uid", in.UID, "error", pubErr)
		return pkgerrors.NewUnexpected("failed to revoke FGA access for deleted key contact", pubErr)
	}

	return nil
}

const legacyAuth0UsernamePrefix = "auth0|"

func (o *keyContactWriterOrchestrator) resolveUsernameForContact(ctx context.Context, currentUsername, email string) string {
	if currentUsername != "" && !strings.HasPrefix(currentUsername, legacyAuth0UsernamePrefix) {
		return currentUsername
	}
	if email != "" {
		username, err := o.userReader.UsernameByEmail(ctx, email)
		if err != nil {
			slog.WarnContext(ctx, "failed to resolve LFID username for key contact FGA",
				"email", email, "error", err)
		} else if username != "" {
			return username
		}
	}
	// Fallback when lookup is unavailable: strip the legacy auth0| prefix from safe-slug identifiers.
	if strings.HasPrefix(currentUsername, legacyAuth0UsernamePrefix) {
		return strings.TrimPrefix(currentUsername, legacyAuth0UsernamePrefix)
	}
	return ""
}

func (o *keyContactWriterOrchestrator) publishFGAPut(ctx context.Context, membershipUID, username string) {
	if username == "" {
		return
	}
	msg := BuildKeyContactFGAPutMessage(membershipUID, username)
	if pubErr := o.memberPublisher.Access(ctx, fgaconstants.GenericMemberPutSubject, msg, false); pubErr != nil {
		slog.WarnContext(ctx, "key contact FGA put publish failed",
			"membership_uid", membershipUID, "error", pubErr, "publish_failed_for_backfill_repair", true)
	}
}

func (o *keyContactWriterOrchestrator) publishFGARemove(ctx context.Context, membershipUID, username string) error {
	if username == "" {
		return nil
	}
	msg := BuildKeyContactFGARemoveMessage(membershipUID, username)
	return o.memberPublisher.Access(ctx, fgaconstants.GenericMemberRemoveSubject, msg, true)
}

func hasAnyKCChange(in KeyContactUpdateInput) bool {
	return in.Email != nil || in.Role != nil || in.Status != nil ||
		in.BoardMember != nil || in.PrimaryContact != nil ||
		in.Title != nil || in.FirstName != nil || in.LastName != nil
}

func derefPtrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefOrStr(s *string, fallback string) string {
	if s == nil {
		return fallback
	}
	return *s
}
