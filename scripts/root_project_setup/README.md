# Root Project Setup

A utility that ensures a root project exists in the project service key-value store for teams permissions assignment.

## Overview

This script creates a special "ROOT" project in the NATS JetStream key-value store. The ROOT project is used for teams permissions assignment and is ordinarily hidden from users. It serves as a foundational project for the project service's authorization system.

## Build

To build the binary:

```bash
go build -o main main.go
```

Or build from the project root:

```bash
go build -o scripts/root-project-setup/main scripts/root-project-setup/main.go
```

## Usage

### Environment Variables

- `NATS_URL`: NATS server URL (default: `nats://localhost:4222`)

### Running the Script

```bash
# Using default NATS URL
./main

# With custom NATS URL
NATS_URL=nats://your-nats-server:4222 ./main
```

### Example Output

```
2024-01-15T10:30:00Z INF connecting to NATS nats_url=nats://localhost:4222
2024-01-15T10:30:00Z INF NATS connection established nats_url=nats://localhost:4222
2024-01-15T10:30:00Z INF ROOT project created successfully slug=ROOT uid=550e8400-e29b-41d4-a716-446655440000
2024-01-15T10:30:00Z INF root project setup completed successfully
```

If the ROOT project already exists:

```
2024-01-15T10:30:00Z INF ROOT project already exists, nothing to do
2024-01-15T10:30:00Z INF root project setup completed successfully
```

## What It Does

1. **Connects to NATS**: Establishes a connection to the NATS server using JetStream
2. **Checks for Existing ROOT Project**: Looks for a project with slug "ROOT" in the key-value store
3. **Creates ROOT Project**: If not found, creates a new ROOT project with:
   - Slug: "ROOT"
   - Name: "ROOT"
   - Description: "A root project for teams permissions assignment, ordinarily hidden from users."
   - Public: false
   - No parent project
   - Empty auditors and writers lists
4. **Stores in Key-Value Store**: Saves the project using both slug-based and UID-based keys

## Dependencies

- Go 1.21+
- NATS server with JetStream enabled
- Access to the project service's NATS key-value bucket

## Error Handling

The script will exit with code 1 if any of the following occur:
- Unable to connect to NATS
- Unable to access the key-value store
- Error checking for existing ROOT project
- Error creating or storing the ROOT project

The script uses structured logging to provide detailed error information for troubleshooting.

## Timeout

The script has a 30-second timeout for the entire operation to prevent hanging in case of network issues.