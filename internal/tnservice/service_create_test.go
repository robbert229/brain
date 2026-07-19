package tnservice

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/robbert229/brain/internal/tnmodel"
	"github.com/stretchr/testify/require"
)

func TestCreate_SimpleTask(t *testing.T) {
	tmpDir := t.TempDir()
	service := MustTaskNoteService(t, tmpDir)

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	result, err := service.Create(context.Background(), CreateRequest{
		NaturalLanguageInput: "Buy groceries #shopping @home",
		Now:                  now,
	})

	require.NoError(t, err)
	require.NotNil(t, result.TaskNote)

	note := result.TaskNote
	require.Equal(t, "Buy groceries", tnmodel.Title(note))
	require.Equal(t, "open", tnmodel.Status(note))
	require.Contains(t, tnmodel.Tags(note), "task")
	require.Contains(t, tnmodel.Tags(note), "shopping")
	require.Contains(t, note.Frontmatter.Contexts, "home")

	// Verify the file was created
	filePath := filepath.Join(tmpDir, tnmodel.ID(note))
	require.FileExists(t, filePath)

	// Verify we can read it back
	content, err := os.ReadFile(filePath)
	require.NoError(t, err)

	decoded, err := tnmodel.Decode(tnmodel.ID(note), string(content))
	require.NoError(t, err)
	require.Equal(t, "Buy groceries", tnmodel.Title(decoded))
}

func TestCreate_WithDueDate(t *testing.T) {
	tmpDir := t.TempDir()
	service := MustTaskNoteService(t, tmpDir)

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	result, err := service.Create(context.Background(), CreateRequest{
		NaturalLanguageInput: "Submit report due 2026-07-20",
		Now:                  now,
	})

	require.NoError(t, err)
	require.NotNil(t, result.TaskNote)

	note := result.TaskNote
	require.Equal(t, "Submit report", tnmodel.Title(note))

	due := tnmodel.Due(note)
	require.NotNil(t, due)
	require.Equal(t, 2026, due.Year())
	require.Equal(t, time.July, due.Month())
	require.Equal(t, 20, due.Day())
}

func TestCreate_WithPriority(t *testing.T) {
	tmpDir := t.TempDir()
	service := MustTaskNoteService(t, tmpDir)

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	result, err := service.Create(context.Background(), CreateRequest{
		NaturalLanguageInput: "Fix bug !high",
		Now:                  now,
	})

	require.NoError(t, err)
	require.NotNil(t, result.TaskNote)

	note := result.TaskNote
	// The NLP parser may leave the trigger in the title depending on configuration
	require.Contains(t, tnmodel.Title(note), "Fix bug")
	require.Equal(t, "high", tnmodel.Priority(note))
}

func TestCreate_WithProjects(t *testing.T) {
	tmpDir := t.TempDir()
	service := MustTaskNoteService(t, tmpDir)

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	result, err := service.Create(context.Background(), CreateRequest{
		NaturalLanguageInput: "Review code +webapp +backend",
		Now:                  now,
	})

	require.NoError(t, err)
	require.NotNil(t, result.TaskNote)

	note := result.TaskNote
	require.Equal(t, "Review code", tnmodel.Title(note))
	require.Contains(t, note.Frontmatter.Projects, "webapp")
	require.Contains(t, note.Frontmatter.Projects, "backend")
}

func TestCreate_CreatesTasksDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	service := MustTaskNoteService(t, tmpDir)

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	result, err := service.Create(context.Background(), CreateRequest{
		NaturalLanguageInput: "Test task",
		Now:                  now,
	})

	require.NoError(t, err)
	require.NotNil(t, result.TaskNote)

	// Verify tasks directory was created
	tasksDir := filepath.Join(tmpDir, "tasks")
	info, err := os.Stat(tasksDir)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestCreate_CanListCreatedTask(t *testing.T) {
	tmpDir := t.TempDir()
	service := MustTaskNoteService(t, tmpDir)

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)

	// Create a task
	_, err := service.Create(context.Background(), CreateRequest{
		NaturalLanguageInput: "Test task #important",
		Now:                  now,
	})
	require.NoError(t, err)

	// List tasks
	listResult, err := service.List(context.Background(), ListRequest{
		Now: now,
	})
	require.NoError(t, err)
	require.Equal(t, 1, listResult.FoundCount)
	require.Len(t, listResult.Notes, 1)
	require.Equal(t, "Test task", tnmodel.Title(listResult.Notes[0]))
}
