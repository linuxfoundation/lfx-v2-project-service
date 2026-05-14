// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package salesforce

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errs "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/sfuuid"
)

// canonicalAccountSFID is a real 15-char Salesforce Account.Id used throughout
// reader and writer tests. Taken from the sfuuid test suite fixture.
const canonicalAccountSFID = "001B000000IqhSL"

// canonicalAccountJSON is a fully-populated Salesforce Account sObject REST
// response body. All fields included in b2bOrgFields are present.
const canonicalAccountJSON = `{
	"Id":"001B000000IqhSL",
	"Name":"Linux Foundation",
	"Logo_URL__c":"https://linuxfoundation.org/logo.png",
	"Website":"https://linuxfoundation.org",
	"Account_Domain__c":"linuxfoundation.org",
	"Domain_Alias__c":"lf.org, thelinuxfoundation.org",
	"Description":"Supporting open source ecosystems.",
	"Phone":"+1-415-723-9709",
	"ParentId":null,
	"Industry":"Technology",
	"Sector__c":"Non-Profit",
	"CrunchBase_URL__c":"https://www.crunchbase.com/organization/linux-foundation",
	"NumberOfEmployees":200,
	"LF_Membership_Status__c":"Active",
	"Slug__c":"linux-foundation",
	"CreatedDate":"2020-01-15T10:30:00.000+0000",
	"LastModifiedDate":"2024-06-01T08:00:00.000+0000",
	"SystemModstamp":"2024-06-01T08:00:00.000+0000"
}`

// TestSobjectAccountToB2BOrg_FixtureEquivalence verifies that
// sobjectAccountToB2BOrg (sObject REST path) and convertSOQLToB2BOrg (SOQL
// path) produce identical model.B2BOrg values for every field that both
// converters handle. The sObject path additionally populates Slug (Slug__c),
// which accountsSOQLBase does not select — that divergence is expected and
// asserted explicitly below.
func TestSobjectAccountToB2BOrg_FixtureEquivalence(t *testing.T) {
	t.Parallel()

	uid, err := sfuuid.ToUUID(canonicalAccountSFID)
	require.NoError(t, err)

	// ── sObject path ──────────────────────────────────────────────────────────
	var sObjRaw sobjectAccount
	require.NoError(t, json.Unmarshal([]byte(canonicalAccountJSON), &sObjRaw))
	sObjOrg, err := sobjectAccountToB2BOrg(context.Background(), &sObjRaw, uid)
	require.NoError(t, err)
	require.NotNil(t, sObjOrg)

	// ── SOQL path ─────────────────────────────────────────────────────────────
	// Populate soqlAccount with the same values (Slug__c excluded — accountsSOQLBase
	// does not select it).
	crunchURL := "https://www.crunchbase.com/organization/linux-foundation"
	var empCount int64 = 200
	logoURL := "https://linuxfoundation.org/logo.png"
	website := "https://linuxfoundation.org"
	primaryDomain := "linuxfoundation.org"
	domainAlias := "lf.org, thelinuxfoundation.org"
	description := "Supporting open source ecosystems."
	phone := "+1-415-723-9709"
	industry := "Technology"
	sector := "Non-Profit"
	status := "Active"

	soqlRaw := soqlAccount{
		ID:                canonicalAccountSFID,
		Name:              "Linux Foundation",
		LogoURL:           &logoURL,
		Website:           &website,
		PrimaryDomain:     &primaryDomain,
		DomainAlias:       &domainAlias,
		Description:       &description,
		Phone:             &phone,
		Industry:          &industry,
		Sector:            &sector,
		CrunchBaseURL:     &crunchURL,
		NumberOfEmployees: &empCount,
		Status:            &status,
		CreatedDate:       "2020-01-15T10:30:00.000+0000",
		LastModifiedDate:  "2024-06-01T08:00:00.000+0000",
	}
	soqlOrg, err := convertSOQLToB2BOrg(context.Background(), soqlRaw)
	require.NoError(t, err)
	require.NotNil(t, soqlOrg)

	// ── compare shared fields ─────────────────────────────────────────────────
	assert.Equal(t, soqlOrg.UID, sObjOrg.UID, "UID")
	assert.Equal(t, soqlOrg.SFID, sObjOrg.SFID, "SFID")
	assert.Equal(t, soqlOrg.Name, sObjOrg.Name, "Name")
	assert.Equal(t, soqlOrg.LogoURL, sObjOrg.LogoURL, "LogoURL")
	assert.Equal(t, soqlOrg.Website, sObjOrg.Website, "Website")
	assert.Equal(t, soqlOrg.PrimaryDomain, sObjOrg.PrimaryDomain, "PrimaryDomain")
	assert.Equal(t, soqlOrg.DomainAliases, sObjOrg.DomainAliases, "DomainAliases")
	assert.Equal(t, soqlOrg.Description, sObjOrg.Description, "Description")
	assert.Equal(t, soqlOrg.Phone, sObjOrg.Phone, "Phone")
	assert.Equal(t, soqlOrg.Industry, sObjOrg.Industry, "Industry")
	assert.Equal(t, soqlOrg.Sector, sObjOrg.Sector, "Sector")
	assert.Equal(t, soqlOrg.CrunchBaseURL, sObjOrg.CrunchBaseURL, "CrunchBaseURL")
	assert.Equal(t, soqlOrg.NumberOfEmployees, sObjOrg.NumberOfEmployees, "NumberOfEmployees")
	assert.Equal(t, soqlOrg.Status, sObjOrg.Status, "Status")
	assert.Equal(t, soqlOrg.CreatedAt.UTC(), sObjOrg.CreatedAt.UTC(), "CreatedAt")
	assert.Equal(t, soqlOrg.UpdatedAt.UTC(), sObjOrg.UpdatedAt.UTC(), "UpdatedAt")

	// Slug is only populated by the sObject path (Slug__c absent from accountsSOQLBase).
	assert.Equal(t, "linux-foundation", sObjOrg.Slug, "sObject path must populate Slug")
	assert.Empty(t, soqlOrg.Slug, "SOQL path must leave Slug empty (not in accountsSOQLBase)")
}

// TestB2BOrgReader_GetB2BOrg_Happy verifies that GetB2BOrg returns a fully-
// populated B2BOrg when Salesforce responds with 200 OK.
func TestB2BOrgReader_GetB2BOrg_Happy(t *testing.T) {
	t.Parallel()

	uid, err := sfuuid.ToUUID(canonicalAccountSFID)
	require.NoError(t, err)

	transport := newRoutingTransport(fakeResponse(http.StatusOK, canonicalAccountJSON, nil))
	client := &SObjectClient{sf: fakeSalesforce(t, transport), cache: newMemCache()}
	reader := NewB2BOrgReader(client)

	org, err := reader.GetB2BOrg(context.Background(), uid)

	require.NoError(t, err)
	require.NotNil(t, org)
	assert.Equal(t, uid, org.UID)
	assert.Equal(t, "Linux Foundation", org.Name)
	assert.Equal(t, "linux-foundation", org.Slug)
	assert.Equal(t, "Technology", org.Industry)
	assert.Equal(t, "Non-Profit", org.Sector)
	assert.Equal(t, "Active", org.Status)
	require.NotNil(t, org.NumberOfEmployees)
	assert.Equal(t, int64(200), *org.NumberOfEmployees)
	assert.Equal(t, []string{"lf.org", "thelinuxfoundation.org"}, org.DomainAliases)
}

// TestB2BOrgReader_GetB2BOrg_NotFound verifies that GetB2BOrg propagates a
// NotFound error when Salesforce returns 404.
func TestB2BOrgReader_GetB2BOrg_NotFound(t *testing.T) {
	t.Parallel()

	uid, err := sfuuid.ToUUID(canonicalAccountSFID)
	require.NoError(t, err)

	transport := newRoutingTransport(fakeResponse(http.StatusNotFound,
		`[{"errorCode":"NOT_FOUND","message":"The requested resource does not exist"}]`,
		nil))
	client := &SObjectClient{sf: fakeSalesforce(t, transport), cache: newMemCache()}
	reader := NewB2BOrgReader(client)

	org, err := reader.GetB2BOrg(context.Background(), uid)

	require.Error(t, err)
	assert.Nil(t, org)

	var notFound errs.NotFound
	assert.True(t, errors.As(err, &notFound),
		"expected NotFound error, got %T: %v", err, err)
}

// TestB2BOrgReader_GetB2BOrg_InvalidUID verifies that GetB2BOrg returns a
// Validation error when the provided UID is not a valid LFX_ UUID.
func TestB2BOrgReader_GetB2BOrg_InvalidUID(t *testing.T) {
	t.Parallel()

	transport := &routingTransport{}
	transport.route("/limits", fakeResponse(http.StatusOK, `{}`, nil))
	client := &SObjectClient{sf: fakeSalesforce(t, transport), cache: newMemCache()}
	reader := NewB2BOrgReader(client)

	org, err := reader.GetB2BOrg(context.Background(), "not-a-valid-uid")

	require.Error(t, err)
	assert.Nil(t, org)
}
