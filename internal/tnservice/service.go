package tnservice

import (
	"context"
	"fmt"
	"go/parser"
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

type ListResult struct {
	Notes      []*tnmodel.TaskNote
	FoundCount int
}

type Service struct {
	Repository tnstorage.TaskNoteRepository
	Parser     *tnnlpcore.NaturalLanguageParserCore
}

func NewTaskNoteService(repository tnstorage.TaskNoteRepository) Service {
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

func (s Service) List(ctx context.Context, req ListRequest) (ListResult, error) {
	now := getNow(req.Now)

	repo := s.Repository
	if repo == nil {
		return ListResult{}, fmt.Errorf("task note repository is required")
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
		return ListResult{}, fmt.Errorf("failed to crawl tasks: %w", err)
	}

	sort.Slice(notes, func(i, j int) bool {
		return tnmodel.ID(notes[i]) < tnmodel.ID(notes[j])
	})

	foundCount := len(notes)
	notes = limitTaskNotes(notes, req.Limit)

	return ListResult{
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

	parsed := s.Parser.ParseInput(req.NaturalLanguageInput)
	
}
