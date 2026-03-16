// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	membershipservice "github.com/linuxfoundation/lfx-v2-member-service/gen/membership_service"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
)

// strPtr returns a pointer to s. Used for optional string fields in Goa
// response structs where the zero value is meaningfully absent.
func strPtr(s string) *string { return &s }

// convertTierToResponse converts a domain MembershipTier to a Goa response.
func convertTierToResponse(t *model.MembershipTier) *membershipservice.MembershipTierResponse {
	if t == nil {
		return nil
	}

	r := &membershipservice.MembershipTierResponse{
		UID:        &t.UID,
		ProjectUID: &t.ProjectUID,
		Name:       &t.Name,
	}

	if t.Family != "" {
		r.Family = &t.Family
	}
	if t.ProductType != "" {
		r.ProductType = &t.ProductType
	}
	if !t.CreatedAt.IsZero() {
		s := t.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
		r.CreatedAt = &s
	}
	if !t.UpdatedAt.IsZero() {
		s := t.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
		r.UpdatedAt = &s
	}

	return r
}

// convertProjectMembershipToResponse converts a domain ProjectMembership to a
// Goa response.
func convertProjectMembershipToResponse(m *model.ProjectMembership) *membershipservice.ProjectMembershipResponse {
	if m == nil {
		return nil
	}

	r := &membershipservice.ProjectMembershipResponse{
		UID:             &m.UID,
		TierUID:         &m.TierUID,
		ProjectUID:      &m.ProjectUID,
		Status:          &m.Status,
		MembershipType:  &m.MembershipType,
		AutoRenew:       &m.AutoRenew,
		Price:           &m.Price,
		AnnualFullPrice: &m.AnnualFullPrice,
		CompanyName:     &m.CompanyName,
	}

	if m.Year != "" {
		r.Year = &m.Year
	}
	if m.Tier != "" {
		r.Tier = &m.Tier
	}
	if m.RenewalType != "" {
		r.RenewalType = &m.RenewalType
	}
	if m.PaymentFrequency != "" {
		r.PaymentFrequency = &m.PaymentFrequency
	}
	if m.PaymentTerms != "" {
		r.PaymentTerms = &m.PaymentTerms
	}
	if m.AgreementDate != "" {
		r.AgreementDate = &m.AgreementDate
	}
	if m.PurchaseDate != "" {
		r.PurchaseDate = &m.PurchaseDate
	}
	if m.StartDate != "" {
		r.StartDate = &m.StartDate
	}
	if m.EndDate != "" {
		r.EndDate = &m.EndDate
	}
	if m.CompanyLogoURL != "" {
		r.CompanyLogoURL = &m.CompanyLogoURL
	}
	if m.CompanyDomain != "" {
		r.CompanyDomain = &m.CompanyDomain
	}
	if m.TierName != "" {
		r.TierName = &m.TierName
	}
	if m.TierFamily != "" {
		r.TierFamily = &m.TierFamily
	}
	if m.TierProductType != "" {
		r.TierProductType = &m.TierProductType
	}
	if !m.CreatedAt.IsZero() {
		s := m.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
		r.CreatedAt = &s
	}
	if !m.UpdatedAt.IsZero() {
		s := m.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
		r.UpdatedAt = &s
	}

	return r
}

// convertProjectKeyContactToResponse converts a domain ProjectKeyContact to a
// Goa response.
func convertProjectKeyContactToResponse(c *model.ProjectKeyContact) *membershipservice.ProjectKeyContactResponse {
	if c == nil {
		return nil
	}

	r := &membershipservice.ProjectKeyContactResponse{
		UID:            &c.UID,
		MembershipUID:  &c.MembershipUID,
		TierUID:        &c.TierUID,
		ProjectUID:     &c.ProjectUID,
		Role:           &c.Role,
		Status:         &c.Status,
		BoardMember:    &c.BoardMember,
		PrimaryContact: &c.PrimaryContact,
		FirstName:      &c.FirstName,
		LastName:       &c.LastName,
		CompanyName:    &c.CompanyName,
	}

	if c.Title != "" {
		r.Title = &c.Title
	}
	if c.Email != "" {
		r.Email = &c.Email
	}
	if c.CompanyLogoURL != "" {
		r.CompanyLogoURL = &c.CompanyLogoURL
	}
	if c.CompanyDomain != "" {
		r.CompanyDomain = &c.CompanyDomain
	}
	if !c.CreatedAt.IsZero() {
		s := c.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
		r.CreatedAt = &s
	}
	if !c.UpdatedAt.IsZero() {
		s := c.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
		r.UpdatedAt = &s
	}

	return r
}
