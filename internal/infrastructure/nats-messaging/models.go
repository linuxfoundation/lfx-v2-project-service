// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package natsmessaging

// TransactionAction is a type for the action of a project transaction.
type TransactionAction string

// TransactionAction constants for the action of a project transaction.
const (
	ActionCreated TransactionAction = "created"
	ActionUpdated TransactionAction = "updated"
	ActionDeleted TransactionAction = "deleted"
)

// ProjectTransaction is a NATS message schema for sending messages related to projects CRUD operations.
type ProjectTransaction struct {
	Action  TransactionAction `json:"action"`
	Headers map[string]string `json:"headers"`
	Data    any               `json:"data"`
}
