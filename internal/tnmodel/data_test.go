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
	require.Equal(t, "open", note.Status)
	require.Equal(t, "normal", note.Priority)
	require.NotNil(t, note.Scheduled)
	require.Equal(t, "2026-06-20", note.Scheduled.Format("2006-01-02"))
	require.NotNil(t, note.Due)
	require.Equal(t, "2026-06-21", note.Due.Format("2006-01-02"))
	require.NotNil(t, note.DateCreated)
	require.NotNil(t, note.DateModified)
	require.Len(t, note.Tags, 1)
	require.Equal(t, "task", note.Tags[0])
	require.Equal(t, "Go grocery shopping later", note.Title)
	require.Equal(t, "# Go grocery shopping later", strings.TrimSpace(note.Body))
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
	require.NotNil(t, note.Scheduled)
	require.Equal(t, "2025-12-30T09:00", note.Scheduled.Format("2006-01-02T15:04"))
}

func TestDecode_WithoutFrontmatter(t *testing.T) {
	input := "# Heading only\n"
	note, err := Decode("TaskNotes/Tasks/Test.md", input)
	require.NoError(t, err)
	require.Equal(t, "", note.Status)
	require.Equal(t, "", note.Priority)
	require.Equal(t, "Heading only", note.Title)
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
	require.Equal(t, 3, note.Extra["estimate"])
	require.Equal(t, "john", note.Extra["owner"])
}

func TestEncode_ProducesYAMLFrontmatter(t *testing.T) {
	scheduled := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	due := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	created := time.Date(2026, 6, 20, 13, 48, 44, 160000000, time.FixedZone("PDT", -7*3600))
	modified := created.Add(5 * time.Minute)

	note := &TaskNote{
		Status:       "open",
		Priority:     "normal",
		Scheduled:    &scheduled,
		Due:          &due,
		DateCreated:  &created,
		DateModified: &modified,
		Tags:         []string{"task"},
		Body:         "# Go grocery shopping later\n",
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
		Status:       StatusOpen,
		Priority:     PriorityHigh,
		Scheduled:    &scheduled,
		DateCreated:  &created,
		DateModified: &modified,
		Tags:         []string{"task", "shopping"},
		Body:         "# Buy milk\n\nRemember lactose-free.\n",
		Extra: map[string]any{
			"estimate": 2,
			"owner":    "john",
		},
	}

	encoded, err := Encode(original)
	require.NoError(t, err)
	decoded, err := Decode("TaskNotes/Tasks/Test.md", encoded)
	require.NoError(t, err)
	require.Equal(t, original.Status, decoded.Status)
	require.Equal(t, original.Priority, decoded.Priority)
	require.Equal(t, strings.TrimSpace(original.Body), strings.TrimSpace(decoded.Body))
}
