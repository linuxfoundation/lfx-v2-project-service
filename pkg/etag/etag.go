// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package etag

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// LFXEtag produces a deterministic ETag for any JSON-serialisable value.
// The value is marshalled to JSON, SHA-256 hashed, and the first 20 bytes are
// encoded as base64url (no padding, no W/ prefix). The result is suitable for
// use in ETag and If-Match HTTP headers.
func LFXEtag(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("etag: marshal: %w", err)
	}
	sum := sha256.Sum256(b)
	return base64.RawURLEncoding.EncodeToString(sum[:20]), nil
}
