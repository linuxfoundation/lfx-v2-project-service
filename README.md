# LFX V2 Project Service

This repository contains the source code the LFX v2 platform project service.

## File Structure

```bash
├── .github/                        # Github files
│   └── workflows/                  # Github Action workflow files
├── charts/                         # Helm charts for running the service in kubernetes
├── cmd/                            # Services (main packages)
│   └── project-api/                # Project service code
├── internal/                       # Internal service packages
│   ├── domain/                     # Domain logic layer
│   ├── service/                    # Business logic layer
│   └── infrastructure/             # Infrastructure layer
└── pkg/                            # Shared packages for internal and external services
```

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
