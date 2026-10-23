// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"errors"
	"strings"

	"github.com/nats-io/nats.go/jetstream"
)

// isJetStreamCASConflict reports whether err is a JetStream optimistic-concurrency
// failure: the supplied revision no longer matches the key's last sequence, or a
// conditional create found the key already present.
func isJetStreamCASConflict(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, jetstream.ErrKeyExists) {
		return true
	}
	return strings.Contains(err.Error(), "wrong last sequence")
}

type NatsRepository struct {
	Projects        INatsKeyValue
	ProjectSettings INatsKeyValue
	Links           INatsKeyValue
	Folders         INatsKeyValue
	Documents       INatsKeyValue
	DocumentFiles   INatsObjectStore
}
