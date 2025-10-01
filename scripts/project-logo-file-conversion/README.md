# project-logo-file-conversion

## Summary

This script converts SVG project logos to PNG format for use in environments that don't support SVG images (e.g., Gmail email clients). It can process individual files, selected projects, or all LFX projects.

The script fetches project data directly from the NATS KV store and converts SVG logos to PNG format using Inkscape. Converted files can optionally be uploaded to an S3 bucket for public access.

Each file is stored in an S3 bucket (when `-write-s3` is enabled) with the name `{project_uid}.png`. The S3 bucket follows the naming convention `lfx-one-project-logos-png-{lfx_environment}` where `lfx_environment` is one of: dev, stg, prod.

### Why does this script exist?

Many project logos are in SVG format, which is not supported by Gmail and other email clients. To display logos in meeting emails and other communications, they need to be converted to PNG format and made publicly accessible.

## Prerequisites

- **Inkscape**: Required for SVG to PNG conversion
  - macOS: `brew install inkscape`
  - Linux: `apt-get install inkscape` or `yum install inkscape`
- **NATS access**: Connection to the NATS server with projects KV store
- **AWS credentials** (optional): Only needed if using `-write-s3` flag

## Usage

### Environment Variables

```bash
# Required for NATS connection (used by -all-project-logos and -select-project-logos modes)
export NATS_URL="nats://localhost:4222"

# Required only when using -write-s3 flag
export LFX_ENVIRONMENT="dev"  # Options: dev, stg, prod
```

### Running the Script

```bash
go run main.go [flags]
```

### Command-Line Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-all-project-logos` | bool | false | Fetch and convert all project logos from NATS KV store |
| `-select-project-logos` | string | "" | Comma-separated project UIDs to convert (e.g., "uid1,uid2,uid3") |
| `-url` | string | "" | URL of a single SVG file to convert (works with any SVG, not just project logos) |
| `-write-s3` | bool | false | Upload converted files to S3 bucket |
| `-keep-files` | bool | true | Keep converted files stored locally after completion |
| `-width` | int | 0 | Output image width in pixels (0 = calculate from height maintaining aspect ratio) |
| `-height` | int | 800 | Output image height in pixels |
| `-d` | bool | false | Enable debug logging |

### Operating Modes

The script has three distinct modes:

#### 1. Single File Mode (`-url`)

Convert any SVG file from a URL to PNG:

```bash
go run main.go -url="https://example.com/logo.svg" -width=300 -height=200
```

#### 2. Selected Projects Mode (`-select-project-logos`)

Convert logos for specific projects:

```bash
export NATS_URL="nats://localhost:4222"
go run main.go \
  -select-project-logos="f47ac10b-58cc-4372-a567-0e02b2c3d479,550e8400-e29b-41d4-a716-446655440000" \
  -height=800 \
  -write-s3
```

#### 3. All Projects Mode (`-all-project-logos`)

Convert logos for all projects in NATS KV store:

```bash
export NATS_URL="nats://localhost:4222"
export LFX_ENVIRONMENT="dev"
go run main.go \
  -all-project-logos \
  -height=800 \
  -write-s3 \
  -keep-files=false
```

### Notes

- **Width Calculation**: If `-width=0`, the width is automatically calculated from the height while maintaining the original aspect ratio
- **Default Dimensions**: If the SVG doesn't contain valid dimensions, defaults to 1600x800
- **S3 Upload**: When using `-write-s3`, ensure you have valid AWS credentials configured and the `LFX_ENVIRONMENT` variable set
- **File Cleanup**: Use `-keep-files=false` to automatically delete local files after processing
- **Skip Logic**: Projects without logos or with non-SVG logos are automatically skipped
