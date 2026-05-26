// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package email

// RoleCapabilityGroup pairs a display role name with the list of capabilities that role grants.
type RoleCapabilityGroup struct {
	Role  string
	Items []string
}

// roleCapabilities maps each display role name to its capability list.
var roleCapabilities = map[string][]string{
	"Manage": {
		"Create & update subprojects",
		"Update project settings",
		"Manage project membership key contacts",
		"Create a vote",
		"Create project groups & mailing lists",
		"Create project meetings",
		"Create project past meetings",
	},
	"View": {
		"View project",
		"View project settings",
		"View project memberships & member companies & key contacts",
		"View B2B organization memberships",
	},
	"Meeting Coordinator": {
		"Create project meetings",
		"Create project past meetings",
	},
}

// capabilityGroupsFor returns a RoleCapabilityGroup for each display role that has registered
// capabilities, preserving the input order.
func capabilityGroupsFor(displayRoles []string) []RoleCapabilityGroup {
	groups := make([]RoleCapabilityGroup, 0, len(displayRoles))
	for _, r := range displayRoles {
		if items, ok := roleCapabilities[r]; ok {
			groups = append(groups, RoleCapabilityGroup{Role: r, Items: items})
		}
	}
	return groups
}
