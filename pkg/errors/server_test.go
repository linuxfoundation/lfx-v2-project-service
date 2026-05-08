// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package errors

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewUnexpected(t *testing.T) {
	tests := []struct {
		name    string
		message string
		err     []error
		wantMsg string
	}{
		{
			name:    "message only",
			message: "unexpected error occurred",
			err:     nil,
			wantMsg: "unexpected error occurred",
		},
		{
			name:    "message with single error",
			message: "unexpected error occurred",
			err:     []error{errors.New("database connection failed")},
			wantMsg: "unexpected error occurred: database connection failed",
		},
		{
			name:    "message with multiple errors",
			message: "unexpected error occurred",
			err:     []error{errors.New("error 1"), errors.New("error 2")},
			wantMsg: "unexpected error occurred: error 1\nerror 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := NewUnexpected(tt.message, tt.err...)
			assert.Equal(t, tt.wantMsg, u.Error())
		})
	}
}

func TestNewServiceUnavailable(t *testing.T) {
	tests := []struct {
		name    string
		message string
		err     []error
		wantMsg string
	}{
		{
			name:    "message only",
			message: "service unavailable",
			err:     nil,
			wantMsg: "service unavailable",
		},
		{
			name:    "message with single error",
			message: "service unavailable",
			err:     []error{errors.New("connection timeout")},
			wantMsg: "service unavailable: connection timeout",
		},
		{
			name:    "message with multiple errors",
			message: "service unavailable",
			err:     []error{errors.New("error 1"), errors.New("error 2")},
			wantMsg: "service unavailable: error 1\nerror 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			su := NewServiceUnavailable(tt.message, tt.err...)
			assert.Equal(t, tt.wantMsg, su.Error())
		})
	}
}

func TestNewNotImplemented(t *testing.T) {
	tests := []struct {
		name    string
		message string
		err     []error
		wantMsg string
	}{
		{
			name:    "message only",
			message: "endpoint not implemented",
			err:     nil,
			wantMsg: "endpoint not implemented",
		},
		{
			name:    "message with single error",
			message: "endpoint not implemented",
			err:     []error{errors.New("feature in development")},
			wantMsg: "endpoint not implemented: feature in development",
		},
		{
			name:    "message with multiple errors",
			message: "endpoint not implemented",
			err:     []error{errors.New("error 1"), errors.New("error 2")},
			wantMsg: "endpoint not implemented: error 1\nerror 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ni := NewNotImplemented(tt.message, tt.err...)
			assert.Equal(t, tt.wantMsg, ni.Error())
		})
	}
}
