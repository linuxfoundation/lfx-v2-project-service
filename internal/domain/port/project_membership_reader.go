// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import (
	"context"
	"time"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
)

// ProjectMembershipReader assembles a fully denormalised ProjectMembership from
// its constituent Salesforce objects (Asset, Account, Product2, Project__c).
type ProjectMembershipReader interface {
	// AssembleProjectMembership fetches all related sObjects for the given
	// Asset UID and returns a fully denormalised ProjectMembership. The
	// returned time.Time is the oldest Last-Modified across the constituent
	// records and should be used as the HTTP Last-Modified response header.
	AssembleProjectMembership(ctx context.Context, uid string) (*model.ProjectMembership, time.Time, error)
}
