// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"errors"
	"testing"

	membershipservice "github.com/linuxfoundation/lfx-v2-member-service/gen/membership_service"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/auth"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/mock"
	usecaseSvc "github.com/linuxfoundation/lfx-v2-member-service/internal/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goa "goa.design/goa/v3/pkg"
)

func TestStubHandlers_ReturnNotImplemented(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		callHandler func(svc membershipservice.Service, ctx context.Context) error
	}{
		{
			name:   "GetB2bOrg returns NotImplemented",
			method: "GetB2bOrg",
			callHandler: func(svc membershipservice.Service, ctx context.Context) error {
				_, err := svc.GetB2bOrg(ctx, &membershipservice.GetB2bOrgPayload{
					UID: "test-uid",
				})
				return err
			},
		},
		{
			name:   "CreateB2bOrg returns NotImplemented",
			method: "CreateB2bOrg",
			callHandler: func(svc membershipservice.Service, ctx context.Context) error {
				_, err := svc.CreateB2bOrg(ctx, &membershipservice.CreateB2bOrgPayload{
					Name: "Test Org",
				})
				return err
			},
		},
		{
			name:   "UpdateB2bOrg returns NotImplemented",
			method: "UpdateB2bOrg",
			callHandler: func(svc membershipservice.Service, ctx context.Context) error {
				_, err := svc.UpdateB2bOrg(ctx, &membershipservice.UpdateB2bOrgPayload{
					UID: "test-uid",
				})
				return err
			},
		},
		{
			name:   "GetProjectMembership returns NotImplemented",
			method: "GetProjectMembership",
			callHandler: func(svc membershipservice.Service, ctx context.Context) error {
				_, err := svc.GetProjectMembership(ctx, &membershipservice.GetProjectMembershipPayload{
					UID: "test-uid",
				})
				return err
			},
		},
		{
			name:   "GetKeyContact returns NotImplemented",
			method: "GetKeyContact",
			callHandler: func(svc membershipservice.Service, ctx context.Context) error {
				_, err := svc.GetKeyContact(ctx, &membershipservice.GetKeyContactPayload{
					UID: "test-uid",
				})
				return err
			},
		},
		{
			name:   "CreateKeyContact returns NotImplemented",
			method: "CreateKeyContact",
			callHandler: func(svc membershipservice.Service, ctx context.Context) error {
				_, err := svc.CreateKeyContact(ctx, &membershipservice.CreateKeyContactPayload{
					B2bOrgUID:     "org-uid",
					ProjectUID:    "project-uid",
					MembershipUID: "membership-uid",
					Email:         "test@example.com",
					FirstName:     "Test",
					LastName:      "User",
				})
				return err
			},
		},
		{
			name:   "UpdateKeyContact returns NotImplemented",
			method: "UpdateKeyContact",
			callHandler: func(svc membershipservice.Service, ctx context.Context) error {
				_, err := svc.UpdateKeyContact(ctx, &membershipservice.UpdateKeyContactPayload{
					UID: "test-uid",
				})
				return err
			},
		},
		{
			name:   "DeleteKeyContact returns NotImplemented",
			method: "DeleteKeyContact",
			callHandler: func(svc membershipservice.Service, ctx context.Context) error {
				return svc.DeleteKeyContact(ctx, &membershipservice.DeleteKeyContactPayload{
					UID: "test-uid",
				})
			},
		},
		{
			name:   "AdminReindex returns NotImplemented",
			method: "AdminReindex",
			callHandler: func(svc membershipservice.Service, ctx context.Context) error {
				_, err := svc.AdminReindex(ctx, &membershipservice.AdminReindexPayload{})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestMembershipService()
			ctx := context.Background()

			err := tt.callHandler(svc, ctx)

			require.Error(t, err)

			var serviceErr *goa.ServiceError
			ok := errors.As(err, &serviceErr)
			require.True(t, ok, "error should be a *goa.ServiceError, got %T: %v", err, err)

			assert.Equal(t, "NotImplemented", serviceErr.Name)
		})
	}
}

// newTestMembershipService constructs a MembershipService with mock dependencies
// for testing.
func newTestMembershipService() membershipservice.Service {
	mockRepo := mock.NewMockMembershipRepository()
	mockB2BOrgReader := mock.NewMockB2BOrgReader()
	mockKeyContactWriter := &mockKeyContactWriter{}

	readMemberUseCase := usecaseSvc.NewMemberReaderOrchestrator(
		usecaseSvc.WithMemberReader(mockRepo),
	)

	mockAuth := &auth.MockJWTAuth{}

	return NewMembershipService(
		readMemberUseCase,
		mockRepo,
		mockAuth,
		mockKeyContactWriter,
		mockB2BOrgReader,
	)
}

// mockKeyContactWriter is a simple test implementation of port.KeyContactWriter.
type mockKeyContactWriter struct{}

func (m *mockKeyContactWriter) CreateKeyContact(ctx context.Context, input model.KeyContactInput) (*model.KeyContact, error) {
	return nil, nil
}

func (m *mockKeyContactWriter) UpdateKeyContact(ctx context.Context, contactUID string, input model.KeyContactInput) (*model.KeyContact, error) {
	return nil, nil
}

func (m *mockKeyContactWriter) DeleteKeyContact(ctx context.Context, contactUID string, membershipUID string) error {
	return nil
}

// Verify that the mock ports are properly implemented.
var (
	_ port.MemberReader     = (*mock.MockMembershipRepository)(nil)
	_ port.KeyContactWriter = (*mockKeyContactWriter)(nil)
	_ port.B2BOrgReader     = (*mock.MockB2BOrgReader)(nil)
)
