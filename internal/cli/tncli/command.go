package tncli

import (
	"io"
	"os"

	"github.com/robbert229/brain/internal/tn"
	"github.com/spf13/cobra"
)

type taskNoteServiceFactory func() (tn.TaskNoteService, error)

// NewCommand creates the tn command and all related subcommands.
func NewCommand(stdout io.Writer) *cobra.Command {
	newService := func() (tn.TaskNoteService, error) {
		wd, err := os.Getwd()
		if err != nil {
			return tn.TaskNoteService{}, err
		}

		return tn.NewTaskNoteService(tn.NewDiskTaskNoteRepository(wd)), nil
	}

	tnCmd := &cobra.Command{
		Use:   "tn [input]",
		Short: "TaskNotes command group",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return printStubInteractiveMode(stdout)
			}

			return printStubCreate(stdout, args)
		},
	}

	tnCmd.AddCommand(
		newListCommand(stdout, newService),
		newCompleteCommand(stdout),
		newToggleCommand(stdout),
		newArchiveCommand(stdout),
		newDeleteCommand(stdout),
		newUpdateCommand(stdout),
		newSearchCommand(stdout),
	)
	return tnCmd
}

func newListCommand(stdout io.Writer, newService taskNoteServiceFactory) *cobra.Command {
	var req tn.ListRequest

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			service, err := newService()
			if err != nil {
				return err
			}

			result, err := service.List(cmd.Context(), req)
			if err != nil {
				return err
			}

			return PrintList(stdout, req, result)
		},
	}
	cmd.Flags().BoolVar(&req.Today, "today", false, "show tasks due today")
	cmd.Flags().BoolVar(&req.Overdue, "overdue", false, "show overdue tasks")
	cmd.Flags().BoolVar(&req.Completed, "completed", false, "show completed tasks")
	cmd.Flags().StringVar(&req.Filter, "filter", "", "filter expression")
	cmd.Flags().BoolVar(&req.JSON, "json", false, "output as JSON")
	cmd.Flags().IntVar(&req.Limit, "limit", 20, "maximum number of tasks to return")

	return cmd
}

func newCompleteCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "complete <taskId>",
		Short: "Mark a task complete",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return printStubComplete(stdout, args[0])
		},
	}
}

func newToggleCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "toggle <taskId>",
		Short: "Toggle task completion",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return printStubToggle(stdout, args[0])
		},
	}
}

func newArchiveCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "archive <taskId>",
		Short: "Archive a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return printStubArchive(stdout, args[0])
		},
	}
}

func newDeleteCommand(stdout io.Writer) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete <taskId>",
		Short: "Delete a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return printStubDelete(stdout, args[0], force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "delete without confirmation")

	return cmd
}

func newUpdateCommand(stdout io.Writer) *cobra.Command {
	var (
		status      string
		priority    string
		due         string
		addTags     string
		removeTags  string
		addContexts string
		addProjects string
	)

	cmd := &cobra.Command{
		Use:   "update <taskId>",
		Short: "Update a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return printStubUpdate(
				stdout,
				args[0],
				status,
				priority,
				due,
				addTags,
				removeTags,
				addContexts,
				addProjects,
			)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "new status")
	cmd.Flags().StringVar(&priority, "priority", "", "new priority")
	cmd.Flags().StringVar(&due, "due", "", "new due date")
	cmd.Flags().StringVar(&addTags, "add-tags", "", "comma-separated tags to add")
	cmd.Flags().StringVar(&removeTags, "remove-tags", "", "comma-separated tags to remove")
	cmd.Flags().StringVar(&addContexts, "add-contexts", "", "comma-separated contexts to add")
	cmd.Flags().StringVar(&addProjects, "add-projects", "", "comma-separated projects to add")

	return cmd
}

func newSearchCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search tasks",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return printStubSearch(stdout, args[0])
		},
	}
}
