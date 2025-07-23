// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package nats contains the models for the NATS messaging.
package nats

// TransactionAction is a type for the action of a project transaction.
type TransactionAction string

// TransactionAction constants for the action of a project transaction.
const (
	// ActionCreated is the action for a resource creation transaction.
	ActionCreated TransactionAction = "created"
	// ActionUpdated is the action for a resource update transaction.
	ActionUpdated TransactionAction = "updated"
	// ActionDeleted is the action for a resource deletion transaction.
	ActionDeleted TransactionAction = "deleted"
)

// ProjectTransaction is a NATS message schema for sending messages related to projects CRUD operations.
type ProjectTransaction struct {
	Action  TransactionAction `json:"action"`
	Headers map[string]string `json:"headers"`
	Data    any               `json:"data"`
}
