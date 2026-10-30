# Implementation Plan: Edit Book Reviews

**Branch**: `011-edit-books` | **Spec**: [spec.md](./spec.md)

## Summary

Add role-gated edit links and a prefilled form for existing shelf notes. Resolve a relative note identifier safely under the configured Books folder, preserve unrelated YAML fields, validate edited content with the existing review rules, and atomically replace that same note.

## Technical Context

**Language/Version**: Go 1.26; standard library HTTP handlers and filesystem APIs.
**Storage**: Existing Markdown note remains canonical.
**Constraints**: Contributor/admin authorization, CSRF, read-only mode, size bounds, path confinement, no duplicate note.
**Testing**: Fixture-based domain and handler tests are the repository convention.

## Design

- Carry a slash-separated relative note path from catalog entries to edit links.
- Add read/update operations to the books domain package with local-path and symlink checks.
- Parse and retain unrelated frontmatter while updating editable fields and the note body.
- Reuse the existing review validation and form conventions for the edit form.
- Mount GET and POST `/books/edit` behind `RoleUser` and shared CSRF middleware.
