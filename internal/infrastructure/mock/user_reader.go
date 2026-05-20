// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package mock

import "context"

// MockUserReader provides a no-op mock implementation of port.UserReader
// for local development when REPOSITORY_SOURCE=mock.
type MockUserReader struct{}

// SubByEmail always returns an empty string. Satisfies port.UserReader for
// local development without auth-service.
func (m *MockUserReader) SubByEmail(_ context.Context, _ string) (string, error) {
	return "", nil
}
