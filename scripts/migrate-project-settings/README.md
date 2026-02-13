# Project Settings Migration Script

This script migrates project settings from the old format (where writers and auditors are arrays of strings) to the new format (where they are arrays of UserInfo objects with name, username, email, and avatar fields).

## Usage

```bash
cd scripts/migrate-project-settings
go run main.go <project-uid>
```

## Environment Variables

- `NATS_URL`: NATS server URL (defaults to `nats://localhost:4222`)

## What it does

1. Connects to NATS and retrieves the project settings from the `project-settings` KV store
2. Checks if the settings are already in the new format
3. If in old format, prompts for user details (name, username, email, avatar) for each writer and auditor
4. Updates the settings in the NATS KV store with the new format
5. Sends an indexer sync message to update the search index

## Example

```bash
# Set NATS URL if different from default
export NATS_URL=nats://localhost:4222

# Run migration for a specific project
go run main.go 7cad5a8d-19d0-41a4-81a6-043453daf9ee
```

The script will prompt you for each user:

```
Migrating writer: johndoe
Enter details for user 'johndoe':
Name: John Doe
Username [johndoe]: 
Email: john.doe@example.com
Avatar URL (optional): https://example.com/avatar.jpg

Migrating auditor: janesmith
Enter details for user 'janesmith':
Name: Jane Smith
Username [janesmith]: 
Email: jane.smith@example.com
Avatar URL (optional): 
```

## Notes

- The script preserves all existing project settings data
- Only writers and auditors fields are migrated from string arrays to UserInfo arrays
- The script updates the `updated_at` timestamp
- An indexer sync message is sent after successful migration
- If the project is already in the new format, the script will exit without making changes