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

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
)

type fakeProjectLister struct {
	bases []*models.ProjectBase
	err   error
}

func (f fakeProjectLister) ListAllProjectsBase(context.Context) ([]*models.ProjectBase, error) {
	return f.bases, f.err
}

func TestRunIndexDrift(t *testing.T) {
	bases := []*models.ProjectBase{
		{UID: uid1, Slug: "alpha"},
		{UID: uid2, Slug: "beta"},
		{UID: uid3, Slug: "ROOT"},
	}

	t.Run("fully indexed projects are clean and ROOT is not queried", func(t *testing.T) {
		client := newOpenSearchClient(t,
			"project:"+uid1, "project_settings:"+uid1,
			"project:"+uid2, "project_settings:"+uid2)
		var out bytes.Buffer

		require.NoError(t, runIndexDrift(context.Background(), fakeProjectLister{bases: bases}, client, &out))

		var report indexDriftReport
		require.NoError(t, json.Unmarshal(out.Bytes(), &report))
		assert.Equal(t, 2, report.Projects, "the hidden ROOT project is left out")
		assert.Empty(t, report.Findings)
	})

	t.Run("missing documents are reported per project", func(t *testing.T) {
		client := newOpenSearchClient(t, "project:"+uid1, "project_settings:"+uid1, "project:"+uid2)
		var out bytes.Buffer

		err := runIndexDrift(context.Background(), fakeProjectLister{bases: bases}, client, &out)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "found 1 problem(s)")
		var report indexDriftReport
		require.NoError(t, json.Unmarshal(out.Bytes(), &report))
		assert.Equal(t, map[string]int{kindMissingSettingsDoc: 1}, report.Counts)
		assert.Equal(t, []finding{{Kind: kindMissingSettingsDoc, Slug: "beta", UID: uid2}}, report.Findings)
	})

	t.Run("a project with no documents lacks both", func(t *testing.T) {
		var out bytes.Buffer

		err := runIndexDrift(context.Background(), fakeProjectLister{bases: bases[:1]}, newOpenSearchClient(t), &out)

		require.Error(t, err)
		var report indexDriftReport
		require.NoError(t, json.Unmarshal(out.Bytes(), &report))
		assert.Equal(t, map[string]int{kindMissingProjectDoc: 1, kindMissingSettingsDoc: 1}, report.Counts)
	})

	t.Run("project listing failure", func(t *testing.T) {
		err := runIndexDrift(context.Background(), fakeProjectLister{err: errors.New("boom")}, newOpenSearchClient(t), &bytes.Buffer{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list project bases")
	})
}

func TestIndexDriftSubcommand_Run(t *testing.T) {
	runSubcommand(t, &indexDriftSubcommand{}, []runCase{
		{name: "rejects extra args", args: []string{"extra"}, wantErr: "unexpected argument"},
		{name: "fails at the NATS connect"},
		{name: "help is not an error", args: []string{"-h"}, wantErr: "-"},
	})
}
