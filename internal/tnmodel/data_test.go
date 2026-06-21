package tnmodel

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDecode_YAMLFrontmatterAndBody(t *testing.T) {
	input := `---
status: open
priority: normal
scheduled: 2026-06-20
due: 2026-06-21
dateCreated: 2026-06-20T13:48:44.160-07:00
dateModified: 2026-06-20T13:48:44.160-07:00
tags:
  - task
---
# Go grocery shopping later
`

	note, err := Decode("TaskNotes/Tasks/Test.md", input)
	require.NoError(t, err)
	require.Equal(t, "open", Status(note))
	require.Equal(t, "normal", Priority(note))
	require.NotNil(t, Scheduled(note))
	require.Equal(t, "2026-06-20", Scheduled(note).Format("2006-01-02"))
	require.NotNil(t, Due(note))
	require.Equal(t, "2026-06-21", Due(note).Format("2006-01-02"))
	require.False(t, note.Frontmatter.DateCreated.IsZero())
	require.False(t, note.Frontmatter.DateModified.IsZero())
	require.Len(t, Tags(note), 1)
	require.Equal(t, "task", Tags(note)[0])
	require.Equal(t, "Go grocery shopping later", Title(note))
	require.Equal(t, "# Go grocery shopping later", strings.TrimSpace(Body(note)))
}

func TestDecode_ScheduledDateTimeParses(t *testing.T) {
	input := `---
status: todo
scheduled: 2025-12-30T09:00
---
# Daily standup
`

	note, err := Decode("TaskNotes/Tasks/Test.md", input)
	require.NoError(t, err)
	require.NotNil(t, Scheduled(note))
	require.Equal(t, "2025-12-30T09:00", Scheduled(note).Format("2006-01-02T15:04"))
}

func TestDecode_WithoutFrontmatter(t *testing.T) {
	input := "# Heading only\n"
	note, err := Decode("TaskNotes/Tasks/Test.md", input)
	require.NoError(t, err)
	require.Equal(t, "", Status(note))
	require.Equal(t, "", Priority(note))
	require.Equal(t, "Heading only", Title(note))
}

func TestDecode_InvalidFrontmatter(t *testing.T) {
	input := `---
status: open
priority normal
---
# broken
`
	_, err := Decode("TaskNotes/Tasks/Test.md", input)
	require.Error(t, err)
}

func TestDecode_PreservesUnknownFieldsInExtra(t *testing.T) {
	input := `---
status: open
estimate: 3
owner: john
nested:
  k: v
---
# task
`
	note, err := Decode("TaskNotes/Tasks/Test.md", input)
	require.NoError(t, err)
	extra := note.Frontmatter.AdditionalProperties.(map[string]any)
	require.Equal(t, 3, extra["estimate"])
	require.Equal(t, "john", extra["owner"])
}

func TestEncode_ProducesYAMLFrontmatter(t *testing.T) {
	scheduled := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	due := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	created := time.Date(2026, 6, 20, 13, 48, 44, 160000000, time.FixedZone("PDT", -7*3600))
	modified := created.Add(5 * time.Minute)

	note := &TaskNote{
		Body: stringPtr("# Go grocery shopping later\n"),
		Frontmatter: TaskFrontmatter{
			Status:       "open",
			Priority:     stringPtr("normal"),
			Scheduled:    dateOrDateTimePtr(scheduled.Format(dateLayout)),
			Due:          dateOrDateTimePtr(due.Format(dateLayout)),
			DateCreated:  created,
			DateModified: modified,
			Tags:         []string{"task"},
		},
	}

	got, err := Encode(note)
	require.NoError(t, err)

	for _, want := range []string{
		"status: open",
		"priority: normal",
		"scheduled: \"2026-06-20\"",
		"due: \"2026-06-21\"",
		"dateCreated: \"2026-06-20T13:48:44.16-07:00\"",
		"dateModified: \"2026-06-20T13:53:44.16-07:00\"",
		"tags:",
		"- task",
		"# Go grocery shopping later",
	} {
		require.Contains(t, got, want)
	}
}

func TestEncodeDecode_RoundTrip(t *testing.T) {
	scheduled := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	created := time.Date(2026, 1, 2, 10, 11, 12, 0, time.UTC)
	modified := time.Date(2026, 1, 3, 10, 11, 12, 0, time.UTC)

	original := &TaskNote{
		Body: stringPtr("# Buy milk\n\nRemember lactose-free.\n"),
		Frontmatter: TaskFrontmatter{
			Status:       StatusOpen,
			Priority:     stringPtr(PriorityHigh),
			Scheduled:    dateOrDateTimePtr(scheduled.Format(dateLayout)),
			DateCreated:  created,
			DateModified: modified,
			Tags:         []string{"task", "shopping"},
			AdditionalProperties: map[string]any{
				"estimate": 2,
				"owner":    "john",
			},
		},
	}

	encoded, err := Encode(original)
	require.NoError(t, err)
	decoded, err := Decode("TaskNotes/Tasks/Test.md", encoded)
	require.NoError(t, err)
	require.Equal(t, Status(original), Status(decoded))
	require.Equal(t, Priority(original), Priority(decoded))
	require.Equal(t, strings.TrimSpace(Body(original)), strings.TrimSpace(Body(decoded)))
}

func dateOrDateTimePtr(value string) *DateOrDateTime {
	dateOrDateTime := DateOrDateTime(value)
	return &dateOrDateTime
}
