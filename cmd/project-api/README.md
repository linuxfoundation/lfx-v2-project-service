# Project API

This directory contains the Project API service. The service does a couple of things:

- It serves HTTP requests via Traefik to perform CRUD operations on project data
- It listens on a NATS connection for messages from external services to also perform operations on project data

Applications with a BFF should use the REST API with HTTP requests to perform the needed operations on projects, while other resource API services should communicate with this service via NATS messages.

This service contains the following API endpoints:

- `/readyz`:
  - `GET`: checks that the service is able to take in inbound requests
- `/livez`:
  - `GET`: checks that the service is alive
- `/projects`
  - `GET`: fetch the list of projects (Note: this will be removed in favor of using the query service, once implemented)
  - `POST` create a new project
- `/projects/:id`
  - `GET`: fetch a project's base information by its UID
  - `PUT`: update a project's base information by its UID - only certain attributes can be updated, read the openapi spec for more details
  - `DELETE`: delete a project by its UID
- `/projects/:id/settings`
  - `GET`: fetch a project's settings information by its UID
  - `PUT`: update a project's settings by its UID

This service handles the following NATS subjects:

- `lfx.projects-api.get_name`: Get a project name from a given project UID
- `lfx.projects-api.get_slug`: Get a project slug from a given project UID
- `lfx.projects-api.slug_to_uid`: Get a project UID from a given project slug

## File Structure

```bash
├── design/                         # Goa design files
│   ├── project.go                  # Goa project service
│   └── types.go                    # Goa models
├── gen/                            # Goa generated implementation code (not committed)
├── main.go                         # Dependency injection and startup
├── service.go                      # ProjectsAPI implementation (presentation layer)
├── service_endpoint.go             # Health check endpoints implementation
└── service_endpoint_project.go     # Project REST API endpoints implementation

# Dependencies from internal/ packages:
# - internal/service/              # Business logic layer
# - internal/domain/               # Domain interfaces and models
# - internal/infrastructure/       # Infrastructure implementations (NATS, Auth)
```

## Architecture

This service follows clean architecture principles with clear separation of concerns:

### Layers

1. **Presentation Layer** (`cmd/project-api/`)
   - `ProjectsAPI` struct implements the Goa-generated service interface
   - HTTP endpoint handlers (`service_endpoint_project.go`)
   - Dependency injection and startup (`main.go`)

2. **Service Layer** (`internal/service/`)
   - `ProjectsService` contains core business logic
   - Message handlers (`project_handlers.go`)
   - Orchestrates operations between domain and infrastructure

3. **Domain Layer** (`internal/domain/`)
   - Domain models (`models/`)
   - Repository interfaces (`repository.go`)
   - Message handling interfaces (`message.go`)
   - Domain-specific errors and validation

4. **Infrastructure Layer** (`internal/infrastructure/`)
   - NATS repository implementation (`nats/repository.go`)
   - JWT authentication implementation (`auth/jwt.go`)
   - External service integrations

### Key Benefits

- **Database Independence**: Can switch from NATS to PostgreSQL without changing business logic
- **Testability**: Each layer can be tested in isolation using mocks
- **Maintainability**: Clear separation of concerns and dependency direction
- **Flexibility**: Easy to add new storage backends or external services

## Development

### Prerequisites

- [**Go**](https://go.dev/): the service is built with the Go programming language [[Install](https://go.dev/doc/install)]
- [**Kubernetes**](https://kubernetes.io/): used for deployment of resources [[Install](https://kubernetes.io/releases/download/)]
- [**Helm**](https://helm.sh/): used to manage kubernetes applications [[Install](https://helm.sh/docs/intro/install/)]
- [**NATS**](https://docs.nats.io/): used to communicate with other LFX V2 services [[Install](https://docs.nats.io/running-a-nats-service/introduction/installation)]
- [**GOA Framework**](https://goa.design/): used for API code generation

#### GOA Framework

Follow the [GOA installation guide](https://goa.design/docs/2-getting-started/1-installation/) to install GOA:

```bash
go install goa.design/goa/v3/cmd/goa@latest
```

Verify the installation:

```bash
goa version
```

### Building and Development

#### 1. Generate Code

The service uses GOA to generate API code from the design specification. Run the following command to generate all necessary code:

```bash
make apigen

# or directly run the "goa gen" command
goa gen github.com/linuxfoundation/lfx-v2-project-service/cmd/project-api/design
```

This command generates:

- HTTP server and client code
- OpenAPI specification
- Service interfaces and types
- Transport layer implementations

#### 2. Set up resources and external services

The service relies on some resources and external services being spun up prior to running this service.

- [NATS service](https://docs.nats.io/): ensure you have a NATS server instance running and set the `NATS_URL` environment variable with the URL of the server

    ```bash
    export NATS_URL=nats://lfx-platform-nats.lfx.svc.cluster.local:4222
    ```

- [NATS key-value bucket](https://docs.nats.io/nats-concepts/jetstream/key-value-store): once you have a NATS service running, you need to create a bucket used by the project service.

    ```bash
    # if using the nats cli tool
    nats kv add projects --history=20 --storage=file --max-value-size=10485760 --max-bucket-size=1073741824
    nats kv add project-settings --history=20 --storage=file --max-value-size=10485760 --max-bucket-size=1073741824
    ```

#### 3. Export environment variables

|Environment Variable Name|Description|Default|Required|
|-----------------------|--------------------|-----------|-----|
|PORT|the port for http requests to the project service API|8080|false|
|NATS_URL|the URL of the nats server instance|nats://localhost:4222|false|
|LOG_LEVEL|the log level for outputted logs|info|false|
|LOG_ADD_SOURCE|whether to add the source field to outputted logs|false|false|
|JWKS_URL|the URL to the endpoint for verifying ID tokens and JWT access tokens||false|
|AUDIENCE|the audience of the app that the JWT token should have set - for verification of the JWT token|lfx-v2-project-service|false|
|JWT_AUTH_DISABLED_MOCK_LOCAL_PRINCIPAL|a mocked auth principal value for local development (to avoid needing a valid JWT token)||false|

#### 4. Development Workflow

1. **Make design or implementation changes**: Edit files in the `design/` directory for design changes, and edit the other files for implementation changes.

2. **Regenerate code**: Run `make apigen` after design changes

3. **Build the service**:

   ```bash
   make build
   ```

4. **Run the service**:

   ```bash
   make run

   # or run with debug logs enabled
   make debug

   # or run with the go command to set custom flags
   # -bind string   interface to bind on (default "*")
   # -d          enable debug logging (default false)
   # -p    string   listen port (default "8080")
   go run
   ```

   Once the service is running, make a request to the `/livez` endpoint to ensure that the service is alive.

   ```bash
    curl http://localhost:8080/livez
   ```

   You should get a 200 status code response with a text/plain content payload of `OK`.

5. **Run tests**:

   ```bash
   make test

   # or run go test to set custom flags
   go test . -v
   ```

6. **Lint the code**

   ```bash
   # From the root of the directory, run megalinter (https://megalinter.io/latest/mega-linter-runner/) to ensure the code passes the linter checks. The CI/CD has a check that uses megalinter.
   npx mega-linter-runner .
   ```

7. **Docker build + K8**

    ```bash
    # Build the dockerfile (from the root of the repo)
    docker build -t lfx-v2-project-service:<release_number> .

    # Install the helm chart for the service into the lfx namespace (from the root of the repo)
    helm install lfx-v2-project-service ./charts/lfx-v2-project-service/ -n lfx

    # Once you have already installed the helm chart and need to just update it, use the following command (from the root of the repo):
    helm upgrade lfx-v2-project-service ./charts/lfx-v2-project-service/ -n lfx

    # Check that the REST API is accessible by hitting the `/livez` endpoint (you should get a response of OK if it is working):
    #
    # Note: replace the hostname with the host from ./charts/lfx-v2-project-service/ingressroute.yaml
    curl http://lfx-api.k8s.orb.local/livez
    ```

8. **Insert Root Project**

All projects that are created require a `parent_uid` to be set. This means that if there are no existing projects, it is impossible to create a new project. To resolve this, you can insert a root project when developing locally using [the root_project_setup tooling](../../scripts/root-project-setup/README.md).

This will create a root project with a randomly generated UID and a slug of `ROOT` which is expected to be used as the root of the project hierarchy.

The [helm chart](../../charts/lfx-v2-project-service) runs this automatically as an init container.

### Authorization with OpenFGA

When deployed via Kubernetes, the project service uses OpenFGA for fine-grained authorization control. The authorization is handled by Heimdall middleware before requests reach the service.

#### Configuration

OpenFGA authorization is controlled by the `openfga.enabled` value in the Helm chart:

```yaml
# In values.yaml or via --set flag
openfga:
  enabled: true  # Enable OpenFGA authorization (default)
  # enabled: false  # Disable for local development only
```

#### Authorization Rules

When OpenFGA is enabled, the following authorization checks are enforced:

- **GET /projects** - No OpenFGA check (returns list of all projects)
- **POST /projects** - Requires `writer` relation on the parent project (if parent_uid is specified)
- **GET /projects/:id** - Requires `viewer` relation on the specific project
- **GET /projects/:id/settings** - Requires `auditor` relation on the specific project
- **PUT /projects/:id** - Requires `writer` relation on the specific project
- **PUT /projects/:id/settings** - Requires `writer` relation on the specific project
- **DELETE /projects/:id** - Requires `owner` relation on the specific project

#### Local Development

For local development without OpenFGA:

1. Set `openfga.enabled: false` in your Helm values
2. All requests will be allowed through (after JWT authentication)
3. **Warning**: Never disable OpenFGA in production environments

### Add new API endpoints

Note: follow the [Development Workflow](#4-development-workflow) section on how to run the service code

1. **Update design files**: Edit project file in `design/` to include specification of the new endpoint with all of its supported parameters, responses, and errors, etc.
2. **Regenerate code**: Run `make apigen` after design changes
3. **Implement code**: Implement the new endpoint in `service_endpoint_project.go` (for project-related endpoints) or create a new `service_endpoint_*.go` file for other resource types. Follow similar standards of the other endpoint methods. Include tests for the new endpoint in the corresponding `*_test.go` file.
4. **Update heimdall ruleset**: Ensure that `/charts/lfx-v2-project-service/templates/ruleset.yaml` has the route and method for the endpoint set so that authentication is configured when deployed. If the endpoint modifies data (PUT, DELETE, PATCH), consider adding OpenFGA authorization checks in the ruleset for proper access control

### Add new message handlers

Note: follow the [Development Workflow](#4-development-workflow) section on how to run the service code

1. **Update main.go**: In `main.go` is the code for subscribing the service to specific NATS queue subjects. Add the subscription code in the `createNatsSubcriptions` function. If a new subject needs to be subscribed, add the subject to the `../pkg/constants` directory in a similiar fashion as the other subject names (so that it can be referenced by other services that need to send messages for the subject).
2. **Update project_handlers.go**: Implement the NATS message handler in `internal/service/project_handlers.go`. Add a new function, such as `HandleProjectGetName` for handling messages with respect to getting the name of a project. The `HandleNatsMessage` function switch statement should also be updated to include the new subject and function call.
3. **Update project_handlers_test.go**: Add unit tests for the new handler function in `internal/service/project_handlers_test.go`. Mock external service calls so that the tests are modular.
