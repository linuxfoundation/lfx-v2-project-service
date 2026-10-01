// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	fgaconstants "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
)

// Global team grants written on every project's access message. The relations are typed
// [team#member] in the model, so they travel as references rather than username relations.
const (
	relationGlobalOwner        = "global_owner"
	relationGlobalWriter       = "global_writer"
	relationGlobalAuditor      = "global_auditor"
	relationGlobalMarketingOps = "global_marketing_ops"

	stageProspect              = "Prospect"
	stageFormationConfidential = "Formation - Confidential"
	teamFormation              = "formation"
	teamProductSupport         = "product-support"
	teamLFStaff                = "lf-staff"
	teamGlobalProjectAuditors  = "global-project-auditors"
	teamGlobalProjectWriters   = "global-project-writers"
	teamMarketingOps           = "marketing-ops"
)

// globalOwnerTeams hold global_owner on every project, whatever its stage. Owner stays out of
// the stage predicate on purpose: owner composes into writer, and a direct writer cascades to
// child projects, so withholding owner on one project would not withhold anything.
var globalOwnerTeams = []string{teamFormation, teamProductSupport}

// stageGatedGrants are withheld while a project is a prospect or confidential. Omitting a
// relation from the message is what withdraws it: fga-sync removes team tuples on global_*
// relations that the message no longer carries.
var stageGatedGrants = map[string][]string{
	relationGlobalWriter:       {teamGlobalProjectWriters},
	relationGlobalAuditor:      {teamLFStaff, teamGlobalProjectAuditors},
	relationGlobalMarketingOps: {teamMarketingOps},
}

// withholdsGlobalGrants reports whether a project's stage withholds the stage-gated grants.
// The match is exact: a near-miss spelling is an ordinary stage and receives every grant.
func withholdsGlobalGrants(stage string) bool {
	return stage == stageProspect || stage == stageFormationConfidential
}

// globalTeamGrants returns the global team grants for a project in the stage given, keyed by
// relation, with each team rendered as a team#member subject.
func globalTeamGrants(stage string) map[string][]string {
	grants := map[string][]string{relationGlobalOwner: teamMemberSubjects(globalOwnerTeams)}
	if withholdsGlobalGrants(stage) {
		return grants
	}
	for relation, teams := range stageGatedGrants {
		grants[relation] = teamMemberSubjects(teams)
	}
	return grants
}

func teamMemberSubjects(teams []string) []string {
	subjects := make([]string, len(teams))
	for i, team := range teams {
		subjects[i] = fgaconstants.ObjectTypeTeam + team + "#member"
	}
	return subjects
}
