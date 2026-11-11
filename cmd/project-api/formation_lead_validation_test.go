// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	projectserver "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/http/project_service/server"
)

func TestFormationLeadEmailValidation(t *testing.T) {
	for _, tt := range []struct {
		name    string
		email   string
		wantErr bool
	}{
		{name: "apostrophe in local part", email: "o'connor@example.com"},
		{name: "ordinary address", email: "alice@example.com"},
		{name: "empty allowed", email: ""},
		{name: "invalid domain", email: "alice@not-a-domain", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := projectserver.ValidateFormationLeadRequestBody(&projectserver.FormationLeadRequestBody{Email: &tt.email})
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFormationLeadRequestBody(%q) error = %v, wantErr %t", tt.email, err, tt.wantErr)
			}
		})
	}
}
