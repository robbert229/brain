package tnstorage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/robbert229/brain/internal/tnmodel"
)

// TaskNoteRepository provides access to TaskNotes.
type TaskNoteRepository interface {
	Crawl(ctx context.Context, fn func(*tnmodel.TaskNote) error) error
}

// DiskTaskNoteRepository reads TaskNotes from a directory on disk.
type DiskTaskNoteRepository struct {
	workingDirectory string
}

// NewDiskTaskNoteRepository creates a repository backed by workingDirectory.
func NewDiskTaskNoteRepository(workingDirectory string) DiskTaskNoteRepository {
	return DiskTaskNoteRepository{workingDirectory: workingDirectory}
}

// Crawl crawls all notes in the repository's working directory.
// It recursively walks the directory, finds all .md files,
// decodes them as TaskNotes, and invokes fn on each successfully decoded note.
// If fn returns an error, Crawl stops and returns that error.
func (repo DiskTaskNoteRepository) Crawl(ctx context.Context, fn func(*tnmodel.TaskNote) error) error {
	if _, err := os.Stat(repo.workingDirectory); err != nil {
		return fmt.Errorf("stat directory: %w", err)
	}

	return filepath.WalkDir(repo.workingDirectory, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("failed to walk directory: %w", err)
		}

		if d.IsDir() {
			return nil
		}

		if !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read file %q: %w", path, err)
		}

		taskID, err := filepath.Rel(repo.workingDirectory, path)
		if err != nil {
			return fmt.Errorf("get relative path: %w", err)
		}

		note, err := tnmodel.Decode(taskID, string(content))
		if err != nil {
			return fmt.Errorf("decode file %q: %w", path, err)
		}

		isTaskNote, err := CoincidenceDetector(ctx, note)
		if err != nil {
			return fmt.Errorf("detect coincidence: %w", err)
		}

		if !isTaskNote {
			return nil
		}

		if err := fn(note); err != nil {
			return fmt.Errorf("callback for file %q: %w", path, err)
		}

		return nil
	})
}

func CoincidenceDetector(ctx context.Context, note *tnmodel.TaskNote) (bool, error) {
	tags := tnmodel.Tags(note)
	if len(tags) == 0 {
		return false, nil
	}

	var foundTask bool
	for _, tag := range tags {
		if tag == "task" {
			foundTask = true
			break
		}
	}

	if !foundTask {
		return false, nil
	}

	return true, nil
}
