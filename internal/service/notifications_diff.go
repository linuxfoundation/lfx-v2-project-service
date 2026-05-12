// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"
)

// roleAssignment pairs a user with the role they were added to.
type roleAssignment struct {
	User events.UserInfo
	Role string
}

// diffNewMembers returns the users that appear in newSettings but not in oldSettings,
// across writers, auditors, and meeting_coordinators. Users are matched by Username
// when present, otherwise by Email. Users with neither Username nor Email are skipped.
func diffNewMembers(oldSettings, newSettings events.ProjectSettings) []roleAssignment {
	var additions []roleAssignment
	additions = append(additions, diffRole(oldSettings.Writers, newSettings.Writers, "Writer")...)
	additions = append(additions, diffRole(oldSettings.Auditors, newSettings.Auditors, "Auditor")...)
	additions = append(additions, diffRole(oldSettings.MeetingCoordinators, newSettings.MeetingCoordinators, "Meeting Coordinator")...)
	return additions
}

func diffRole(old, new []events.UserInfo, role string) []roleAssignment {
	oldSet := make(map[string]struct{}, len(old))
	for _, u := range old {
		if key := memberKey(u); key != "" {
			oldSet[key] = struct{}{}
		}
	}
	seenNew := make(map[string]struct{}, len(new))
	var additions []roleAssignment
	for _, u := range new {
		key := memberKey(u)
		if key == "" {
			continue
		}
		if _, alreadySeen := seenNew[key]; alreadySeen {
			continue
		}
		seenNew[key] = struct{}{}
		if _, exists := oldSet[key]; !exists {
			additions = append(additions, roleAssignment{User: u, Role: role})
		}
	}
	return additions
}

// memberKey returns a stable identity key for a user.
// Username takes priority; Email is the fallback. Returns "" if neither is set.
func memberKey(u events.UserInfo) string {
	if u.Username != "" {
		return "username:" + u.Username
	}
	if u.Email != "" {
		return "email:" + u.Email
	}
	return ""
}
