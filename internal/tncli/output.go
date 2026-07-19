package tncli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/robbert229/brain/internal/tnmodel"
	"github.com/robbert229/brain/internal/tnservice"
)

const (
	listDividerWidth = 50
	taskDateLayout   = "2006-01-02 15:04"
)

// PrintList prints out the response header and list of tasks.
func PrintList(stdout io.Writer, req tnservice.ListRequest, result tnservice.ListResponse) error {
	if req.JSON {
		return printListJSON(stdout, req, result)
	}

	err := printListHeader(stdout, req, result.FoundCount)
	if err != nil {
		return fmt.Errorf("failed to print list header: %w", err)
	}

	err = printListTasks(stdout, result.Notes)
	if err != nil {
		return fmt.Errorf("failed to print tasks: %w", err)
	}

	return nil
}

// DTOMeta is a struct for the JSON response metadata.
type DTOMeta struct {
	Filter    any  `json:"filter"`
	Today     bool `json:"today"`
	Overdue   bool `json:"overdue"`
	Completed bool `json:"completed"`
	Limit     int  `json:"limit"`
}

// DTOOutput is a struct for the JSON response output.
type DTOOutput struct {
	Meta    DTOMeta `json:"meta"`
	Success bool    `json:"success"`
	Data    DTOData `json:"data"`
}

// DTOData is a struct for the JSON response data.
type DTOData struct {
	Tasks []DTOTask `json:"tasks"`
}

// DTOTask is a struct for the JSON response task.
type DTOTask struct {
	Path             string   `json:"path"`
	Title            string   `json:"title"`
	Status           string   `json:"status"`
	Priority         string   `json:"priority"`
	Due              string   `json:"due,omitempty"`
	Scheduled        string   `json:"scheduled,omitempty"`
	DateCreated      string   `json:"dateCreated"`
	DateModified     string   `json:"dateModified"`
	CompletedDate    string   `json:"completedDate,omitempty"`
	Tags             []string `json:"tags"`
	Archived         bool     `json:"archived"`
	ID               string   `json:"id"`
	Contexts         []string `json:"contexts"`
	Projects         []string `json:"projects"`
	TotalTrackedTime int      `json:"totalTrackedTime"`
	IsBlocked        bool     `json:"isBlocked"`
	IsBlocking       bool     `json:"isBlocking"`
}

// DTOFromTask converts a domain model into a dto task.
func DTOFromTask(note *tnmodel.TaskNote) DTOTask {
	due := tnmodel.Due(note)
	scheduled := tnmodel.Scheduled(note)

	contexts := note.Frontmatter.Contexts
	if len(contexts) == 0 {
		contexts = []string{}
	}

	projects := note.Frontmatter.Projects
	if len(projects) == 0 {
		projects = []string{}
	}

	return DTOTask{
		Path:             tnmodel.ID(note),
		Title:            tnmodel.Title(note),
		Status:           tnmodel.Status(note),
		Priority:         tnmodel.Priority(note),
		Due:              formatTaskTime(due, "2006-01-02"),
		Scheduled:        formatTaskTime(scheduled, "2006-01-02"),
		DateCreated:      formatRequiredTaskTime(note.Frontmatter.DateCreated),
		DateModified:     formatRequiredTaskTime(note.Frontmatter.DateModified),
		CompletedDate:    formatTaskCompletedDate(note.Frontmatter.CompletedDate),
		Tags:             tnmodel.Tags(note),
		Archived:         false,
		ID:               tnmodel.ID(note),
		Contexts:         contexts,
		Projects:         projects,
		TotalTrackedTime: 0,
		IsBlocked:        len(note.Frontmatter.BlockedBy) > 0,
		IsBlocking:       false,
	}
}

func formatTaskTime(value *time.Time, layout string) string {
	if value == nil {
		return ""
	}

	return value.Format(layout)
}

func formatRequiredTaskTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}

	return value.Format("2006-01-02T15:04:05.000-07:00")
}

func formatTaskCompletedDate(value *tnmodel.Date) string {
	if value == nil || value.IsZero() {
		return ""
	}

	return (*value).Format("2006-01-02")
}

func DTOSFromTasks(notes []*tnmodel.TaskNote) []DTOTask {
	tasks := make([]DTOTask, len(notes))
	for i, note := range notes {
		tasks[i] = DTOFromTask(note)
	}
	return tasks
}

func printListJSON(stdout io.Writer, req tnservice.ListRequest, result tnservice.ListResponse) error {
	tasks := DTOSFromTasks(result.Notes)
	sort.SliceStable(tasks, func(i, j int) bool {
		return tasks[i].DateCreated > tasks[j].DateCreated
	})

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")

	return enc.Encode(DTOOutput{
		Success: true,
		Data: DTOData{
			Tasks: tasks,
		},
		Meta: DTOMeta{
			Filter:    nil,
			Today:     req.Today,
			Overdue:   req.Overdue,
			Completed: req.Completed,
			Limit:     req.Limit,
		},
	})
}

func printListHeader(stdout io.Writer, req tnservice.ListRequest, foundCount int) error {
	if err := printf(stdout, "%s Found %d tasks\n\n", color.CyanString("✔"), foundCount); err != nil {
		return err
	}

	if foundCount == 0 {
		return printf(stdout, "\n\nℹ No tasks found matching your criteria\n")
	}

	if title := listTitle(req); title != "" {
		if err := printf(stdout, "%s\n", title); err != nil {
			return err
		}
	}

	return printf(stdout, "%s\n", strings.Repeat("─", listDividerWidth))
}

func printListTasks(stdout io.Writer, notes []*tnmodel.TaskNote) error {
	for i, note := range notes {
		if i != 0 {
			if err := printf(stdout, "\n\n"); err != nil {
				return err
			}
		}

		err := printListTask(stdout, note)
		if err != nil {
			return fmt.Errorf("print task note: %w", err)
		}
	}

	return nil
}

func printListTask(stdout io.Writer, note *tnmodel.TaskNote) error {
	if err := printListTaskTitle(stdout, note); err != nil {
		return err
	}

	if err := printListTaskTags(stdout, tnmodel.Tags(note)); err != nil {
		return err
	}

	if scheduled := tnmodel.Scheduled(note); scheduled != nil {
		if err := printListTaskScheduled(stdout, *scheduled); err != nil {
			return err
		}
	}

	return printListTaskID(stdout, tnmodel.ID(note))
}

func printListTaskTitle(stdout io.Writer, note *tnmodel.TaskNote) error {
	return printf(
		stdout,
		"%s %s\n",
		color.HiWhiteString("○ %s", tnmodel.Title(note)),
		color.CyanString("[%s]", strings.ToUpper(tnmodel.Priority(note))),
	)
}

func printListTaskTags(stdout io.Writer, tags []string) error {
	return printf(stdout, "  Tags: %s\n", color.HiWhiteString(formatTags(tags)))
}

func printListTaskScheduled(stdout io.Writer, scheduled time.Time) error {
	return printf(stdout, "  Scheduled: %s\n", color.BlueString(scheduled.Format(taskDateLayout)))
}

func printListTaskID(stdout io.Writer, id string) error {
	return printf(stdout, "  ID: %s\n", id)
}

func listTitle(req tnservice.ListRequest) string {
	switch {
	case req.Today:
		return "Today's Tasks:"
	case req.Overdue:
		return "Overdue Tasks:"
	case req.Completed:
		return "Completed Tasks:"
	default:
		return ""
	}
}

func formatTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}

	return "#" + strings.Join(tags, " #")
}

func printf(stdout io.Writer, format string, args ...any) error {
	_, err := fmt.Fprintf(stdout, format, args...)
	return err
}

// PrintCreate prints out the created task.
func PrintCreate(stdout io.Writer, result tnservice.CreateResponse) error {
	note := result.TaskNote
	if note == nil {
		return fmt.Errorf("created task is nil")
	}

	if err := printf(stdout, "%s Task created successfully!\n\n", color.GreenString("✔")); err != nil {
		return err
	}

	// Print task details
	if err := printf(stdout, "%s %s\n", color.HiWhiteString("Title:"), tnmodel.Title(note)); err != nil {
		return err
	}

	if tnmodel.Status(note) != "" {
		if err := printf(stdout, "%s %s\n", color.HiWhiteString("Status:"), tnmodel.Status(note)); err != nil {
			return err
		}
	}

	if tnmodel.Priority(note) != "" {
		if err := printf(stdout, "%s %s\n", color.HiWhiteString("Priority:"), tnmodel.Priority(note)); err != nil {
			return err
		}
	}

	if len(tnmodel.Tags(note)) > 0 {
		if err := printf(stdout, "%s %s\n", color.HiWhiteString("Tags:"), formatTags(tnmodel.Tags(note))); err != nil {
			return err
		}
	}

	if len(note.Frontmatter.Contexts) > 0 {
		if err := printf(stdout, "%s %s\n", color.HiWhiteString("Contexts:"), formatContexts(note.Frontmatter.Contexts)); err != nil {
			return err
		}
	}

	if len(note.Frontmatter.Projects) > 0 {
		if err := printf(stdout, "%s %s\n", color.HiWhiteString("Projects:"), formatProjects(note.Frontmatter.Projects)); err != nil {
			return err
		}
	}

	if scheduled := tnmodel.Scheduled(note); scheduled != nil {
		if err := printf(stdout, "%s %s\n", color.HiWhiteString("Scheduled:"), scheduled.Format(taskDateLayout)); err != nil {
			return err
		}
	}

	if due := tnmodel.Due(note); due != nil {
		if err := printf(stdout, "%s %s\n", color.HiWhiteString("Due:"), due.Format(taskDateLayout)); err != nil {
			return err
		}
	}

	if err := printf(stdout, "%s %s\n", color.HiWhiteString("File:"), tnmodel.ID(note)); err != nil {
		return err
	}

	return nil
}

func formatContexts(contexts []string) string {
	if len(contexts) == 0 {
		return ""
	}
	return "@" + strings.Join(contexts, " @")
}

func formatProjects(projects []string) string {
	if len(projects) == 0 {
		return ""
	}
	return "+" + strings.Join(projects, " +")
}
