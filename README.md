# brain

Brain is a command line utility, and daemon for managing a secondary brain in markdown, and yaml. It is designed to be 
used with Obsidian notebook's, but can be used standalone.

## Features

### TaskNotes

The TaskNotes CLI is a re-implementation of the TaskNotes CLI written in Go. It is completely daemonless and functions
without any dependency on Obsidian. 

When the CLI is invoked from within a vault it will search the entire vault. It does this by navigating to parent 
folders until it finds a `.obsidian` folder. If invoked outside of a vault it will only search files in the current folder.

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
