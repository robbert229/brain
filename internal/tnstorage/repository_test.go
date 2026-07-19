package tnstorage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/robbert229/brain/internal/tnmodel"
	"github.com/stretchr/testify/require"
)

func TestDiskTaskNoteRepository_Crawl_WalksDirectoryAndDecodesMarkdownFiles(t *testing.T) {
	tmpDir := t.TempDir()
	repo, err := NewDiskTaskNoteRepository(WithWorkingDirectory(tmpDir))
	require.NoError(t, err)

	// Create test markdown files
	file1 := filepath.Join(tmpDir, "task1.md")
	file2 := filepath.Join(tmpDir, "subdir", "task2.md")

	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755))

	content1 := `---
status: open
priority: high
tags:
- task
---
# Task One

First task description.`

	content2 := `---
status: completed
priority: low
tags:
- task
---
# Task Two

Second task description.`

	require.NoError(t, os.WriteFile(file1, []byte(content1), 0644))
	require.NoError(t, os.WriteFile(file2, []byte(content2), 0644))

	// Create a non-markdown file that should be ignored
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("not markdown"), 0644))

	var notes []*tnmodel.TaskNote
	err = repo.Crawl(t.Context(), func(note *tnmodel.TaskNote) error {
		notes = append(notes, note)
		return nil
	})

	require.NoError(t, err)
	require.Len(t, notes, 2)

	// Find tasks by title since order may vary
	var task1, task2 *tnmodel.TaskNote
	for _, note := range notes {
		if tnmodel.Title(note) == "Task One" {
			task1 = note
		} else if tnmodel.Title(note) == "Task Two" {
			task2 = note
		}
	}

	require.NotNil(t, task1)
	require.NotNil(t, task2)

	// Check first task
	require.Equal(t, "open", tnmodel.Status(task1))
	require.Equal(t, "high", tnmodel.Priority(task1))
	require.Len(t, tnmodel.Tags(task1), 1)
	require.Equal(t, "task", tnmodel.Tags(task1)[0])

	// Check second task
	require.Equal(t, "completed", tnmodel.Status(task2))
	require.Equal(t, "low", tnmodel.Priority(task2))
}

func TestDiskTaskNoteRepository_Crawl_DirectoryNotFound(t *testing.T) {
	repo, err := NewDiskTaskNoteRepository(WithWorkingDirectory("/nonexistent/directory"))
	require.NoError(t, err)

	err = repo.Crawl(t.Context(), func(_ *tnmodel.TaskNote) error {
		return nil
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "stat directory")
}

func TestDiskTaskNoteRepository_Crawl_StopsOnCallbackError(t *testing.T) {
	tmpDir := t.TempDir()
	repo, err := NewDiskTaskNoteRepository(WithWorkingDirectory(tmpDir))
	require.NoError(t, err)

	file1 := filepath.Join(tmpDir, "task1.md")
	file2 := filepath.Join(tmpDir, "task2.md")

	content := `---
status: open
tags:
- task
---
# Task`

	require.NoError(t, os.WriteFile(file1, []byte(content), 0644))
	require.NoError(t, os.WriteFile(file2, []byte(content), 0644))

	callCount := 0
	testErr := errors.New("test callback error")
	err = repo.Crawl(t.Context(), func(_ *tnmodel.TaskNote) error {
		callCount++
		if callCount == 1 {
			return nil
		}
		return testErr
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "callback")
	// Only the first call should have succeeded; second should have failed
	require.Equal(t, 2, callCount)
}

func TestDiskTaskNoteRepository_Crawl_SkipsInvalidMarkdownFile(t *testing.T) {
	tmpDir := t.TempDir()
	repo, err := NewDiskTaskNoteRepository(WithWorkingDirectory(tmpDir))
	require.NoError(t, err)

	file := filepath.Join(tmpDir, "invalid.md")
	require.NoError(t, os.WriteFile(file, []byte("---\ninvalid: [syntax"), 0644))

	callCount := 0
	err = repo.Crawl(t.Context(), func(_ *tnmodel.TaskNote) error {
		callCount++
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, 0, callCount)
}

func TestDiskTaskNoteRepository_Crawl_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	repo, err := NewDiskTaskNoteRepository(WithWorkingDirectory(tmpDir))
	require.NoError(t, err)

	callCount := 0
	err = repo.Crawl(t.Context(), func(_ *tnmodel.TaskNote) error {
		callCount++
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, 0, callCount)
}
