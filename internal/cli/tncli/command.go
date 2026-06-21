package tncli

import (
	"io"
	"os"

	"github.com/robbert229/brain/internal/tn"
	"github.com/spf13/cobra"
)

// NewCommand creates the tn command and all related subcommands.
func NewCommand(stdout io.Writer) *cobra.Command {
	var (
		listToday     bool
		listOverdue   bool
		listCompleted bool
		listFilter    string
		listJSON      bool
		listLimit     int

		deleteForce bool

		updateStatus      string
		updatePriority    string
		updateDue         string
		updateAddTags     string
		updateRemoveTags  string
		updateAddContexts string
		updateAddProjects string
	)

	tnCmd := &cobra.Command{
		Use:   "tn [input]",
		Short: "TaskNotes command group",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return tn.StubInteractiveMode(stdout)
			}

			return tn.StubCreate(stdout, args)
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}

			return tn.List(cmd.Context(), stdout, tn.ListRequest{
				WorkingDirectory: wd,

				Today:     listToday,
				Overdue:   listOverdue,
				Completed: listCompleted,
				Filter:    listFilter,
				JSON:      listJSON,
				Limit:     listLimit,
			})
		},
	}
	listCmd.Flags().BoolVar(&listToday, "today", false, "show tasks due today")
	listCmd.Flags().BoolVar(&listOverdue, "overdue", false, "show overdue tasks")
	listCmd.Flags().BoolVar(&listCompleted, "completed", false, "show completed tasks")
	listCmd.Flags().StringVar(&listFilter, "filter", "", "filter expression")
	listCmd.Flags().BoolVar(&listJSON, "json", false, "output as JSON")
	listCmd.Flags().IntVar(&listLimit, "limit", 20, "maximum number of tasks to return")

	completeCmd := &cobra.Command{
		Use:   "complete <taskId>",
		Short: "Mark a task complete",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return tn.StubComplete(stdout, args[0])
		},
	}

	toggleCmd := &cobra.Command{
		Use:   "toggle <taskId>",
		Short: "Toggle task completion",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return tn.StubToggle(stdout, args[0])
		},
	}

	archiveCmd := &cobra.Command{
		Use:   "archive <taskId>",
		Short: "Archive a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return tn.StubArchive(stdout, args[0])
		},
	}

	deleteCmd := &cobra.Command{
		Use:   "delete <taskId>",
		Short: "Delete a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return tn.StubDelete(stdout, args[0], deleteForce)
		},
	}
	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "delete without confirmation")

	updateCmd := &cobra.Command{
		Use:   "update <taskId>",
		Short: "Update a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return tn.StubUpdate(
				stdout,
				args[0],
				updateStatus,
				updatePriority,
				updateDue,
				updateAddTags,
				updateRemoveTags,
				updateAddContexts,
				updateAddProjects,
			)
		},
	}
	updateCmd.Flags().StringVar(&updateStatus, "status", "", "new status")
	updateCmd.Flags().StringVar(&updatePriority, "priority", "", "new priority")
	updateCmd.Flags().StringVar(&updateDue, "due", "", "new due date")
	updateCmd.Flags().StringVar(&updateAddTags, "add-tags", "", "comma-separated tags to add")
	updateCmd.Flags().StringVar(&updateRemoveTags, "remove-tags", "", "comma-separated tags to remove")
	updateCmd.Flags().StringVar(&updateAddContexts, "add-contexts", "", "comma-separated contexts to add")
	updateCmd.Flags().StringVar(&updateAddProjects, "add-projects", "", "comma-separated projects to add")

	searchCmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search tasks",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return tn.StubSearch(stdout, args[0])
		},
	}

	tnCmd.AddCommand(listCmd, completeCmd, toggleCmd, archiveCmd, deleteCmd, updateCmd, searchCmd)
	return tnCmd
}
