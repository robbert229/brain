package tnservice

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/robbert229/brain/internal/tnmodel"
	"github.com/robbert229/brain/internal/tnstorage"
	"github.com/stretchr/testify/require"
)

type stubTaskNoteRepository struct {
	notes []*tnmodel.TaskNote
}

func (repo stubTaskNoteRepository) Crawl(_ context.Context, fn func(*tnmodel.TaskNote) error) error {
	for _, note := range repo.notes {
		if err := fn(note); err != nil {
			return err
		}
	}

	return nil
}

func TestList_UsesProvidedRepository(t *testing.T) {
	service := NewTaskNoteService(stubTaskNoteRepository{
		notes: []*tnmodel.TaskNote{
			{
				ID:       "from-repository.md",
				Title:    "From repository",
				Status:   "open",
				Priority: "normal",
				Tags:     []string{"task"},
			},
		},
	})

	result, err := service.List(t.Context(), ListRequest{})
	require.NoError(t, err)

	require.Equal(t, 1, result.FoundCount)
	require.Len(t, result.Notes, 1)
	require.Equal(t, "From repository", result.Notes[0].Title)
	require.Equal(t, "from-repository.md", result.Notes[0].ID)
}

func TestList_TodayFilter(t *testing.T) {
	tmpDir := t.TempDir()
	service := NewTaskNoteService(tnstorage.NewDiskTaskNoteRepository(tmpDir))

	writeTaskNote(t, tmpDir, "today.md", `---
status: open
priority: normal
scheduled: 2026-06-21
tags:
  - task
---
# Today task
`)
	writeTaskNote(t, tmpDir, "other.md", `---
status: open
priority: normal
scheduled: 2026-06-22
tags:
  - task
---
# Other task
`)

	result, err := service.List(t.Context(), ListRequest{
		Today: true,
		Now:   time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	require.Equal(t, 1, result.FoundCount)
	require.Len(t, result.Notes, 1)
	require.Equal(t, "Today task", result.Notes[0].Title)
}

func TestList_OverdueFilter(t *testing.T) {
	tmpDir := t.TempDir()
	service := NewTaskNoteService(tnstorage.NewDiskTaskNoteRepository(tmpDir))

	writeTaskNote(t, tmpDir, "overdue.md", `---
status: open
priority: normal
due: 2026-06-20
tags:
  - task
---
# Overdue task
`)
	writeTaskNote(t, tmpDir, "completed-overdue.md", `---
status: done
priority: normal
due: 2026-06-20
tags:
  - task
---
# Completed overdue task
`)
	writeTaskNote(t, tmpDir, "future.md", `---
status: open
priority: normal
due: 2026-06-22
tags:
  - task
---
# Future task
`)

	result, err := service.List(t.Context(), ListRequest{
		Overdue: true,
		Now:     time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	require.Equal(t, 1, result.FoundCount)
	require.Len(t, result.Notes, 1)
	require.Equal(t, "Overdue task", result.Notes[0].Title)
}

func TestList_CompletedFilter(t *testing.T) {
	tmpDir := t.TempDir()
	service := NewTaskNoteService(tnstorage.NewDiskTaskNoteRepository(tmpDir))

	writeTaskNote(t, tmpDir, "done.md", `---
status: done
priority: normal
tags:
  - task
---
# Done task
`)
	writeTaskNote(t, tmpDir, "open.md", `---
status: open
priority: normal
tags:
  - task
---
# Open task
`)

	result, err := service.List(t.Context(), ListRequest{
		Completed: true,
		Now:       time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	require.Equal(t, 1, result.FoundCount)
	require.Len(t, result.Notes, 1)
	require.Equal(t, "Done task", result.Notes[0].Title)
}

func TestList_Limit(t *testing.T) {
	tmpDir := t.TempDir()
	service := NewTaskNoteService(tnstorage.NewDiskTaskNoteRepository(tmpDir))

	writeTaskNote(t, tmpDir, "a.md", `---
status: open
priority: normal
tags:
  - task
---
# First task
`)
	writeTaskNote(t, tmpDir, "b.md", `---
status: open
priority: normal
tags:
  - task
---
# Second task
`)
	writeTaskNote(t, tmpDir, "c.md", `---
status: open
priority: normal
tags:
  - task
---
# Third task
`)

	result, err := service.List(t.Context(), ListRequest{
		Limit: 2,
	})
	require.NoError(t, err)

	require.Equal(t, 3, result.FoundCount)
	require.Len(t, result.Notes, 2)
	require.Equal(t, "First task", result.Notes[0].Title)
	require.Equal(t, "Second task", result.Notes[1].Title)
}

func writeTaskNote(t *testing.T, root string, name string, content string) {
	t.Helper()

	path := filepath.Join(root, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}
