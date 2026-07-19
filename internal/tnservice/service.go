package tnservice

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/robbert229/brain/internal/tnmodel"
	"github.com/robbert229/brain/internal/tnnlpcore"
	"github.com/robbert229/brain/internal/tnstorage"
)

type ListRequest struct {
	Today     bool
	Overdue   bool
	Completed bool
	Filter    string
	JSON      bool
	Limit     int
	Now       time.Time
}

type ListResponse struct {
	Notes      []*tnmodel.TaskNote
	FoundCount int
}

type Service struct {
	Repository tnstorage.Repository
	Parser     *tnnlpcore.NaturalLanguageParserCore
}

func NewService(repository tnstorage.Repository) Service {
	parserOpts := tnnlpcore.ParserOptions{}
	parserOpts.DateLocale = "en-US"

	return Service{
		Repository: repository,
		Parser: tnnlpcore.NewNaturalLanguageParserCore(
			nil,
			nil,
			true,
			"EN-us",
			nil,
			nil,
			parserOpts,
		),
	}
}

func getNow(now time.Time) time.Time {
	if now.IsZero() {
		now = time.Now()
	}

	return now
}

func (s Service) List(ctx context.Context, req ListRequest) (ListResponse, error) {
	now := getNow(req.Now)

	repo := s.Repository
	if repo == nil {
		return ListResponse{}, fmt.Errorf("task note repository is required")
	}

	var notes []*tnmodel.TaskNote
	err := repo.Crawl(ctx, func(note *tnmodel.TaskNote) error {
		if !matchesListRequest(note, req, now) {
			return nil
		}

		notes = append(notes, note)

		return nil
	})
	if err != nil {
		return ListResponse{}, fmt.Errorf("failed to crawl tasks: %w", err)
	}

	sort.Slice(notes, func(i, j int) bool {
		return tnmodel.ID(notes[i]) < tnmodel.ID(notes[j])
	})

	foundCount := len(notes)
	notes = limitTaskNotes(notes, req.Limit)

	return ListResponse{
		Notes:      notes,
		FoundCount: foundCount,
	}, nil
}

func matchesListRequest(note *tnmodel.TaskNote, req ListRequest, now time.Time) bool {
	if req.Today && !isTaskToday(note, now) {
		return false
	}
	if req.Overdue && !isTaskOverdue(note, now) {
		return false
	}
	if req.Completed && !isTaskCompleted(note) {
		return false
	}

	return true
}

func limitTaskNotes(notes []*tnmodel.TaskNote, limit int) []*tnmodel.TaskNote {
	if limit <= 0 || len(notes) <= limit {
		return notes
	}

	return notes[:limit]
}

func isTaskToday(note *tnmodel.TaskNote, now time.Time) bool {
	return sameDate(tnmodel.Due(note), now) || sameDate(tnmodel.Scheduled(note), now)
}

func isTaskOverdue(note *tnmodel.TaskNote, now time.Time) bool {
	due := tnmodel.Due(note)
	if due == nil || isTaskCompleted(note) {
		return false
	}

	return compareDate(due, now) < 0
}

func isTaskCompleted(note *tnmodel.TaskNote) bool {
	switch strings.ToLower(strings.TrimSpace(tnmodel.Status(note))) {
	case tnmodel.StatusClosed, tnmodel.StatusCompleted, tnmodel.StatusDone:
		return true
	default:
		return false
	}
}

func sameDate(t *time.Time, now time.Time) bool {
	if t == nil {
		return false
	}

	return compareDate(t, now) == 0
}

func compareDate(t *time.Time, now time.Time) int {
	taskYear, taskMonth, taskDay := t.Date()
	nowYear, nowMonth, nowDay := now.Date()
	taskDate := time.Date(taskYear, taskMonth, taskDay, 0, 0, 0, 0, time.UTC)
	today := time.Date(nowYear, nowMonth, nowDay, 0, 0, 0, 0, time.UTC)

	if taskDate.Before(today) {
		return -1
	}
	if taskDate.After(today) {
		return 1
	}

	return 0
}

type CreateRequest struct {
	NaturalLanguageInput string
	Now                  time.Time
}

type CreateResponse struct {
	TaskNote *tnmodel.TaskNote
}

func (s Service) Create(ctx context.Context, req CreateRequest) (CreateResponse, error) {
	now := getNow(req.Now)

	if s.Repository == nil {
		return CreateResponse{}, fmt.Errorf("task note repository is required")
	}

	// Parse the natural language input
	parsed := s.Parser.ParseInput(req.NaturalLanguageInput)

	// Create the task note from parsed data
	note := &tnmodel.TaskNote{
		Frontmatter: tnmodel.TaskFrontmatter{
			Title:        parsed.Title,
			Status:       tnmodel.StatusOpen,
			Tags:         append([]string{"task"}, parsed.Tags...),
			Contexts:     parsed.Contexts,
			Projects:     parsed.Projects,
			DateCreated:  now,
			DateModified: now,
		},
	}

	// Set status if provided
	if parsed.Status != "" {
		note.Frontmatter.Status = parsed.Status
	}

	// Set priority if provided
	if parsed.Priority != "" {
		note.Frontmatter.Priority = &parsed.Priority
	}

	// Set due date if provided
	if parsed.DueDate != "" {
		dueDateTime := tnmodel.DateOrDateTime(parsed.DueDate)
		if parsed.DueTime != "" {
			dueDateTime = tnmodel.DateOrDateTime(parsed.DueDate + "T" + parsed.DueTime)
		}
		note.Frontmatter.Due = &dueDateTime
	}

	// Set scheduled date if provided
	if parsed.ScheduledDate != "" {
		scheduledDateTime := tnmodel.DateOrDateTime(parsed.ScheduledDate)
		if parsed.ScheduledTime != "" {
			scheduledDateTime = tnmodel.DateOrDateTime(parsed.ScheduledDate + "T" + parsed.ScheduledTime)
		}
		note.Frontmatter.Scheduled = &scheduledDateTime
	}

	// Set time estimate if provided
	if parsed.Estimate > 0 {
		note.Frontmatter.TimeEstimate = &parsed.Estimate
	}

	// Set recurrence if provided
	if parsed.Recurrence != "" {
		note.Frontmatter.Recurrence = &parsed.Recurrence
	}

	// Set details/body if provided
	if parsed.Details != "" {
		note.Body = &parsed.Details
	}

	// Generate a filename based on the title
	filename := generateFilename(parsed.Title, now)
	note.File = &tnmodel.TaskNoteFile{
		Path: &filename,
	}

	// Save the task note
	if err := s.Repository.Save(ctx, note); err != nil {
		return CreateResponse{}, fmt.Errorf("failed to save task: %w", err)
	}

	return CreateResponse{
		TaskNote: note,
	}, nil
}

// generateFilename creates a safe filename from a task title and timestamp
func generateFilename(title string, now time.Time) string {
	// Use timestamp prefix to ensure uniqueness
	timestamp := now.Format("20060102-150405")

	// Sanitize the title for use in a filename
	sanitized := strings.TrimSpace(title)
	if sanitized == "" {
		sanitized = "untitled"
	}

	// Replace unsafe characters
	sanitized = strings.Map(func(r rune) rune {
		if r == ' ' {
			return '-'
		}
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return '-'
		}
		return r
	}, sanitized)

	// Limit length
	maxLen := 50
	if len(sanitized) > maxLen {
		sanitized = sanitized[:maxLen]
	}

	// Remove trailing dashes
	sanitized = strings.TrimRight(sanitized, "-")

	return fmt.Sprintf("tasks/%s-%s.md", timestamp, sanitized)
}
