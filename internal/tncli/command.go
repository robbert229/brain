package tncli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-kit/kit/endpoint"
	"github.com/robbert229/brain/internal/tnservice"
	"github.com/robbert229/brain/internal/tnstorage"
	"github.com/spf13/cobra"
)

type endpointFactory func() (endpoint.Endpoint, error)

type endpointSetFactory func() (tnservice.Endpoints, error)

var ErrNotImplemented = errors.New("not implemented")

func endpointAdapter(factory endpointSetFactory, selector func(endpoints tnservice.Endpoints) endpoint.Endpoint) endpointFactory {
	return func() (endpoint.Endpoint, error) {
		set, err := factory()
		if err != nil {
			return nil, err
		}

		return selector(set), nil
	}
}

// NewCommand creates the tn command and all related subcommands.
func NewCommand(stdout io.Writer) *cobra.Command {
	endpointSetFactory := func() (tnservice.Endpoints, error) {
		wd, err := os.Getwd()
		if err != nil {
			return tnservice.Endpoints{}, err
		}

		vaultDir, err := tnstorage.FindVault(wd)
		if err != nil {
			if !errors.Is(err, tnstorage.ErrVaultNotFound) {
				return tnservice.Endpoints{}, err
			}

			vaultDir = wd
		}

		repository, err := tnstorage.NewDiskTaskNoteRepository(
			tnstorage.WithWorkingDirectory(filepath.Join(vaultDir, "../")),
		)
		if err != nil {
			return tnservice.Endpoints{}, err
		}

		service := tnservice.NewService(repository)
		return tnservice.NewTaskNoteEndpoints(service), nil
	}

	tnCmd := newCreateCommand(stdout, endpointAdapter(endpointSetFactory, func(endpoints tnservice.Endpoints) endpoint.Endpoint { return endpoints.Create }))

	tnCmd.AddCommand(
		newListCommand(stdout, endpointAdapter(endpointSetFactory, func(endpoints tnservice.Endpoints) endpoint.Endpoint { return endpoints.List })),
		newCompleteCommand(stdout),
		newToggleCommand(stdout),
		newArchiveCommand(stdout),
		newDeleteCommand(stdout),
		newUpdateCommand(stdout),
		newSearchCommand(stdout),
	)
	return tnCmd
}

func newCreateCommand(stdout io.Writer, factory endpointFactory) *cobra.Command {
	var req tnservice.CreateRequest

	tnCmd := &cobra.Command{
		Use:   "tn [input]",
		Short: "TaskNotes command group",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return notImplemented("tn interactive")
			}

			// Join all arguments to form the natural language input
			req.NaturalLanguageInput = strings.Join(args, " ")

			createEndpoint, err := factory()
			if err != nil {
				return err
			}

			response, err := createEndpoint(cmd.Context(), req)
			if err != nil {
				return err
			}

			result, ok := response.(tnservice.CreateResponse)
			if !ok {
				return fmt.Errorf("expected %T, got %T", tnservice.CreateResponse{}, response)
			}

			return PrintCreate(stdout, result)
		},
	}

	return tnCmd
}

func newListCommand(stdout io.Writer, factory endpointFactory) *cobra.Command {
	var req tnservice.ListRequest

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			listEndpoint, err := factory()
			if err != nil {
				return err
			}

			response, err := listEndpoint(cmd.Context(), req)
			if err != nil {
				return err
			}

			result, ok := response.(tnservice.ListResponse)
			if !ok {
				return fmt.Errorf("expected %T, got %T", tnservice.ListResponse{}, response)
			}

			return PrintList(stdout, req, result)
		},
	}
	cmd.Flags().BoolVar(&req.Today, "today", false, "show tasks due/scheduled for today")
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
			return notImplemented("tn complete")
		},
	}
}

func newToggleCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "toggle <taskId>",
		Short: "Toggle task completion",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return notImplemented("tn toggle")
		},
	}
}

func newArchiveCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "archive <taskId>",
		Short: "Archive a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return notImplemented("tn archive")
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
			return notImplemented("tn delete")
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
			return notImplemented("tn update")
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
			return notImplemented("tn search")
		},
	}
}

func notImplemented(command string) error {
	return fmt.Errorf("%s: %w", command, ErrNotImplemented)
}
