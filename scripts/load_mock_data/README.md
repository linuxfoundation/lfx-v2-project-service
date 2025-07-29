# Project Mock Data Loader

This Go script allows you to insert project mock data via the project service API. It can create any number of projects based on input parameters.

## Features

- Generate random project data with realistic names and descriptions
- Configurable number of projects to create
- Proper error handling and logging
- Rate limiting to avoid overwhelming the API
- Support for authentication via Bearer token
- Configurable API endpoint and timeout

## Prerequisites

- Go 1.23 or later
- Access to the project service API
- Valid Bearer token for authentication

## Building the Script

From the project root directory:

```bash
go build -o bin/load_mock_data tools/load_mock_data/main.go
```

Or run directly:

```bash
go run tools/load_mock_data/main.go [flags]
```

## Usage

### Basic Usage

```bash
# Create 10 projects with default settings
./bin/load_mock_data -bearer-token "your-jwt-token-here" -parent-uid "root-project-uid"

# Create 5 projects
./bin/load_mock_data -bearer-token "your-jwt-token-here" -parent-uid "root-project-uid" -num-projects 5

# Use a different API endpoint
./bin/load_mock_data -bearer-token "your-jwt-token-here" -parent-uid "root-project-uid" -api-url "http://api.example.com/projects"
```

### Command Line Flags

| Flag | Description | Default | Required |
|------|-------------|---------|----------|
| `-parent-uid` | Parent UID for all generated projects | "" | **Yes** |
| `-bearer-token` | JWT Bearer token for authentication | "" | No |
| `-num-projects` | Number of projects to create | 10 | No |
<!-- markdownlint-disable-next-line MD034 -->
<!-- markdown-link-check-disable-next-line -->
| `-api-url` | Project service API URL | "http://localhost:8080/projects" | No |
| `-version` | API version | "1" | No |
| `-timeout` | Request timeout | "30s" | No |

### Examples

```bash
# Create 100 projects with custom timeout
./bin/load_mock_data \
  -parent-uid "root-project-uid" \
  -bearer-token "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
  -num-projects 100 \
  -timeout 60s

# Create 5 projects using staging API
./bin/load_mock_data \
  -parent-uid "root-project-uid" \
  -bearer-token "your-token" \
  -num-projects 5 \
  -api-url "https://staging-api.example.com/projects"

# Create 25 projects with API version 2
./bin/load_mock_data \
  -parent-uid "root-project-uid" \
  -bearer-token "your-token" \
  -num-projects 25 \
  -version "2"
```

## Generated Data

The script generates project data including:

### Project Names

- Random selection of 10 hard-coded project names (e.g. Project 1, Project 2)

### Descriptions

- Random selection of 10 hard-coded project descriptions (e.g. A test description 1)

### Auditors and Writers

- Random selection of 1-3 auditors and writers from a predefined list (e.g. user123, admin001)

### Slugs

- Automatically generated from project names
- URL-friendly format following the pattern: `^[a-z][a-z0-9_\-]*[a-z0-9]$`
- Ensures uniqueness by adding index numbers when needed

## Output

The script provides detailed logging of the creation process:

```text
2024/01/15 10:30:00 Starting to create 10 projects...
2024/01/15 10:30:01 Creating project 1/10: Project 1 (project-1)
2024/01/15 10:30:01 Successfully created project: project-1
2024/01/15 10:30:01 Creating project 2/10: Project 2 (project-2)
2024/01/15 10:30:02 Successfully created project: project-2
...
2024/01/15 10:30:10 Completed! Successfully created 10 projects, 0 errors
2024/01/15 10:30:10 Mock data loading completed successfully!
```

## Error Handling

The script handles various error scenarios:

- **Authentication errors**: Invalid or missing Bearer token
- **API errors**: Network issues, server errors, validation errors
- **Rate limiting**: Built-in delays between requests
- **Duplicate slugs**: Automatic index addition to ensure uniqueness

## Rate Limiting

To avoid overwhelming the API, the script includes a 100ms delay between project creation requests. This can be adjusted by modifying the `time.Sleep(100 * time.Millisecond)` line in the code.

## Security Notes

- Store your Bearer token securely
- Consider using environment variables for sensitive data
- The script does not store or log the Bearer token
- Use HTTPS endpoints in production environments

## Troubleshooting

### Common Issues

1. **Authentication Error**: Ensure your Bearer token is valid and not expired
2. **Network Error**: Check if the API endpoint is accessible
3. **Validation Error**: The script generates valid data, but API validation rules may change
4. **Timeout Error**: Increase the timeout value for large numbers of projects

### Debug Mode

To see more detailed information, you can modify the script to include debug logging or use Go's built-in logging flags:

```bash
go run -v tools/load_mock_data/main.go -bearer-token "your-token"
```

## Contributing

When modifying the script:

1. Update the project names, descriptions, and manager IDs as needed
2. Test with small numbers of projects first
3. Ensure the slug generation logic follows the API's validation rules
4. Update this README if adding new features or changing behavior
