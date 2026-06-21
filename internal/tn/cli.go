package tn

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/fatih/color"
)

var (
	colorCyan  = color.FgCyan
	colorBlue  = color.FgBlue
	colorWhite = color.FgWhite
)

type ListRequest struct {
	WorkingDirectory string

	Today     bool
	Overdue   bool
	Completed bool
	Filter    string
	JSON      bool
	Limit     int
}

func StubInteractiveMode(stdout io.Writer) error {
	_, err := fmt.Fprintln(stdout, "[stub] tn interactive mode")
	return err
}

func StubCreate(stdout io.Writer, args []string) error {
	_, err := fmt.Fprintf(stdout, "[stub] tn create: %s\n", strings.Join(args, " "))
	return err
}

func listPrintHeader(stdout io.Writer, req ListRequest, notes []*TaskNote) error {
	fmt.Fprintf(stdout, "%s Found %d tasks\n\n", color.CyanString("✔"), len(notes))
	if len(notes) == 0 {
		fmt.Fprintf(stdout, "\n\nℹ No tasks found matching your criteria\n")
		return nil
	}

	if req.Today {
		fmt.Fprintf(stdout, "Today's Tasks:\n")
	} else if req.Overdue {
		fmt.Fprintf(stdout, "Overdue Tasks:\n")
	} else if req.Completed {
		fmt.Fprintf(stdout, "Completed Tasks:\n")
	}

	fmt.Fprintf(stdout, "%s\n", strings.Repeat("─", 50))

	return nil
}

func listPrintTasks(stdout io.Writer, notes []*TaskNote) error {
	for i, note := range notes {
		if i != 0 {
			fmt.Fprintf(stdout, "\n\n")
		}

		err := listPrintTask(stdout, note)
		if err != nil {
			return fmt.Errorf("print task note: %w", err)
		}
	}

	return nil
}

func List(ctx context.Context, stdout io.Writer, req ListRequest) error {
	var notes []*TaskNote
	err := Crawl(ctx, req.WorkingDirectory, func(note *TaskNote) error {
		notes = append(notes, note)

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to crawl tasks: %w", err)
	}

	sort.Slice(notes, func(i, j int) bool {
		return notes[i].ID < notes[j].ID
	})

	err = listPrintHeader(stdout, req, notes)
	if err != nil {
		return fmt.Errorf("failed to print list header: %w", err)
	}

	err = listPrintTasks(stdout, notes)
	if err != nil {
		return fmt.Errorf("failed to print tasks: %w", err)
	}

	return nil
}

func StubComplete(stdout io.Writer, taskID string) error {
	_, err := fmt.Fprintf(stdout, "[stub] tn complete %s\n", taskID)
	return err
}

func StubToggle(stdout io.Writer, taskID string) error {
	_, err := fmt.Fprintf(stdout, "[stub] tn toggle %s\n", taskID)
	return err
}

func StubArchive(stdout io.Writer, taskID string) error {
	_, err := fmt.Fprintf(stdout, "[stub] tn archive %s\n", taskID)
	return err
}

func StubDelete(stdout io.Writer, taskID string, force bool) error {
	_, err := fmt.Fprintf(stdout, "[stub] tn delete %s force=%t\n", taskID, force)
	return err
}

func StubUpdate(
	stdout io.Writer,
	taskID, status, priority, due, addTags, removeTags, addContexts, addProjects string,
) error {
	_, err := fmt.Fprintf(
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
	return err
}

func StubSearch(stdout io.Writer, query string) error {
	_, err := fmt.Fprintf(stdout, "[stub] tn search %q\n", query)
	return err
}

func listPrintTask(stdout io.Writer, note *TaskNote) error {
	_, err := fmt.Fprintf(
		stdout,
		"%s %s\n",
		color.HiWhiteString("○ %s", note.Title),
		color.CyanString("[%s]", strings.ToUpper(note.Priority)),
	)
	if err != nil {
		return err
	}

	var tags string
	if len(note.Tags) > 0 {
		tags = "#" + strings.Join(note.Tags, " #")

	}

	_, err = fmt.Fprintf(stdout, "  Tags: %s\n", color.HiWhiteString(tags))
	if err != nil {
		return err
	}

	if note.Scheduled != nil {
		_, err = fmt.Fprintf(
			stdout,
			"  Scheduled: %s\n",
			color.BlueString(note.Scheduled.Format("2006-01-02 15:04")),
		)
		if err != nil {
			return err
		}
	}

	_, err = fmt.Fprintf(stdout, "  ID: %s\n", note.ID)
	if err != nil {
		return err
	}

	return nil
}
