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
    export NATS_URL=nats://nats.lfx.svc.cluster.local:4222
    ```

- [NATS key-value bucket](https://docs.nats.io/nats-concepts/jetstream/key-value-store): once you have a NATS service running, you need to create a bucket used by the project service.

    ```bash
    # if using the nats cli tool
    nats kv add projects
    ```

#### 3. Export environment variables

|Environment Variable Name|Description|Default|Required|
|-----------------------|--------------------|-----------|-----|
|PORT|the port for http requests to the project service API|8080|false|
|NATS_URL|the URL of the nats server instance|nats://localhost:4222|false|
|LFX_ENVIRONMENT|the LFX environment (enum: prod, stg, dev)|dev|false|
|LOG_LEVEL|the log level for outputted logs|info|false|
|LOG_ADD_SOURCE|whether to add the source field to outputted logs|false|false|

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
   # -d	         enable debug logging (default false)
   # -p    string   listen port (default "8080")
   go run
   ```
5. **Run tests**:
   ```bash
   make test

   # or run go test to set custom flags
   go test . -v
   ```

6. **Docker build + K8**

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

### Add new API endpoints
Note: follow the [Development Workflow](#4-development-workflow) section on how to run the service code
1. **Update design files**: Edit project file in `design/` to include specicification of the new endpoint with all of its supported parameters, responses, and errors, etc.
2. **Regenerate code**: Run `make apigen` after design changes
3. **Implement code**: Implement the new endpoint in `service_endpoint.go`. Follow similar standards of the other endpoint methods. Include tests for the new endpoint in `service_endpoint_test.go`.
4. **Update heimdall ruleset**: Ensure that `/charts/lfx-v2-project-service/templates/ruleset.yaml` has the route and method for the endpoint set so that authentication is configured when deployed

### Add new message handlers
Note: follow the [Development Workflow](#4-development-workflow) section on how to run the service code
1. **Update main.go**: In `main.go` is the code for subscribing the service to specific NATS queue subjects. Add the subscription code in the `createNatsSubcriptions` function. If a new subject needs to be subscribed, add the subject to the `../pkg/constants` directory in a similiar fashion as the other subject names (so that it can be referenced by other services that need to send messages for the subject).
2. **Update service_handler.go**: Implement the NATS message handler. Add a new function, such as `HandleProjectGetName` for handling messages with respect to getting the name of a project. The `HandleNatsMessage` function switch statement should also be updated to include the new subject and function call.
3. **Update service_handler_test.go**: Add unit tests for the new handler function. Mock external service calls so that the tests are modular.
