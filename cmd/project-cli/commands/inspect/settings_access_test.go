// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package inspect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/internal/fgaread"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
)

type fakeAccessRepo struct {
	bases       []*models.ProjectBase
	settings    []*models.ProjectSettings
	failProject bool
	failSetting bool
}

func (f fakeAccessRepo) ListAllProjectsBase(context.Context) ([]*models.ProjectBase, error) {
	if f.failProject {
		return nil, errors.New("boom")
	}
	return f.bases, nil
}

func (f fakeAccessRepo) ListAllProjectsSettings(context.Context) ([]*models.ProjectSettings, error) {
	if f.failSetting {
		return nil, errors.New("boom")
	}
	return f.settings, nil
}

// fakeTupleReader serves the tuples of each project and applies the relation filter.
type fakeTupleReader struct {
	tuples map[string][]fgaread.Tuple
	errFor string
}

func (f fakeTupleReader) ReadProjectTuples(_ context.Context, uid string, keep func(string) bool) ([]fgaread.Tuple, error) {
	if uid == f.errFor {
		return nil, errors.New("OpenFGA read returned HTTP 500")
	}
	var kept []fgaread.Tuple
	for _, tuple := range f.tuples[uid] {
		if keep(tuple.Relation) {
			kept = append(kept, tuple)
		}
	}
	return kept, nil
}

func userInfo(username string) models.UserInfo { return models.UserInfo{Username: username} }

func TestRunSettingsAccess(t *testing.T) {
	base := &models.ProjectBase{UID: uid1, Slug: "alpha", ParentUID: uid2}
	settings := &models.ProjectSettings{
		UID:     uid1,
		Writers: []models.UserInfo{userInfo("alice"), userInfo("bob")},
		// An email-only user has no username, so the builder writes no tuple for it.
		Auditors: []models.UserInfo{userInfo("carol"), {Email: "invited@example.com"}},
	}
	inSync := []fgaread.Tuple{
		{User: "user:alice", Relation: "writer"},
		{User: "user:bob", Relation: "writer"},
		{User: "user:carol", Relation: "auditor"},
		{User: "project:" + uid2, Relation: "parent"},
		// Global grants are not this command's concern.
		{User: "team:formation#member", Relation: "global_owner"},
	}

	tests := []struct {
		name       string
		repo       fakeAccessRepo
		reader     fakeTupleReader
		wantErr    string
		wantCounts map[string]int
		wantDetail map[string]string
	}{
		{
			name:   "tuples that match the settings are clean",
			repo:   fakeAccessRepo{bases: []*models.ProjectBase{base}, settings: []*models.ProjectSettings{settings}},
			reader: fakeTupleReader{tuples: map[string][]fgaread.Tuple{uid1: inSync}},
		},
		{
			name: "a writer that was never synced and a stale auditor",
			repo: fakeAccessRepo{bases: []*models.ProjectBase{base}, settings: []*models.ProjectSettings{settings}},
			reader: fakeTupleReader{tuples: map[string][]fgaread.Tuple{uid1: {
				{User: "user:alice", Relation: "writer"},
				{User: "user:carol", Relation: "auditor"},
				{User: "user:mallory", Relation: "auditor"},
				{User: "project:" + uid2, Relation: "parent"},
			}}},
			wantErr:    "found 2 problem(s)",
			wantCounts: map[string]int{kindAccessMissing: 1, kindAccessUnexpected: 1},
			wantDetail: map[string]string{kindAccessMissing: "writer=1", kindAccessUnexpected: "auditor=1"},
		},
		{
			name:       "a missing parent tuple",
			repo:       fakeAccessRepo{bases: []*models.ProjectBase{base}, settings: []*models.ProjectSettings{settings}},
			reader:     fakeTupleReader{tuples: map[string][]fgaread.Tuple{uid1: inSync[:3]}},
			wantErr:    "found 1 problem(s)",
			wantCounts: map[string]int{kindAccessMissing: 1},
			wantDetail: map[string]string{kindAccessMissing: "parent=1"},
		},
		{
			name: "a conditioned tuple",
			repo: fakeAccessRepo{bases: []*models.ProjectBase{base}, settings: []*models.ProjectSettings{settings}},
			reader: fakeTupleReader{tuples: map[string][]fgaread.Tuple{uid1: {
				{User: "user:alice", Relation: "writer", Conditioned: true},
				{User: "user:bob", Relation: "writer"},
				{User: "user:carol", Relation: "auditor"},
				{User: "project:" + uid2, Relation: "parent"},
			}}},
			wantErr:    "found 1 problem(s)",
			wantCounts: map[string]int{kindAccessConditioned: 1},
		},
		{
			name:       "a project with no settings record",
			repo:       fakeAccessRepo{bases: []*models.ProjectBase{base}},
			reader:     fakeTupleReader{},
			wantErr:    "found 1 problem(s)",
			wantCounts: map[string]int{kindMissingSettings: 1},
		},
		{
			name:       "a read failure is a finding, not an abort",
			repo:       fakeAccessRepo{bases: []*models.ProjectBase{base}, settings: []*models.ProjectSettings{settings}},
			reader:     fakeTupleReader{errFor: uid1},
			wantErr:    "found 1 problem(s)",
			wantCounts: map[string]int{kindAccessReadError: 1},
		},
		{name: "project listing failure", repo: fakeAccessRepo{failProject: true}, wantErr: "list project bases"},
		{name: "settings listing failure", repo: fakeAccessRepo{failSetting: true}, wantErr: "list project settings"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := runSettingsAccess(context.Background(), tt.repo, tt.reader, 2, &out)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			if out.Len() == 0 {
				return
			}
			var report settingsAccessReport
			require.NoError(t, json.Unmarshal(out.Bytes(), &report))
			if tt.wantCounts == nil {
				assert.Empty(t, report.Findings)
				return
			}
			assert.Equal(t, tt.wantCounts, report.Counts)
			for _, f := range report.Findings {
				if want, ok := tt.wantDetail[f.Kind]; ok {
					assert.Equal(t, want, f.Detail)
				}
				assert.NotContains(t, f.Detail, "mallory", "findings never name a user")
			}
		})
	}
}

func TestBuildExpectedAccess(t *testing.T) {
	expected := buildExpectedAccess(
		&models.ProjectBase{UID: uid1, ParentUID: uid2},
		&models.ProjectSettings{UID: uid1, Writers: []models.UserInfo{userInfo("alice")}, ExecutiveDirector: &models.UserInfo{Username: "dana"}})

	assert.Equal(t, map[string]struct{}{
		accessKey("writer", "user:alice"):            {},
		accessKey("executive_director", "user:dana"): {},
		accessKey("parent", "project:"+uid2):         {},
	}, expected.tuples)
	for _, relation := range []string{"writer", "auditor", "meeting_coordinator", "executive_director", "mentorship_program_admin", "parent"} {
		assert.Contains(t, expected.relations, relation)
	}
	assert.NotContains(t, expected.relations, "global_owner")
}

func TestDescribeCounts(t *testing.T) {
	assert.Equal(t, "auditor=1, writer=2", describeCounts(map[string]int{"writer": 2, "auditor": 1}))
}

func TestSettingsAccessSubcommand_Run(t *testing.T) {
	runSubcommand(t, &settingsAccessSubcommand{}, []runCase{
		{name: "rejects extra args", args: []string{"extra"}, wantErr: "unexpected argument"},
		{name: "rejects a low concurrency", args: []string{"--concurrency=0"}, wantErr: "concurrency must be between 1 and 100"},
		{name: "rejects a high concurrency", args: []string{"--concurrency=101"}, wantErr: "concurrency must be between 1 and 100"},
		{name: "needs the OpenFGA settings", env: map[string]string{"OPENFGA_API_URL": "", "OPENFGA_STORE_ID": ""}, wantErr: "OPENFGA_API_URL and OPENFGA_STORE_ID are required"},
		{name: "an unreachable store fails the store check", env: map[string]string{"OPENFGA_API_URL": "http://127.0.0.1:1", "OPENFGA_STORE_ID": "01ARZ3NDEKTSV4RRFFQ69G5FAV"}, wantErr: "OpenFGA store check failed"},
		{name: "help is not an error", args: []string{"-h"}, wantErr: "-"},
	})
}
