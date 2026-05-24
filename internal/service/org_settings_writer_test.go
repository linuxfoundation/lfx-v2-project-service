// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"testing"
	"time"

	fgatypes "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/types"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/mock"
	svc "github.com/linuxfoundation/lfx-v2-member-service/internal/service"
	pkgerrors "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/etag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testOrgUID = "00000000-0000-0000-0000-000000000001"

func newOrgSettingsWriter(store *mock.MockB2BOrgSettings, orgReader *mock.MockB2BOrgReader, pub *mock.MockMemberPublisher) svc.OrgSettingsWriter {
	return svc.NewOrgSettingsWriter(
		svc.WithOrgSettingsReader(store),
		svc.WithOrgSettingsWriter(store),
		svc.WithOrgSettingsB2BOrgReader(orgReader),
		svc.WithOrgSettingsPublisher(pub),
	)
}

// ── Update ──────────────────────────────────────────────────────────────────

func TestOrgSettingsWriter_Update_NilExisting_CreatesNewSettings(t *testing.T) {
	store := mock.NewMockB2BOrgSettings() // empty — no existing settings
	pub := mock.NewMockMemberPublisher()
	writer := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), pub)

	writers := []model.B2BOrgUser{{Email: "alice@example.com", Username: "alice"}}
	in := svc.B2BOrgSettingsUpdate{OrgUID: testOrgUID, Writers: writers}

	result, err := writer.Update(context.Background(), in)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Writers, 1)
	assert.Equal(t, "alice@example.com", result.Writers[0].Email)
}

func TestOrgSettingsWriter_Update_NilWriters_KeepsExisting(t *testing.T) {
	store := mock.NewMockB2BOrgSettings()
	existing := &model.B2BOrgSettings{
		UID:     testOrgUID,
		Writers: []model.B2BOrgUser{{Email: "bob@example.com"}},
	}
	store.Seed(testOrgUID, existing, 1)

	writer := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())

	// nil Writers → keep bob
	in := svc.B2BOrgSettingsUpdate{OrgUID: testOrgUID, Writers: nil}
	result, err := writer.Update(context.Background(), in)

	require.NoError(t, err)
	require.Len(t, result.Writers, 1)
	assert.Equal(t, "bob@example.com", result.Writers[0].Email)
}

func TestOrgSettingsWriter_Update_EmptyWriters_ClearsAll(t *testing.T) {
	store := mock.NewMockB2BOrgSettings()
	existing := &model.B2BOrgSettings{
		UID:     testOrgUID,
		Writers: []model.B2BOrgUser{{Email: "bob@example.com"}},
	}
	store.Seed(testOrgUID, existing, 1)

	writer := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())

	// empty (non-nil) slice → clear all
	in := svc.B2BOrgSettingsUpdate{OrgUID: testOrgUID, Writers: []model.B2BOrgUser{}}
	result, err := writer.Update(context.Background(), in)

	require.NoError(t, err)
	assert.Empty(t, result.Writers, "explicit empty slice should clear writers")
}

func TestOrgSettingsWriter_Update_IfMatch_Matches_Succeeds(t *testing.T) {
	store := mock.NewMockB2BOrgSettings()
	existing := &model.B2BOrgSettings{UID: testOrgUID, CreatedAt: time.Now().UTC()}
	store.Seed(testOrgUID, existing, 1)

	etagVal, err := etag.LFXEtag(existing)
	require.NoError(t, err)

	writer := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())
	in := svc.B2BOrgSettingsUpdate{OrgUID: testOrgUID, IfMatch: etagVal}
	_, err = writer.Update(context.Background(), in)

	assert.NoError(t, err)
}

func TestOrgSettingsWriter_Update_IfMatch_NoExistingRecord_PreconditionFailed(t *testing.T) {
	store := mock.NewMockB2BOrgSettings() // empty — no existing settings

	writer := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())
	in := svc.B2BOrgSettingsUpdate{OrgUID: testOrgUID, IfMatch: "\"some-etag\""}
	_, err := writer.Update(context.Background(), in)

	require.Error(t, err)
	assert.True(t, pkgerrors.IsPreconditionFailed(err), "If-Match on non-existent record must return PreconditionFailed, got: %v", err)
}

func TestOrgSettingsWriter_Update_IfMatch_Mismatch_PreconditionFailed(t *testing.T) {
	store := mock.NewMockB2BOrgSettings()
	existing := &model.B2BOrgSettings{UID: testOrgUID}
	store.Seed(testOrgUID, existing, 1)

	writer := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())
	in := svc.B2BOrgSettingsUpdate{OrgUID: testOrgUID, IfMatch: "\"stale-etag\""}
	_, err := writer.Update(context.Background(), in)

	require.Error(t, err)
	assert.True(t, pkgerrors.IsPreconditionFailed(err), "expected PreconditionFailed, got: %v", err)
}

func TestOrgSettingsWriter_Update_OrgFetchFailure_FGASwallowed(t *testing.T) {
	store := mock.NewMockB2BOrgSettings() // no existing settings
	// MockB2BOrgReader always returns not-found — FGA publish will be swallowed
	writer := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())

	in := svc.B2BOrgSettingsUpdate{OrgUID: testOrgUID}
	_, err := writer.Update(context.Background(), in)

	assert.NoError(t, err, "org fetch failure for FGA must not propagate")
}

func TestOrgSettingsWriter_Update_PublishFailure_Swallowed(t *testing.T) {
	store := mock.NewMockB2BOrgSettings()
	// Seed an org so the FGA reader succeeds, but the publisher errors
	pub := mock.NewMockMemberPublisher()
	pub.SetAccessError(pkgerrors.NewUnexpected("nats down", nil))

	// Use a seeded org reader so org fetch succeeds and publish is actually attempted
	seededReader := &seedB2BOrgReader{org: &model.B2BOrg{UID: testOrgUID}}
	writer := svc.NewOrgSettingsWriter(
		svc.WithOrgSettingsReader(store),
		svc.WithOrgSettingsWriter(store),
		svc.WithOrgSettingsB2BOrgReader(seededReader),
		svc.WithOrgSettingsPublisher(pub),
	)

	in := svc.B2BOrgSettingsUpdate{OrgUID: testOrgUID}
	_, err := writer.Update(context.Background(), in)

	assert.NoError(t, err, "FGA publish failure must not propagate")
}

func TestOrgSettingsWriter_Update_ClearWriters_FGADoesNotExcludeWriter(t *testing.T) {
	store := mock.NewMockB2BOrgSettings()
	existing := &model.B2BOrgSettings{
		UID:     testOrgUID,
		Writers: []model.B2BOrgUser{{Email: "alice@example.com", Username: "alice", InviteStatus: model.InviteStatusAccepted}},
	}
	store.Seed(testOrgUID, existing, 1)

	pub := mock.NewMockMemberPublisher()
	seededReader := &seedB2BOrgReader{org: &model.B2BOrg{UID: testOrgUID}}
	writer := svc.NewOrgSettingsWriter(
		svc.WithOrgSettingsReader(store),
		svc.WithOrgSettingsWriter(store),
		svc.WithOrgSettingsB2BOrgReader(seededReader),
		svc.WithOrgSettingsPublisher(pub),
	)

	// explicit empty slice — intent: clear all writers
	in := svc.B2BOrgSettingsUpdate{OrgUID: testOrgUID, Writers: []model.B2BOrgUser{}}
	_, err := writer.Update(context.Background(), in)

	require.NoError(t, err)
	require.NotNil(t, pub.LastAccessData, "FGA message must be published")

	// The FGA message must NOT exclude "writer" — an explicit empty list must let
	// the full-sync run so it revokes alice's existing tuple.
	fgaMsg, ok := pub.LastAccessData.(fgatypes.GenericFGAMessage)
	require.True(t, ok, "expected GenericFGAMessage, got %T", pub.LastAccessData)
	data, ok := fgaMsg.Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.NotContains(t, data.ExcludeRelations, "writer",
		"explicit empty writers must not be excluded from FGA sync — revocation requires full-sync")
}

// ── Helpers ────────────────────────────────────────────────────────────────

// seedB2BOrgReader returns a fixed org for any UID.
type seedB2BOrgReader struct{ org *model.B2BOrg }

func (r *seedB2BOrgReader) GetB2BOrg(_ context.Context, _ string) (*model.B2BOrg, error) {
	return r.org, nil
}

func (r *seedB2BOrgReader) FetchChildUIDsByParentUID(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}
