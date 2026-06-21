# brain

A minimal Go project scaffold with a tiny CLI and tests.

## Prerequisites

- Go 1.22+

## Quick start

```bash
go run ./cmd/brain
```

Try a custom name:

```bash
go run ./cmd/brain --name john
```

Print version:

```bash
go run ./cmd/brain --version
```

## Test

```bash
go test ./...
```

## TaskNotes

The TaskNotes CLI is a re-implementation of the TaskNotes CLI written in Go. 

```
# Create task (natural language parsed)
tn "Review PR #123 tomorrow high priority @work"

# Interactive mode
tn

# List tasks
tn list
tn list --today
tn list --overdue
tn list --completed
tn list --filter "priority:urgent AND tags:work"
tn list --json

# Task operations
tn complete <taskId>
tn toggle <taskId>
tn archive <taskId>
tn delete <taskId> --force
tn update <taskId> --status completed --priority high --due 2025-08-20

# Update tags/contexts/projects
tn update <taskId> --add-tags "urgent,bug" --remove-tags "low-priority"
tn update <taskId> --add-contexts "office" --add-projects "Website"

# Search
tn search "groceries"
```
