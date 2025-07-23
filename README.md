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

## Contributing

To contribute to this repository:

1. Fork the repository
2. Make your changes
3. Submit a pull request

## License

Copyright The Linux Foundation and each contributor to LFX.

This project’s source code is licensed under the MIT License. A copy of the
license is available in `LICENSE`.

This project’s documentation is licensed under the Creative Commons Attribution
4.0 International License \(CC-BY-4.0\). A copy of the license is available in
`LICENSE-docs`.
