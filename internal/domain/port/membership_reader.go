// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
)

// MembershipReader provides read access to project-scoped membership data from
// NATS KV. This interface is retained for compatibility with the mock
// implementation and test helpers; production code uses MemberReader directly.
type MembershipReader interface {
	GetMembership(ctx context.Context, uid string) (*model.ProjectMembership, error)
	ListMembershipsForProject(ctx context.Context, projectSFID string) ([]*model.ProjectMembership, error)
	ListKeyContactsForMembership(ctx context.Context, membershipUID string) ([]*model.ProjectKeyContact, error)
	IsReady(ctx context.Context) error
}
