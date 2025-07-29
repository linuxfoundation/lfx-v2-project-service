# LFX V2 Project Service

This repository contains the source code for the LFX v2 platform project service.

## Overview

The LFX v2 Project Service is a RESTful API service that manages projects within the Linux Foundation's LFX platform. It provides endpoints for creating, reading, updating, and deleting projects with built-in authorization and audit capabilities.

## File Structure

```bash
├── .github/                        # Github files
│   └── workflows/                  # Github Action workflow files
├── charts/                         # Helm charts for running the service in kubernetes
├── cmd/                            # Services (main packages)
│   └── project-api/                # Project service code
│       ├── gen/                    # Generated code from Goa design
│       └── design/                 # API design specifications
├── internal/                       # Internal service packages
│   ├── domain/                     # Domain logic layer
│   ├── service/                    # Business logic layer
│   ├── middleware/                 # HTTP middleware components
│   └── infrastructure/             # Infrastructure layer
│       └── nats/                   # NATS messaging infrastructure
└── pkg/                            # Shared packages
    └── constants/                  # Shared constants and configurations
```

## Key Features

- **RESTful API**: Full CRUD operations for project management
- **NATS Integration**: Event-driven architecture using NATS for messaging and key-value storage
- **Authorization**: JWT-based authentication with Heimdall middleware integration
- **OpenFGA Support**: Fine-grained authorization control (configurable)
- **Health Checks**: Built-in `/livez` and `/readyz` endpoints
- **Request Tracking**: Automatic request ID generation and propagation
- **Structured Logging**: JSON-formatted logs with contextual information

## Releases

### Creating a Release

To create a new release of the project service:

1. **Update the chart version** in `charts/lfx-v2-project-service/Chart.yaml` prior to any project releases, or if any
   change is made to the chart manifests or configuration:
   ```yaml
   version: 0.2.0  # Increment this version
   appVersion: "development"  # Keep this as "development"
   ```

2. **After the pull request is merged**, create a GitHub release and choose the
   option for GitHub to also tag the repository. The tag must follow the format
   `v{version}` (e.g., `v0.2.0`). This tag does _not_ have to match the chart
   version: it is the version for the project release, which will dynamically
   update the `appVersion` in the released chart.

3. **The GitHub Actions workflow will automatically**:
   - Build and publish the container images (project-api and root-project-setup)
   - Package and publish the Helm chart to GitHub Pages
   - Publish the chart to GitHub Container Registry (GHCR)
   - Sign the chart with Cosign
   - Generate SLSA provenance

### Important Notes

- The `appVersion` in `Chart.yaml` should always remain `"development"` in the committed code.
- During the release process, the `ko-build-tag.yaml` workflow automatically overrides the `appVersion` with the actual tag version (e.g., `v0.2.0` becomes `0.2.0`).
- Only update the chart `version` field when making releases - this represents the Helm chart version.
- The container image tags are automatically managed by the consolidated CI/CD pipeline using the git tag.
- Both container images (project-api and root-project-setup) and the Helm chart are published together in a single workflow.

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

## License

Copyright The Linux Foundation and each contributor to LFX.

This project’s source code is licensed under the MIT License. A copy of the
license is available in `LICENSE`.

This project’s documentation is licensed under the Creative Commons Attribution
4.0 International License \(CC-BY-4.0\). A copy of the license is available in
`LICENSE-docs`.
