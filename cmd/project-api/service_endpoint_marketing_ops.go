// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
)

// AddProjectMarketingOpsMember grants a user Marketing Ops access scoped to a single project.
func (s *ProjectsAPI) AddProjectMarketingOpsMember(ctx context.Context, payload *projsvc.AddProjectMarketingOpsMemberPayload) error {
	if err := s.service.AddMarketingOpsMember(ctx, payload.UID, payload.Username); err != nil {
		return handleError(ctx, err)
	}
	return nil
}

// RemoveProjectMarketingOpsMember revokes a user's Marketing Ops access for a single project.
func (s *ProjectsAPI) RemoveProjectMarketingOpsMember(ctx context.Context, payload *projsvc.RemoveProjectMarketingOpsMemberPayload) error {
	if err := s.service.RemoveMarketingOpsMember(ctx, payload.UID, payload.Username); err != nil {
		return handleError(ctx, err)
	}
	return nil
}
