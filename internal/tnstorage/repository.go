package tnstorage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/robbert229/brain/internal/tnmodel"
)

// Repository provides access to TaskNotes.
type Repository interface {
	Crawl(ctx context.Context, fn func(*tnmodel.TaskNote) error) error
	Save(ctx context.Context, note *tnmodel.TaskNote) error
}

// DiskTaskNoteRepository reads TaskNotes from a directory on disk.
type DiskTaskNoteRepository struct {
	workingDirectory string
}

type Config struct {
	workingDirectory string
}

func (c Config) validate() error {
	if c.workingDirectory == "" {
		return fmt.Errorf("working directory is required")
	}

	return nil
}

type Option func(cfg *Config)

func WithWorkingDirectory(workingDirectory string) Option {
	return func(cfg *Config) {
		cfg.workingDirectory = workingDirectory
	}
}

// NewDiskTaskNoteRepository creates a repository backed by workingDirectory.
func NewDiskTaskNoteRepository(opts ...Option) (DiskTaskNoteRepository, error) {
	cfg := &Config{}

	for _, opt := range opts {
		opt(cfg)
	}

	if err := cfg.validate(); err != nil {
		return DiskTaskNoteRepository{}, err
	}

	return DiskTaskNoteRepository{
		workingDirectory: cfg.workingDirectory,
	}, nil
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
			// instead of returning an error, we just skip the file. It's
			// possible that the note is not a TaskNote, so we don't want to
			// fail the entire crawl.
			return nil
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

// FilterFn is a function that filters the task notes.
type FilterFn func(*tnmodel.TaskNote) (bool, error)

// Filter returns the task notes that satisfy the filter.
func (repo DiskTaskNoteRepository) Filter(ctx context.Context, fn FilterFn) ([]*tnmodel.TaskNote, error) {
	var result []*tnmodel.TaskNote
	err := repo.Crawl(ctx, func(note *tnmodel.TaskNote) error {
		ok, err := fn(note)
		if err != nil {
			return fmt.Errorf("filter unable to handle note: %v %w", note, err)
		}

		if ok {
			result = append(result, note)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("unable to filter: %w", err)
	}

	return result, nil
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

// Save writes a TaskNote to disk.
func (repo DiskTaskNoteRepository) Save(ctx context.Context, note *tnmodel.TaskNote) error {
	if note == nil {
		return fmt.Errorf("note is required")
	}

	id := tnmodel.ID(note)
	if id == "" {
		return fmt.Errorf("note must have a file path")
	}

	path := filepath.Join(repo.workingDirectory, id)

	// Ensure the directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory %q: %w", dir, err)
	}

	// Encode the note
	content, err := tnmodel.Encode(note)
	if err != nil {
		return fmt.Errorf("encode note: %w", err)
	}

	// Write the file
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write file %q: %w", path, err)
	}

	return nil
}
