# LFX V2 Project Service

This repository contains the source code for the LFX v2 platform project service.

## Overview

The LFX v2 Project Service is a RESTful API service that manages projects within the Linux Foundation's LFX platform. It provides endpoints for creating, reading, updating, and deleting projects with built-in authorization and audit capabilities.

### API Endpoints

- `/readyz`: `GET` - checks that the service is able to take in inbound requests
- `/livez`: `GET` - checks that the service is alive
- `/projects`:
  - `GET` - fetch the list of projects (Note: this will be removed in favor of using the query service, once implemented)
  - `POST` - create a new project
- `/projects/:id`:
  - `GET` - fetch a project's base information by its UID
  - `PUT` - update a project's base information by its UID - only certain attributes can be updated, read the openapi spec for more details
  - `DELETE` - delete a project by its UID
- `/projects/:id/settings`:
  - `GET` - fetch a project's settings information by its UID
  - `PUT` - update a project's settings by its UID

### NATS Message Handlers

This service handles the following NATS subjects for inter-service communication:

- `lfx.projects-api.get_name`: Get a project name from a given project UID
- `lfx.projects-api.get_slug`: Get a project slug from a given project UID
- `lfx.projects-api.get_logo`: Get a project logo URL from a given project UID
- `lfx.projects-api.get_parent_uid`: Get a project's parent UID from a given project UID
- `lfx.projects-api.slug_to_uid`: Get a project UID from a given project slug

### NATS Events Published

This service publishes the following NATS events:

- `lfx.projects-api.project_settings.updated`: Published when project settings are updated. Contains both the old and new settings to allow downstream services to react to changes. Message format:

  ```json
  {
    "project_uid": "string",
    "old_settings": { /* ProjectSettings object */ },
    "new_settings": { /* ProjectSettings object */ }
  }
  ```

### Project Tags

The LFX v2 Project Service generates tags for projects that are sent to the indexer-service.

#### Tags Generated for Projects

When projects are created or updated, the following tag is generated:

| Project Field | Tag Format | Example | Purpose |
|--------------|-----------|---------|---------|
| Slug | `project_slug:<value>` | `project_slug:test-project` | Namespaced lookup by slug |

**Note**: Additional project metadata (UID, ParentUID, Name, Description, etc.) is sent via the `IndexingConfig` field.

#### Tags Generated for Project Settings

Project settings generate no tags. All metadata is sent via the `IndexingConfig` field.

## Quick Start

### Pre-requisites

- Kubernetes
- Helm

### Setup

1. Install the `lfx-platform` helm chart from [lfx-v2-helm repo](<https://github.com/linuxfoundation/lfx-v2-helm>). This is a general helm chart that is used for all LFX platform services. It contains all of the dependencies packaged in kubernetes that are needed by the platform: NATS, Heimdall, Authelia, Traefik, etc..

   Either read the official [instructions](https://github.com/linuxfoundation/lfx-v2-helm/tree/main/charts/lfx-platform) from the repo containing the chart, or run the commands below:

   ```bash
   # Create namespace (recommended). You should use this for all LFX services. You may already have the namespace created if you have worked on another LFX service. In that case, you can proceed to the next command.
   kubectl create namespace lfx

   # Install the chart via the OCI registry.
   # Note: change the version to use the latest (or desired) chart version according to the releases for the lfx-platform chart: https://github.com/linuxfoundation/lfx-v2-helm/pkgs/container/lfx-v2-helm%2Fchart%2Flfx-platform
   helm install -n lfx lfx-platform \
   oci://ghcr.io/linuxfoundation/lfx-v2-helm/chart/lfx-platform \
   --version 0.1.12
   ```

2. Install the `lfx-v2-project-service` helm chart from this current repository. You have two options: either install from the OCI registry or from the source. If you don't plan to develop the service, you can just use the packaged version from the [github packages](https://github.com/linuxfoundation/lfx-v2-project-service/pkgs/container/lfx-v2-project-service%2Fchart%2Flfx-v2-project-service). <!-- markdown-link-check-disable-line -->

   ```bash
   # From OCI registry
   # Note: check the latest (or desired) version from https://github.com/linuxfoundation/lfx-v2-project-service/pkgs/container/lfx-v2-project-service%2Fchart%2Flfx-v2-project-service
   helm install -n lfx lfx-v2-project-service \
   oci://ghcr.io/linuxfoundation/lfx-v2-project-service/chart/lfx-v2-project-service \
   --version 0.4.0

   # From source (current local directory)
   helm install -n lfx lfx-v2-project-service ./charts/lfx-v2-project-service
   ```

3. After installing the required helm charts, you should have the project REST API running on your machine in kubernetes, and can therefore start making some requests to the API.

### Making requests to the API

1. Get an ID token from the Authelia IdP server.

   In order to make a request to the service via Traefik, you need to be making an authenticated request as a valid Authelia user. If you have the lfx-platform chart installed from the previous steps, then you can use the `kubectl` CLI tool to get the list of users that you can use for authentication. They are stored in kubernetes as a secret resource.

   ```bash
   kubectl get secret authelia-users -n lfx -o json
   ```

   The list of users in Authelia are set by the lfx-platform chart to help for testing basic scenarios. You can find the users and how they are set up in Authelia from [lfx-v2-helm repo chart](https://github.com/linuxfoundation/lfx-v2-helm/tree/main/charts/lfx-platform) Currently, the list is as follows:

   ```text
   committee_member_1
   committee_member_2
   project_admin_1
   project_admin_2
   project_super_admin
   ```

   Currently, you should use the existing [token helper script](https://github.com/linuxfoundation/lfx-architecture-scratch/tree/main/2024-12%20ReBAC%20Demo/token_helper) to generate the ID token. The script is only accessible if you are LF staff. The team has a TODO in order to include the helper script in a public repo or come up with a better solution for generating ID tokens for local testing. <!-- markdown-link-check-disable-line -->

   If you have access to the token helper script, run the following command to get the ID token. Note that you will be prompted in your web browser to log in as one of the valid Authelia users. Use the kubernetes secret `authelia-users` as previously mentioned to determine the password for each user. Use the username and password for the user you want to authenticate with.

   ```bash
   id_token=$(./token_helper.py); echo $id_token
   ```

2. Use the ID token in the Authorization Header to make a request to the project service

   You can find documentation about the list of API endpoints supported by the service by looking at the [OpenAPI specification file](api/project/v1/gen/http/openapi3.yaml)

   For now, try to make a request to list the projects:

   ```bash
   curl -H "Authorization: Bearer $id_token" http://lfx-api.k8s.orb.local/projects
   ```

   You should get a response as follows. Running the app container via the lfx-v2-project-service Helm chart should run an init container that creates a root project. The UID will be a random UUID, but the slug, description, and other fields should be the same.

   ```json
   {
   "projects": [
      {
         "uid": "81570bff-3267-4942-80f3-d469437a46d6",
         "slug": "ROOT",
         "description": "A root project for teams permissions assignment, ordinarily hidden from users.",
         "name": "ROOT",
         "public": false,
         "autojoin_enabled": false,
         "created_at": "2025-07-31T00:41:54Z",
         "updated_at": "2025-07-31T00:41:54Z",
         "mission_statement": "A root project for teams permissions assignment, ordinarily hidden from users."
      }
   ]
   }
   ```

   If you get a `403 Forbidden` error, then you need to check that the ID token you are passing to the project service is valid and not expired. Once you have an ID token, you can check its expiration and other user metadata on the token using this auth server API call:

   ```bash
   curl -s https://auth.k8s.orb.local/api/oidc/userinfo \
      -H "Authorization: Bearer $id_token" |
      jq -c .
   ```

   Next, try to create a project:

   ```bash
   curl -X POST http://lfx-api.k8s.orb.local/projects \
      -H "Authorization: Bearer $id_token" \
      -H "Content-Type: application/json" \
      -d '{
         "name": "My Test Project",
         "slug": "my-test-project",
         "description": "A test project created via API",
         "parent_uid": "<ROOT_PROJECT_UID_GOES_HERE>",
         "public": false,
         "autojoin_enabled": false
      }'
   ```

   You should get a response like:

   ```json
   {
   "uid": "7bdc6e40-8cc8-4536-b537-e6cd31ce058d",
   "slug": "my-test-project",
   "description": "A test project created via API",
   "name": "My Test Project",
   "public": false,
   "parent_uid": "81570bff-3267-4942-80f3-d469437a46d6",
   "autojoin_enabled": false,
   "created_at": "2025-08-12T19:43:24Z",
   "updated_at": "2025-08-12T19:43:24Z"
   }
   ```

   Then try to get the newly created project:

   ```bash
   curl -H "Authorization: Bearer $id_token" http://lfx-api.k8s.orb.local/projects/<NEW_PROJECT_UID_GOES_HERE>
   ```

   You should get a response just like the POST project endpoint:

   ```json
   {
   "uid": "7bdc6e40-8cc8-4536-b537-e6cd31ce058d",
   "slug": "my-test-project",
   "description": "A test project created via API",
   "name": "My Test Project",
   "public": false,
   "parent_uid": "81570bff-3267-4942-80f3-d469437a46d6",
   "autojoin_enabled": false,
   "created_at": "2025-08-12T19:43:24Z",
   "updated_at": "2025-08-12T19:43:24Z"
   }
   ```

## File Structure

```bash
├── .github/                        # Github files
│   └── workflows/                  # Github Action workflow files
├── api/                            # API contracts and specifications
│   └── project/                    # Project service API
│       └── v1/                     # API version 1
│           ├── design/             # Goa API design specifications
│           └── gen/                # Generated code from Goa design
├── charts/                         # Helm charts for running the service in kubernetes
├── cmd/                            # Services (main packages)
│   └── project-api/                # Project service API entry point
├── internal/                       # Internal service packages
│   ├── domain/                     # Domain logic layer (business logic)
│   │   └── models/                 # Domain models and entities
│   ├── service/                    # Service logic layer (service implementations)
│   ├── infrastructure/             # Infrastructure layer
│   │   ├── auth/                   # Authentication abstractions
│   │   └── nats/                   # NATS messaging and repository implementation
│   ├── middleware/                 # HTTP middleware components
│   └── log/                        # Logging utilities
└── pkg/                            # Shared packages
    └── constants/                  # Shared constants and configurations
```

## Development

To contribute to this repository:

1. Fork the repository
2. Commit your changes to a feature branch in your fork. Ensure your commits
   are signed with the [Developer Certificate of Origin
   (DCO)](https://developercertificate.org/).
   You can use the `git commit -s` command to sign your commits.
3. Ensure the chart version in `charts/lfx-v2-project-service/Chart.yaml` has been
   updated following semantic version conventions if you are making changes to the chart.
4. Submit your pull request

For more details about development on this repository, read the [DEVELOPMENT.md](DEVELOPMENT.md).

## Releases

### Creating a Release

To create a new release of the project service:

1. **Update the chart version** in `charts/lfx-v2-project-service/Chart.yaml` prior to any project releases, or if any
   change is made to the chart manifests or configuration:

   ```yaml
   version: 0.2.0  # Increment this version
   appVersion: "latest"  # Keep this as "latest"
   ```

2. **After the pull request is merged**, create a GitHub release and choose the
   option for GitHub to also tag the repository. The tag must follow the format
   `v{version}` (e.g., `v0.2.0`). The tag version used will be the same as the chart version and app version for the helm chart.

3. **The GitHub Actions workflow will automatically**:
   - Build and publish the container images (project-api and root-project-setup)
   - Package and publish the Helm chart to GitHub Pages
   - Publish the chart to GitHub Container Registry (GHCR)
   - Sign the chart with Cosign
   - Generate SLSA provenance

### Important Notes

- The `appVersion` in `Chart.yaml` should always remain `"latest"` in the committed code.
- During the release process, the `ko-build-tag.yaml` workflow automatically overrides the `appVersion` and `version` with the actual tag version (e.g., `v0.2.0` becomes `0.2.0`).
- The container image tags are automatically managed by the consolidated CI/CD pipeline using the git tag.
- Both container images (project-api and root-project-setup) and the Helm chart are published together in a single workflow.

## License

Copyright The Linux Foundation and each contributor to LFX.

This project’s source code is licensed under the MIT License. A copy of the
license is available in `LICENSE`.

This project's documentation is licensed under the Creative Commons Attribution
4.0 International License \(CC-BY-4.0\). A copy of the license is available in
`LICENSE-docs`.
