# Task Creation - Quick Start Guide

## Creating Tasks

You can now create tasks using natural language with the `brain tn` command:

```bash
brain tn "Your task description here"
```

## Examples

### Basic Task
```bash
brain tn "Buy groceries"
```

### Task with Tags
```bash
brain tn "Review code #urgent #review"
```

### Task with Context (where/who)
```bash
brain tn "Call client @phone @work"
```

### Task with Project
```bash
brain tn "Update documentation +website-redesign"
```

### Task with Due Date
```bash
brain tn "Submit report due 2026-07-25"
brain tn "Pay bills due tomorrow"
brain tn "Finish presentation due next week"
```

### Task with Scheduled Date
```bash
brain tn "Team meeting scheduled 2026-07-20"
```

### Combining Multiple Attributes
```bash
brain tn "Fix critical bug #urgent @work +webapp due tomorrow"
brain tn "Review pull request #code-review @work due 2026-07-20"
brain tn "Plan vacation #personal +2026-goals"
```

## Natural Language Features

The task creation supports intelligent parsing of:

- **Tags**: Use `#tagname` to categorize tasks
- **Contexts**: Use `@context` to specify where/with whom
- **Projects**: Use `+project` to associate with projects
- **Dates**: 
  - ISO format: `2026-07-25`
  - Relative: `tomorrow`, `next week`, `in 3 days`
  - Natural: `July 25`, `next Monday`
- **Priority**: Include words like `high`, `urgent`, `low`
- **Status**: Keywords like `open`, `in-progress`, etc.

## Where Tasks Are Saved

Tasks are automatically saved in the `tasks/` directory of your vault:
```
your-vault/
  └── tasks/
      └── 20260719-163504-Review-pull-request.md
```

Files are named with:
- Timestamp: `YYYYMMDD-HHMMSS`
- Sanitized title: cleaned version of your task title
- Extension: `.md`

## Task File Format

Each task is saved as a markdown file with YAML frontmatter:

```markdown
---
contexts:
    - work
date_created: 2026-07-19T16:35:04.6086-07:00
date_modified: 2026-07-19T16:35:04.6086-07:00
due: "2026-07-20"
status: open
tags:
    - task
    - code-review
title: Review pull request
---
# Review pull request
```

## Viewing Your Tasks

After creating tasks, list them with:

```bash
# List all tasks
brain tn list

# List tasks due today
brain tn list --today

# List overdue tasks
brain tn list --overdue

# List with custom limit
brain tn list --limit 50
```

## Tips

1. **Always include the "task" tag**: This is automatically added for you
2. **Use descriptive titles**: They become the filename
3. **Combine attributes**: Mix tags, contexts, projects, and dates in one command
4. **Quote your input**: Use single or double quotes around the entire task description

## Next Steps

- Tasks are immediately available after creation
- Edit task files directly in your vault
- Use `brain tn list` to view and filter tasks
- Future commands will support updating, completing, and deleting tasks

