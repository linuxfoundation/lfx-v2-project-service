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

	// SFIDToUUIDLookupSubject is the NATS request/reply subject for resolving
	// a Salesforce SFID to its v2 UUID v8. The member service handles
	// this subject. Resolution is deterministic (no I/O required).
	SFIDToUUIDLookupSubject = "lfx.member.sfid-to-uuid.lookup"

	// UUIDToSFIDLookupSubject is the NATS request/reply subject for resolving
	// a v2 UUID v8 to its Salesforce SFID. The member service handles
	// this subject. Resolution is deterministic (no I/O required).
	UUIDToSFIDLookupSubject = "lfx.member.uuid-to-sfid.lookup"
)
