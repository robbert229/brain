package tncli

import (
	"encoding/json"
	"fmt"
	"io"
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

func printStubInteractiveMode(stdout io.Writer) error {
	return printLine(stdout, "[stub] tn interactive mode")
}

func printStubCreate(stdout io.Writer, args []string) error {
	return printf(stdout, "[stub] tn create: %s\n", strings.Join(args, " "))
}

func printStubComplete(stdout io.Writer, taskID string) error {
	return printf(stdout, "[stub] tn complete %s\n", taskID)
}

func printStubToggle(stdout io.Writer, taskID string) error {
	return printf(stdout, "[stub] tn toggle %s\n", taskID)
}

func printStubArchive(stdout io.Writer, taskID string) error {
	return printf(stdout, "[stub] tn archive %s\n", taskID)
}

func printStubDelete(stdout io.Writer, taskID string, force bool) error {
	return printf(stdout, "[stub] tn delete %s force=%t\n", taskID, force)
}

func printStubUpdate(
	stdout io.Writer,
	taskID, status, priority, due, addTags, removeTags, addContexts, addProjects string,
) error {
	return printf(
		stdout,
		"[stub] tn update %s status=%q priority=%q due=%q add-tags=%q remove-tags=%q add-contexts=%q add-projects=%q\n",
		taskID,
		status,
		priority,
		due,
		addTags,
		removeTags,
		addContexts,
		addProjects,
	)
}

func printStubSearch(stdout io.Writer, query string) error {
	return printf(stdout, "[stub] tn search %q\n", query)
}

func PrintList(stdout io.Writer, req tnservice.ListRequest, result tnservice.ListResult) error {
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

func printListJSON(stdout io.Writer, req tnservice.ListRequest, result tnservice.ListResult) error {
	type metaResult struct {
		Filter    any  `json:"filter"`
		Today     bool `json:"today"`
		Overdue   bool `json:"overdue"`
		Completed bool `json:"completed"`
		Limit     int  `json:"limit"`
	}

	type dataResult struct {
		Tasks []*tnmodel.TaskNote `json:"tasks"`
	}

	type jsonResult struct {
		Meta    metaResult `json:"meta"`
		Success bool       `json:"success"`
		Data    dataResult `json:"data"`
	}

	enc := json.NewEncoder(stdout)
	return enc.Encode(jsonResult{
		Success: true,
		Data: dataResult{
			Tasks: result.Notes,
		},
		Meta: metaResult{
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

func printLine(stdout io.Writer, value string) error {
	_, err := fmt.Fprintln(stdout, value)
	return err
}

func printf(stdout io.Writer, format string, args ...any) error {
	_, err := fmt.Fprintf(stdout, format, args...)
	return err
}
