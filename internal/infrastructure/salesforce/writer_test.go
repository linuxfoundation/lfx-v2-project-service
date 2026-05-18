// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package salesforce

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsDuplicateSFError pins the expected go-salesforce v3 error format so that
// a future library change that alters how the SF response body is surfaced (e.g.
// switching to fmt.Errorf("%w", …)) breaks this test loudly rather than silently
// disabling the self-heal path at runtime.
func TestIsDuplicateSFError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil_error",
			err:  nil,
			want: false,
		},
		{
			name: "duplicate_value_error_code",
			// go-salesforce v3 surfaces the raw SF JSON response body as errors.New(responseData).
			err:  errors.New(`[{"message":"Use one of these records?","errorCode":"DUPLICATE_VALUE","fields":[]}]`),
			want: true,
		},
		{
			name: "duplicates_detected_error_code",
			err:  errors.New(`[{"message":"Duplicate detected","errorCode":"DUPLICATES_DETECTED","fields":[]}]`),
			want: true,
		},
		{
			name: "unrelated_sf_error_code",
			err:  errors.New(`[{"message":"Record not found","errorCode":"NOT_FOUND","fields":[]}]`),
			want: false,
		},
		{
			name: "non_json_error",
			err:  errors.New("connection refused"),
			want: false,
		},
		{
			name: "empty_sf_errors_array",
			err:  errors.New(`[]`),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isDuplicateSFError(tt.err))
		})
	}
}
