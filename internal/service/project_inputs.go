// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"strings"
	"time"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
)

// UserInput is a user entry as a caller supplies it on a project write. A nil
// field means the caller did not set it, so the stored value is kept when the
// entry is merged with an existing record. Invite details are server-managed
// and cannot be supplied.
type UserInput struct {
	Name     *string
	Email    *string
	Username *string
	Avatar   *string
}

// ProjectSettingsInput is the writable part of a project's settings as a caller
// supplies it. A nil role slice means the caller left the role out; an empty,
// non-nil slice means the caller cleared it.
type ProjectSettingsInput struct {
	MissionStatement        string
	AnnouncementDate        *time.Time
	Writers                 []*UserInput
	Auditors                []*UserInput
	MeetingCoordinators     []*UserInput
	ExecutiveDirector       *UserInput
	MentorshipProgramAdmins []*UserInput
	ProgramManager          *UserInput
	OpportunityOwner        *UserInput
}

// userSlices returns every role slice, for steps that treat all roles alike.
func (in ProjectSettingsInput) userSlices() [][]*UserInput {
	return [][]*UserInput{in.Writers, in.Auditors, in.MeetingCoordinators, in.MentorshipProgramAdmins}
}

// userSingles returns every single-user role, for steps that treat all roles alike.
func (in ProjectSettingsInput) userSingles() []*UserInput {
	return []*UserInput{in.ExecutiveDirector, in.ProgramManager, in.OpportunityOwner}
}

// buildProjectSettings turns a caller's input into the settings record to store.
// existing, when non-nil, seeds read-only fields (e.g. Invite) that a caller cannot
// set. Pass nil on create; pass the current stored record on update. The caller
// sets the UID and the timestamps.
func buildProjectSettings(in ProjectSettingsInput, existing *models.ProjectSettings) *models.ProjectSettings {
	var existingWriters, existingAuditors, existingMCs []models.UserInfo
	var existingMentorshipProgramAdmins []models.UserInfo
	var existingED, existingPM, existingOO *models.UserInfo
	if existing != nil {
		existingWriters = existing.Writers
		existingAuditors = existing.Auditors
		existingMCs = existing.MeetingCoordinators
		existingED = existing.ExecutiveDirector
		existingMentorshipProgramAdmins = existing.MentorshipProgramAdmins
		existingPM = existing.ProgramManager
		existingOO = existing.OpportunityOwner
	}

	s := &models.ProjectSettings{
		MissionStatement: in.MissionStatement,
		AnnouncementDate: in.AnnouncementDate,
	}
	if in.Writers != nil {
		s.Writers = mergeUsers(in.Writers, existingWriters)
	}
	if in.Auditors != nil {
		s.Auditors = mergeUsers(in.Auditors, existingAuditors)
	}
	if in.MeetingCoordinators != nil {
		s.MeetingCoordinators = mergeUsers(in.MeetingCoordinators, existingMCs)
	}
	if in.ExecutiveDirector != nil {
		s.ExecutiveDirector = mergeUser(in.ExecutiveDirector, existingED)
	}
	if in.MentorshipProgramAdmins != nil {
		s.MentorshipProgramAdmins = mergeUsers(in.MentorshipProgramAdmins, existingMentorshipProgramAdmins)
	}
	if in.ProgramManager != nil {
		s.ProgramManager = mergeUser(in.ProgramManager, existingPM)
	}
	if in.OpportunityOwner != nil {
		s.OpportunityOwner = mergeUser(in.OpportunityOwner, existingOO)
	}
	return s
}

// mergeUsers converts the caller's users to domain users.
// existing, when provided, seeds each user with the stored record matched by email so
// that read-only fields (e.g. Invite) survive a PUT request without special-casing.
// Incoming fields are overlaid on top of the existing values, except an empty
// username: enrichAllRoleFields clears that on a lookup miss, and overlaying it would
// wipe a stored LFID and omit the writer key from update_access (GH-2301).
func mergeUsers(inputs []*UserInput, existing []models.UserInfo) []models.UserInfo {
	if len(inputs) == 0 {
		return nil
	}

	existingByEmail := make(map[string]models.UserInfo, len(existing))
	for _, u := range existing {
		if u.Email != "" {
			existingByEmail[strings.ToLower(strings.TrimSpace(u.Email))] = u
		}
	}

	users := make([]models.UserInfo, len(inputs))
	for i, input := range inputs {
		if input != nil {
			// Seed from the existing stored record so read-only fields are preserved.
			var user models.UserInfo
			if input.Email != nil {
				user = existingByEmail[strings.ToLower(strings.TrimSpace(*input.Email))]
			}
			users[i] = overlayUser(user, input)
		}
	}
	return users
}

// mergeUser converts a single caller user to a domain user.
// existing, when provided and matched by normalized email, seeds the stored record so a
// lookup miss (empty username) keeps the stored LFID instead of clearing it.
func mergeUser(input *UserInput, existing *models.UserInfo) *models.UserInfo {
	if input == nil {
		return nil
	}
	user := models.UserInfo{}
	if existing != nil && input.Email != nil {
		inputEmail := strings.ToLower(strings.TrimSpace(*input.Email))
		if inputEmail != "" && strings.ToLower(strings.TrimSpace(existing.Email)) == inputEmail {
			user = *existing
		}
	}
	user = overlayUser(user, input)
	return &user
}

// overlayUser lays the fields the caller is allowed to set on top of a stored user.
// Invite is server-managed, so it is never taken from the input.
func overlayUser(user models.UserInfo, input *UserInput) models.UserInfo {
	if input.Name != nil {
		user.Name = *input.Name
	}
	if input.Email != nil {
		user.Email = *input.Email
	}
	// A non-empty username is the auth-verified value from enrichAllRoleFields.
	// Empty means a lookup miss (or a pending invitee): keep the stored LFID.
	if input.Username != nil && strings.TrimSpace(*input.Username) != "" {
		user.Username = *input.Username
	}
	if input.Avatar != nil {
		user.Avatar = *input.Avatar
	}
	return user
}
