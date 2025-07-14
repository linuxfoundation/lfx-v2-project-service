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
    - `GET`: fetch a project by its UID
    - `PUT`: update a project by its UID - only certain attributes can be updated, read the openapi spec for more details
    - `DELETE`: delete a project by its UID

This service handles the following NATS subjects:
- `<lfx_environment>.lfx.projects-api.get_name`: Get a project name from a given project UID
- `<lfx_environment>.lfx.projects-api.slug_to_uid`: Get a project UID from a given project slug

## File Structure

```bash
├── design/                         # Goa design files
│   ├── project.go                  # Goa project service
│   └── types.go                    # Goa models
├── gen/                            # Goa generated implementation code (not committed)
├── main.go                         # Dependency injection and startup
├── service.go                      # Base service implementation
├── service_endpoint.go             # Service implementation of REST API endpoints
├── service_handler.go              # Service implementation of NATS handlers
├── repo.go                         # Interface with data stores
├── mock.go                         # Service mocks for tests
└── jwt.go                          # API authentication with Heimdall
```

## Development

### Prerequisites

This service uses the [GOA Framework](https://goa.design/) for API generation. You'll need to install GOA before building the service.

#### Installing GOA Framework

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

#### 2. Development Workflow

1. **Make design or implementation changes**: Edit files in the `design/` directory for design changes, and edit the other files for implementation changes.
2. **Regenerate code**: Run `make apigen` after design changes
3. **Build the service**:
   ```bash
   make build
   ```
4. **Run the service** (for development):
   ```bash
   make run

   # enable debug logging
   make debug
   ```
5. **Run tests**:
   ```bash
   make test
   ```

### Add new API endpoints
Note: follow the [Development Workflow](#2-development-workflow) section on how to run the service code
1. **Update design files**: Edit project file in `design/` to include specicification of the new endpoint with all of its supported parameters, responses, and errors, etc.
2. **Regenerate code**: Run `make apigen` after design changes
3. **Implement code**: Implement the new endpoint in `service_endpoint.go`. Follow similar standards of the other endpoint methods. Include tests for the new endpoint in `service_endpoint_test.go`.

### Add new message handlers
Note: follow the [Development Workflow](#2-development-workflow) section on how to run the service code
1. **Update main.go**: In `main.go` is the code for subscribing the service to specific NATS queue subjects. Add the subscription code in the `createNatsSubcriptions` function. If a new subject needs to be subscribed, add the subject to the `../pkg/constants` directory in a similiar fashion as the other subject names (so that it can be referenced by other services that need to send messages for the subject).
2. **Update service_handler.go**: Implement the NATS message handler. Add a new function, such as `HandleProjectGetName` for handling messages with respect to getting the name of a project. The `HandleNatsMessage` function switch statement should also be updated to include the new subject and function call.
3. **Update service_handler_test.go**: Add unit tests for the new handler function. Mock external service calls so that the tests are modular.
