# LFX V2 Member Service

This repository contains the source code for the LFX v2 platform member service.

## Overview

The LFX v2 Member Service is a RESTful API service that provides membership data
within the Linux Foundation's LFX platform. It exposes endpoints for querying
tiers, memberships, and their associated key contacts, organised around projects,
as well as write endpoints for managing key contacts. Data is sourced directly
from Salesforce via SOQL queries, with a per-record NATS Key-Value cache to
minimise round-trips.

## File Structure

```bash
├── .github/                        # Github files
│   └── workflows/                  # Github Action workflow files
├── charts/                         # Helm charts for running the service in kubernetes
├── cmd/                            # Services (main packages)
│   └── member-api/                 # Member service API code
│       ├── design/                 # API design specifications (Goa)
│       ├── service/                # Service implementation
│       ├── main.go                 # Application entry point
│       └── http.go                 # HTTP server setup
├── gen/                            # Generated code from Goa design
├── internal/                       # Internal service packages
│   ├── domain/                     # Domain logic layer (business logic)
│   │   ├── model/                  # Domain models and entities
│   │   └── port/                   # Repository and service interfaces
│   ├── service/                    # Service logic layer (use cases)
│   ├── infrastructure/             # Infrastructure layer
│   │   ├── nats/                   # NATS KV cache and project RPC
│   │   ├── salesforce/             # Salesforce SOQL client and repositories
│   │   ├── project/                # ProjectResolver (UID ↔ slug ↔ SFID)
│   │   └── mock/                   # Mock implementations for testing
│   └── middleware/                 # HTTP middleware components
└── pkg/                            # Shared packages
```

## Key Features

- **RESTful API**: Project-scoped endpoints for querying tiers, memberships, and
  key contacts, plus write endpoints (POST/PUT/DELETE) for key contact management.
- **Salesforce-backed**: All membership data is fetched directly from Salesforce
  via SOQL queries; no PostgreSQL dependency at runtime.
- **NATS KV cache**: Per-record caching in the `membership-cache` bucket with
  soft-TTL stale-while-revalidate semantics (6 h stale / 23 h expire / 24 h
  NATS bucket TTL).
- **Project ID resolution**: The `ProjectResolver` translates v2 project UIDs to
  Salesforce `Project__c.Id` values by chaining a NATS RPC to the project-service
  and a SOQL lookup, backed by the same KV cache.
- **NATS RPC endpoint**: Exposes a request/reply subject so other services can
  resolve a v2 project UID to a Salesforce `Project__c.Id` without querying
  Salesforce or PostgreSQL directly.
- **Clean Architecture**: Follows hexagonal architecture with clear separation of
  domain, service, and infrastructure layers.
- **Authorization**: JWT-based authentication with Heimdall middleware integration
  and OpenFGA fine-grained access control.
- **Health Checks**: Built-in `/livez` and `/readyz` endpoints.
- **Request Tracking**: Automatic request ID generation and propagation.
- **Structured Logging**: JSON-formatted logs with contextual information.

## API Endpoints

The API is project-scoped. All data endpoints are nested under
`/projects/{project_id}` where `project_id` is the v2 project UUID.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/projects/{project_id}/tiers` | List membership tiers for a project |
| GET | `/projects/{project_id}/tiers/{tier_id}` | Get a specific membership tier |
| GET | `/projects/{project_id}/memberships` | List memberships for a project |
| GET | `/projects/{project_id}/memberships/{id}` | Get a specific membership |
| GET | `/projects/{project_id}/memberships/{id}/key_contacts` | List key contacts for a membership |
| GET | `/projects/{project_id}/memberships/{id}/key_contacts/{cid}` | Get a specific key contact |
| POST | `/projects/{project_id}/memberships/{id}/key_contacts` | Add a key contact |
| PUT | `/projects/{project_id}/memberships/{id}/key_contacts/{cid}` | Update a key contact |
| DELETE | `/projects/{project_id}/memberships/{id}/key_contacts/{cid}` | Remove a key contact |
| GET | `/readyz` | Readiness check |
| GET | `/livez` | Liveness check |

> **Note:** The legacy `/members/*` and `/memberships/*` endpoints return
> `410 Gone` with a hint pointing to the replacement paths above.

### Filtering

Use the `filter` query parameter with semicolon-separated `key=value` pairs:

```
GET /projects/{project_id}/memberships?filter=status=Active
GET /projects/{project_id}/memberships?filter=status=Active;tier=Gold
```

Supported filter keys: `status`, `tier`, `membership_type`, `year`,
`product_name`.

## NATS API

In addition to the HTTP API, the service handles a NATS request/reply subject
that allows other services to resolve v2 project UIDs to Salesforce
`Project__c.Id` values.

### Project ID Map Lookup

Resolves a v2 project UUID to its corresponding Salesforce `Project__c.Id`.
Resolution is cached in the `membership-cache` KV bucket; a cache miss triggers
a NATS RPC to the project-service (to obtain the project slug) followed by a
Salesforce SOQL query.

| Field | Value |
|-------|-------|
| **Subject** | `lfx.member.project-id-map.lookup` |
| **Transport** | NATS core request/reply |

**Request body (JSON):**

```json
{"project_uid": "<v2 project UUID>"}
```

**Response — success (HTTP 200 equivalent):**

```json
{"project_sfid": "<Salesforce Project__c.Id>"}
```

**Response — not found:**

```json
{"error": "project not found"}
```

**Response — bad request:**

```json
{"error": "project_uid is required"}
```

The reply is always valid JSON. Check for the presence of the `"error"` key to
detect failure.

## Development

To contribute to this repository:

1. Fork the repository.
2. Commit your changes to a feature branch in your fork. Ensure your commits
   are signed with the [Developer Certificate of Origin
   (DCO)](https://developercertificate.org/).
   You can use the `git commit -s` command to sign your commits.
3. Submit your pull request.

### Building

```bash
make apigen    # Generate Goa API code
make build     # Build the member-api binary
make test      # Run tests
```

### Running Locally

```bash
# With NATS and Salesforce credentials
export NATS_URL=nats://localhost:4222
export SF_INSTANCE_URL=https://linuxfoundation.my.salesforce.com
export SF_CLIENT_ID=<connected-app-client-id>
export SF_CLIENT_SECRET=<connected-app-client-secret>
./bin/member-api

# With mock data (no NATS or Salesforce required)
REPOSITORY_SOURCE=mock AUTH_SOURCE=mock ./bin/member-api

# With mock auth only (NATS + Salesforce still required)
export JWT_AUTH_DISABLED_MOCK_LOCAL_PRINCIPAL=local-dev-user
./bin/member-api
```

### Salesforce Credentials

The service reads credentials from environment variables. A pre-existing
Kubernetes Secret is required in production; see the Helm chart `values.yaml`
`salesforce.secrets` stanza for configuration.

| Variable | Description | Required |
|----------|-------------|----------|
| `SF_INSTANCE_URL` | Salesforce instance URL | Yes |
| `SF_CLIENT_ID` | Connected-app consumer key | Yes |
| `SF_CLIENT_SECRET` | Consumer secret (username/password or client-credentials flow) | Conditional |
| `SF_USERNAME` | Salesforce username (username/password or JWT flow) | Conditional |
| `SF_PASSWORD` | Salesforce password (username/password flow) | Conditional |
| `SF_SECURITY_TOKEN` | Security token appended to password | No |
| `SF_CONSUMER_RSA_PEM` | PEM-encoded RSA private key (JWT bearer flow) | Conditional |
| `SF_API_VERSION` | Salesforce REST API version (default: `v60.0`) | No |

At least one complete authentication flow must be configured:

- **JWT bearer**: `SF_USERNAME` + `SF_CONSUMER_RSA_PEM`
- **Username/password**: `SF_USERNAME` + `SF_PASSWORD` + `SF_CLIENT_SECRET`
- **Client-credentials**: `SF_CLIENT_SECRET` (without `SF_USERNAME`)

## License

Copyright The Linux Foundation and each contributor to LFX.

This project's source code is licensed under the MIT License. A copy of the
license is available in `LICENSE`.

This project's documentation is licensed under the Creative Commons Attribution
4.0 International License (CC-BY-4.0). A copy of the license is available in
`LICENSE-docs`.