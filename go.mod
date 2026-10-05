// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT
module github.com/linuxfoundation/lfx-v2-project-service

go 1.25.0

require (
	github.com/auth0/go-jwt-middleware/v2 v2.3.0
	github.com/aws/aws-sdk-go-v2/config v1.32.0
	github.com/aws/aws-sdk-go-v2/feature/s3/manager v1.20.10
	github.com/aws/aws-sdk-go-v2/service/s3 v1.97.3
	github.com/go-chi/chi/v5 v5.2.4
	github.com/go-viper/mapstructure/v2 v2.4.0
	github.com/google/uuid v1.6.0
	github.com/linuxfoundation/lfx-v2-email-service v0.1.0
	github.com/linuxfoundation/lfx-v2-fga-sync v0.2.17
	github.com/linuxfoundation/lfx-v2-indexer-service v0.4.14-0.20260109191409-7371e293d8b5
	github.com/linuxfoundation/lfx-v2-invite-service v0.2.1
	github.com/nats-io/nats.go v1.49.0
	github.com/opensearch-project/opensearch-go/v2 v2.3.0
	github.com/remychantenay/slog-otel v1.3.4
	github.com/rustyoz/svg v0.0.0-20250705135709-8b1786137cb3
	github.com/stretchr/testify v1.11.1
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.65.0
	go.opentelemetry.io/contrib/propagators/jaeger v1.44.0
	go.opentelemetry.io/otel v1.45.0
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc v0.21.0
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp v0.21.0
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.45.0
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.45.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.45.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.45.0
	go.opentelemetry.io/otel/log v0.21.0
	go.opentelemetry.io/otel/sdk v1.45.0
	go.opentelemetry.io/otel/sdk/log v0.21.0
	go.opentelemetry.io/otel/sdk/metric v1.45.0
	go.opentelemetry.io/otel/trace v1.45.0
	goa.design/goa/v3 v3.22.6
	golang.org/x/sync v0.22.0
)

require (
	github.com/aws/aws-sdk-go-v2 v1.41.5 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.8 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.19.0 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.14 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.21 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.21 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.4 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.22 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.21 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.21 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.0.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.30.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.35.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.41.1 // indirect
	github.com/aws/smithy-go v1.24.2 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dimfeld/httppath v0.0.0-20170720192232-ee938bf73598 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/gohugoio/hashstructure v0.6.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.29.0 // indirect
	github.com/klauspost/compress v1.18.7 // indirect
	github.com/manveru/faker v0.0.0-20171103152722-9fbc68a78c4d // indirect
	github.com/nats-io/nkeys v0.4.15 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rustyoz/Mtransform v0.0.0-20250628105438-00796a985d0a // indirect
	github.com/rustyoz/genericlexer v0.0.0-20250522144106-d3cfee480384 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.45.0 // indirect
	go.opentelemetry.io/otel/metric v1.45.0 // indirect
	go.opentelemetry.io/proto/otlp v1.11.0 // indirect
	golang.org/x/crypto v0.55.0 // indirect
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	golang.org/x/tools v0.48.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260803160001-6ac0973c030d // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260803160001-6ac0973c030d // indirect
	google.golang.org/grpc v1.83.2 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/go-jose/go-jose.v2 v2.6.3 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
