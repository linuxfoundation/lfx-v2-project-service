// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package proto contains the generated gRPC client stubs for the Salesforce
// Pub/Sub API (PubSub API v1).
//
// # Why these files exist
//
// These are the gRPC wire-protocol types needed to talk to Salesforce's
// PubSub gRPC endpoint. There is no REST alternative — Salesforce exposes
// real-time CDC (Change Data Capture) exclusively over gRPC. Without these
// stubs you cannot call Subscribe (receive CDC events) or GetSchema (fetch
// the Avro schema used to decode event payloads).
//
// # What we use
//
//   - PubSubClient / NewPubSubClient — gRPC client constructor
//   - FetchRequest / FetchResponse / ConsumerEvent — Subscribe stream messages
//   - SchemaRequest / SchemaInfo — GetSchema RPC (Avro schema lookup by ID)
//
// Everything else in the proto (Publish, TopicInfo, etc.) is unused by this
// service.
//
// # How the files were generated (rare — only needed on proto update)
//
//	make protoc-install   # download protoc binary once (no brew/root required)
//	make protoc-gen       # regenerate from api/salesforce/pubsub/pubsub_api.proto
//
// The generated *.pb.go files are committed to git so that normal builds and
// CI never require protoc. Only re-run protoc-gen when
// api/salesforce/pubsub/pubsub_api.proto itself changes (e.g. a new Salesforce
// API version). See README.md § "Regenerating Salesforce Pub/Sub gRPC stubs".
//
// Source proto: api/salesforce/pubsub/pubsub_api.proto
// Upstream:     https://github.com/forcedotcom/pub-sub-api
package proto
