# Implementation Plan: Fallback Book Covers

**Branch**: `012-fallback-covers` | **Spec**: [spec.md](./spec.md)

## Summary

Replace the blank placeholder tile with an embedded, designed cover asset. The shelf selects it whenever the configured attachment cannot be safely rendered. Keep the asset local and the existing image dimensions/layout.

## Technical Context

**Language/Version**: Go 1.26; server-rendered HTML templates and embedded web assets.
**Storage**: Existing vault files remain canonical; the fallback is a static application asset.
**Constraints**: No external image service; preserve same-origin CSP and attachment path confinement.
**Testing**: Existing Go handler tests and static asset checks are the project conventions.

## Design

- Add an SVG fallback asset under `internal/braind/webassets/static/`.
- Keep fallback selection in the shelf template using the existing `CoverFound` value.
- Give the fallback an accessible alt description and preserve `.cover` sizing.
- Ensure attachment availability is determined by readability as well as metadata/existence.
