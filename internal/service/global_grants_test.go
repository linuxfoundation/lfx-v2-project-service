// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	wantOwnerGrants = []string{"team:formation#member", "team:product-support#member"}
	wantAllGrants   = map[string][]string{
		"global_owner":         wantOwnerGrants,
		"global_writer":        {"team:global-project-writers#member"},
		"global_auditor":       {"team:lf-staff#member", "team:global-project-auditors#member"},
		"global_marketing_ops": {"team:marketing-ops#member"},
	}
	wantOwnerOnlyGrants = map[string][]string{"global_owner": wantOwnerGrants}
)

// wantAllGrantsWithParent is the references map of an ordinary-stage project under parentUID.
func wantAllGrantsWithParent(parentUID string) map[string][]string {
	refs := map[string][]string{"parent": {"project:" + parentUID}}
	for relation, subjects := range wantAllGrants {
		refs[relation] = subjects
	}
	return refs
}

// allProjectStages is the stage enum from the API design, plus the empty stage the root
// project carries.
var allProjectStages = []string{
	"",
	"Formation - Exploratory",
	"Formation - Engaged",
	"Active",
	"Archived",
	"Formation - On Hold",
	"Formation - Disengaged",
	"Formation - Confidential",
	"Prospect",
}

func TestGlobalTeamGrants(t *testing.T) {
	tests := []struct {
		name  string
		stage string
		want  map[string][]string
	}{
		{name: "active project gets every grant", stage: "Active", want: wantAllGrants},
		{name: "exploratory formation gets every grant", stage: "Formation - Exploratory", want: wantAllGrants},
		{name: "engaged formation gets every grant", stage: "Formation - Engaged", want: wantAllGrants},
		{name: "on-hold formation gets every grant", stage: "Formation - On Hold", want: wantAllGrants},
		{name: "disengaged formation gets every grant", stage: "Formation - Disengaged", want: wantAllGrants},
		{name: "archived project gets every grant", stage: "Archived", want: wantAllGrants},
		{name: "project with no stage, such as the root, gets every grant", stage: "", want: wantAllGrants},
		{name: "prospect gets the owner grants only", stage: "Prospect", want: wantOwnerOnlyGrants},
		{name: "confidential formation gets the owner grants only", stage: "Formation - Confidential", want: wantOwnerOnlyGrants},
		{name: "near miss without spaces is an ordinary stage", stage: "Formation-Confidential", want: wantAllGrants},
		{name: "near miss in lower case is an ordinary stage", stage: "formation - confidential", want: wantAllGrants},
		{name: "near miss with trailing space is an ordinary stage", stage: "Formation - Confidential ", want: wantAllGrants},
		{name: "lower-case prospect is an ordinary stage", stage: "prospect", want: wantAllGrants},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, globalTeamGrants(tt.stage))
		})
	}

	t.Run("both owner teams hold global_owner on every stage", func(t *testing.T) {
		for _, stage := range allProjectStages {
			assert.Equal(t, wantOwnerGrants, globalTeamGrants(stage)["global_owner"], "stage %q", stage)
		}
	})

	t.Run("lf-contractor receives no global grant on any stage", func(t *testing.T) {
		for _, stage := range allProjectStages {
			for relation, subjects := range globalTeamGrants(stage) {
				for _, subject := range subjects {
					assert.False(t, strings.Contains(subject, "lf-contractor"), "stage %q: %s grants %s", stage, relation, subject)
				}
			}
		}
	})
}
