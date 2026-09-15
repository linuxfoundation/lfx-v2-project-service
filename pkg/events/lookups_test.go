// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package events

import (
	"errors"
	"testing"
)

func TestParseRPCError(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		wantErr   error // nil means wantIsErr must be false
		wantIsErr bool  // true when an error is expected but no specific sentinel
	}{
		{
			name:      "empty body is not an error envelope",
			data:      []byte{},
			wantErr:   nil,
			wantIsErr: false,
		},
		{
			name:      "non-JSON body is not an error envelope",
			data:      []byte("01234567-89ab-cdef-0123-456789abcdef"),
			wantErr:   nil,
			wantIsErr: false,
		},
		{
			name:      "JSON without error key is not an error envelope",
			data:      []byte(`{"uid":"01234567-89ab-cdef-0123-456789abcdef","slug":"kubernetes"}`),
			wantErr:   nil,
			wantIsErr: false,
		},
		{
			name:      "empty error key is not an error envelope",
			data:      []byte(`{"error":""}`),
			wantErr:   nil,
			wantIsErr: false,
		},
		{
			name:      "not_found code returns ErrRPCNotFound",
			data:      []byte(`{"error":"not_found","message":"project not found"}`),
			wantErr:   ErrRPCNotFound,
			wantIsErr: true,
		},
		{
			name:      "not_found code without message returns ErrRPCNotFound",
			data:      []byte(`{"error":"not_found"}`),
			wantErr:   ErrRPCNotFound,
			wantIsErr: true,
		},
		{
			name:      "internal code returns ErrRPCInternal",
			data:      []byte(`{"error":"internal","message":"internal server error"}`),
			wantErr:   ErrRPCInternal,
			wantIsErr: true,
		},
		{
			name:      "unknown code falls back to ErrRPCInternal",
			data:      []byte(`{"error":"unknown_future_code"}`),
			wantErr:   ErrRPCInternal,
			wantIsErr: true,
		},
		{
			name:      "not_found is not mistakenly ErrRPCInternal",
			data:      []byte(`{"error":"not_found"}`),
			wantErr:   ErrRPCNotFound,
			wantIsErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr, gotIsErr := ParseRPCError(tt.data)

			if gotIsErr != tt.wantIsErr {
				t.Fatalf("ParseRPCError() isErr = %v, want %v", gotIsErr, tt.wantIsErr)
			}

			if !tt.wantIsErr {
				if gotErr != nil {
					t.Errorf("ParseRPCError() err = %v, want nil", gotErr)
				}
				return
			}

			if gotErr == nil {
				t.Fatalf("ParseRPCError() err = nil, want non-nil")
			}

			if tt.wantErr != nil && !errors.Is(gotErr, tt.wantErr) {
				t.Errorf("ParseRPCError() err = %v, want errors.Is(%v)", gotErr, tt.wantErr)
			}

			// Cross-check: not_found must not also satisfy ErrRPCInternal and vice versa.
			if tt.wantErr == ErrRPCNotFound && errors.Is(gotErr, ErrRPCInternal) {
				t.Errorf("ParseRPCError() returned ErrRPCInternal for not_found input")
			}
			if tt.wantErr == ErrRPCInternal && errors.Is(gotErr, ErrRPCNotFound) {
				t.Errorf("ParseRPCError() returned ErrRPCNotFound for internal/unknown input")
			}
		})
	}
}
