// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package constants defines shared constant values used across the service.
package constants

// NATS KV bucket constants shared across the NATS infrastructure layer.
const (
	// ProjectIDMapLookupSubject is the NATS request/reply subject for resolving
	// a v2 project UID to a Salesforce Project__c.Id. The member service handles
	// this subject.
	ProjectIDMapLookupSubject = "lfx.member.project-id-map.lookup"

	// B2BOrgIDMapLookupSubject is the NATS request/reply subject for resolving
	// a Salesforce Account SFID to its v2 b2b_org UUID. The member service
	// handles this subject. Resolution is deterministic (no I/O required).
	B2BOrgIDMapLookupSubject = "lfx.member.b2b-org-id-map.lookup"
)
