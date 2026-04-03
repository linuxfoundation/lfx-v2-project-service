// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package salesforce

import (
	"encoding/json"
	"testing"
)

// TestSoqlAssetProjectsRDecoding verifies that soqlAsset correctly decodes the
// Projects__r field from Salesforce REST API wire JSON. Projects__r is a direct
// parent lookup relationship (not a child sub-select), so Salesforce inlines it
// as a flat object with an "attributes" key alongside the requested fields.
func TestSoqlAssetProjectsRDecoding(t *testing.T) {
	slug := "cncf"

	tests := []struct {
		name        string
		input       string
		wantProject *soqlAssetProject
		wantErr     bool
	}{
		{
			name: "projects_r_as_inline_object_populates_project",
			// This matches the actual Salesforce REST wire format observed in
			// production: Projects__r is a flat object, not a sub-select envelope.
			input: `{
				"Id": "02iQP00000MFnzpYAD",
				"Name": "Test Asset",
				"AccountId": "0014100000Tdzl5AAB",
				"Product2Id": "01t2M00000HP7IpQAL",
				"IsDeleted": false,
				"Auto_Renew__c": true,
				"Price": 37500.0,
				"Annual_Full_Price__c": 50000.0,
				"CreatedDate": "2026-03-30T06:54:50.000+0000",
				"LastModifiedDate": "2026-04-03T17:30:24.000+0000",
				"Account": {
					"attributes": {"type": "Account", "url": "/services/data/v63.0/sobjects/Account/0014100000Tdzl5AAB"},
					"Id": "0014100000Tdzl5AAB",
					"Name": "Atlassian US, Inc"
				},
				"Product2": {
					"attributes": {"type": "Product2", "url": "/services/data/v63.0/sobjects/Product2/01t2M00000HP7IpQAL"},
					"Id": "01t2M00000HP7IpQAL",
					"Name": "CNCF Silver Membership",
					"Family": "Membership"
				},
				"Projects__c": "a0941000002wBz4AAE",
				"Projects__r": {
					"attributes": {"type": "Project__c", "url": "/services/data/v63.0/sobjects/Project__c/a0941000002wBz4AAE"},
					"Id": "a0941000002wBz4AAE",
					"Name": "Cloud Native Computing Foundation (CNCF)",
					"Slug__c": "cncf",
					"Status__c": "Active"
				}
			}`,
			wantProject: &soqlAssetProject{ID: "a0941000002wBz4AAE", Name: "Cloud Native Computing Foundation (CNCF)", Slug: &slug},
		},
		{
			name: "projects_r_null_leaves_project_nil",
			input: `{
				"Id": "02iQP00000XYZ",
				"Name": "Test Asset",
				"AccountId": "001000000AAA",
				"Product2Id": "01t000000BBB",
				"IsDeleted": false,
				"Auto_Renew__c": false,
				"Price": 0.0,
				"Annual_Full_Price__c": 0.0,
				"CreatedDate": "2024-01-01T00:00:00.000+0000",
				"LastModifiedDate": "2024-01-01T00:00:00.000+0000",
				"Projects__r": null
			}`,
			wantProject: nil,
		},
		{
			name: "projects_r_absent_leaves_project_nil",
			input: `{
				"Id": "02iQP00000XYZ",
				"Name": "Test Asset",
				"AccountId": "001000000AAA",
				"Product2Id": "01t000000BBB",
				"IsDeleted": false,
				"Auto_Renew__c": false,
				"Price": 0.0,
				"Annual_Full_Price__c": 0.0,
				"CreatedDate": "2024-01-01T00:00:00.000+0000",
				"LastModifiedDate": "2024-01-01T00:00:00.000+0000"
			}`,
			wantProject: nil,
		},
		{
			name:    "invalid_json_returns_error",
			input:   `{not valid json`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var asset soqlAsset
			err := json.Unmarshal([]byte(tt.input), &asset)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantProject == nil {
				if asset.Project != nil {
					t.Errorf("expected Project to be nil, got %+v", asset.Project)
				}
				return
			}

			if asset.Project == nil {
				t.Fatal("expected Project to be non-nil, got nil")
			}
			if asset.Project.ID != tt.wantProject.ID {
				t.Errorf("Project.ID: got %q, want %q", asset.Project.ID, tt.wantProject.ID)
			}
			if asset.Project.Name != tt.wantProject.Name {
				t.Errorf("Project.Name: got %q, want %q", asset.Project.Name, tt.wantProject.Name)
			}
			if tt.wantProject.Slug != nil {
				if asset.Project.Slug == nil {
					t.Error("expected Project.Slug to be non-nil")
				} else if *asset.Project.Slug != *tt.wantProject.Slug {
					t.Errorf("Project.Slug: got %q, want %q", *asset.Project.Slug, *tt.wantProject.Slug)
				}
			}
		})
	}
}
