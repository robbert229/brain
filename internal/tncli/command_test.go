package tncli_test

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/robbert229/brain/internal/tncli"
	"github.com/robbert229/brain/internal/tnmodel"
	"github.com/robbert229/brain/internal/tnservice"
	"github.com/robbert229/brain/internal/tnstorage"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type CLIE2EScenario struct {
	Date         string `yaml:"date"`
	Cmd          string `yaml:"cmd"`
	OutputJSON   string `yaml:"output_json"`
	OutputStdout string `yaml:"output_stdout"`
}

type listStubArgs struct {
	today     bool
	overdue   bool
	completed bool
	filter    string
	json      bool
	limit     int
}

func parseListStubArgs(cmd string) (listStubArgs, error) {
	tokens := strings.Fields(strings.TrimSpace(cmd))
	if len(tokens) < 2 {
		return listStubArgs{}, errors.New("cmd must start with tn list")
	}
	if tokens[0] != "tn" || tokens[1] != "list" {
		return listStubArgs{}, errors.New("only tn list scenarios are supported")
	}

	args := listStubArgs{limit: 20}
	flags := flag.NewFlagSet("tn list", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.BoolVar(&args.today, "today", false, "")
	flags.BoolVar(&args.overdue, "overdue", false, "")
	flags.BoolVar(&args.completed, "completed", false, "")
	flags.StringVar(&args.filter, "filter", "", "")
	flags.BoolVar(&args.json, "json", false, "")
	flags.IntVar(&args.limit, "limit", 20, "")

	if err := flags.Parse(tokens[2:]); err != nil {
		return listStubArgs{}, err
	}
	if flags.NArg() != 0 {
		return listStubArgs{}, errors.New("unexpected positional arguments for tn list")
	}

	return args, nil
}

func TestPrintListJSON(t *testing.T) {
	const expected = `{
  "meta": {
	"filter": null,
	"today": false,
	"overdue": false,
	"completed": false,
	"limit": 20
  },
  "success": true,
  "data": {
	"tasks": [
	  {
		"path": "TaskNotes/Tasks/Work on 3d printers.md",
		"title": "Work on 3d printers",
		"status": "open",
		"priority": "low",
		"scheduled": "2026-06-20",
		"dateCreated": "2026-06-20T13:48:44.160-07:00",
		"dateModified": "2026-06-20T13:48:44.160-07:00",
		"tags": [
		  "task",
		  "test"
		],
		"archived": false,
		"id": "TaskNotes/Tasks/Work on 3d printers.md",
		"contexts": [],
		"projects": [],
		"totalTrackedTime": 0,
		"isBlocked": false,
		"isBlocking": false
	  },
      {
		"path": "TaskNotes/Tasks/Go grocery shopping later.md",
		"title": "Go grocery shopping later",
		"status": "open",
		"priority": "normal",
		"scheduled": "2026-06-20",
		"dateCreated": "2026-06-20T12:48:44.160-07:00",
		"dateModified": "2026-06-20T12:48:44.160-07:00",
		"tags": [
		  "task"
		],
		"archived": false,
		"id": "TaskNotes/Tasks/Go grocery shopping later.md",
		"contexts": [],
		"projects": [],
		"totalTrackedTime": 0,
		"isBlocked": false,
		"isBlocking": false
	  }
	]
  }
}`

	path := "TaskNotes/Tasks/Work on 3d printers.md"
	earlierPath := "TaskNotes/Tasks/Go grocery shopping later.md"
	priority := tnmodel.PriorityLow
	normalPriority := tnmodel.PriorityNormal
	scheduled := tnmodel.DateOrDateTime("2026-06-20")
	created := time.Date(2026, 6, 20, 13, 48, 44, 160000000, time.FixedZone("PDT", -7*3600))
	earlierCreated := created.Add(-time.Hour)

	task := &tnmodel.TaskNote{
		File: &tnmodel.TaskNoteFile{
			Path: &path,
		},
		Frontmatter: tnmodel.TaskFrontmatter{
			Title:        "Work on 3d printers",
			Status:       tnmodel.StatusOpen,
			Priority:     &priority,
			Scheduled:    &scheduled,
			DateCreated:  created,
			DateModified: created,
			Tags:         []string{"task", "test"},
			Contexts:     []string{},
			Projects:     []string{},
		},
	}
	earlierTask := &tnmodel.TaskNote{
		File: &tnmodel.TaskNoteFile{
			Path: &earlierPath,
		},
		Frontmatter: tnmodel.TaskFrontmatter{
			Title:        "Go grocery shopping later",
			Status:       tnmodel.StatusOpen,
			Priority:     &normalPriority,
			Scheduled:    &scheduled,
			DateCreated:  earlierCreated,
			DateModified: earlierCreated,
			Tags:         []string{"task"},
			Contexts:     []string{},
			Projects:     []string{},
		},
	}

	stdoutBuf := bytes.NewBuffer(nil)

	err := tncli.PrintList(stdoutBuf, tnservice.ListRequest{
		JSON:  true,
		Limit: 20,
	}, tnservice.ListResult{
		Notes:      []*tnmodel.TaskNote{task, earlierTask},
		FoundCount: 2,
	})
	require.NoError(t, err)
	require.JSONEq(t, expected, stdoutBuf.String())
}

func TestCLIE2E_Fixtures(t *testing.T) {
	root := filepath.Join("testdata", "cli-e2e", "list")

	entries, err := os.ReadDir(root)
	require.NoError(t, err)

	scenarios := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			scenarios = append(scenarios, entry.Name())
		}
	}

	require.NotEmpty(t, scenarios, "expected at least one cli-e2e scenario directory in %s", root)
	sort.Strings(scenarios)

	for _, scenario := range scenarios {
		scenario := scenario

		t.Run(scenario, func(t *testing.T) {
			scenarioDir := filepath.Join(root, scenario)

			info, err := os.Stat(scenarioDir)
			require.NoError(t, err)
			require.True(t, info.IsDir())

			envPath := filepath.Join(scenarioDir, "env.yaml")

			envBytes, err := os.ReadFile(envPath)
			require.NoError(t, err)
			require.NotEmpty(t, envBytes, "env.yaml must not be empty")

			var cfg CLIE2EScenario
			err = yaml.Unmarshal(envBytes, &cfg)
			require.NoError(t, err)
			require.NotEmpty(t, cfg.Cmd, "env cmd must not be empty")

			stubArgs, err := parseListStubArgs(cfg.Cmd)
			require.NoError(t, err)
			now, err := parseScenarioDate(cfg.Date)
			require.NoError(t, err)

			service := tnservice.NewTaskNoteService(tnstorage.NewDiskTaskNoteRepository(filepath.Join(scenarioDir, "vault")))

			req := tnservice.ListRequest{
				Today:     stubArgs.today,
				Overdue:   stubArgs.overdue,
				Completed: stubArgs.completed,
				Filter:    stubArgs.filter,
				JSON:      stubArgs.json,
				Limit:     stubArgs.limit,
				Now:       now,
			}
			result, err := service.List(t.Context(), req)
			require.NoError(t, err)

			stdoutBuf := bytes.NewBuffer(nil)

			err = tncli.PrintList(stdoutBuf, req, result)
			require.NoError(t, err)

			if cfg.OutputJSON != "" {
				require.JSONEq(t, cfg.OutputJSON, stdoutBuf.String())
			} else if cfg.OutputStdout != "" {
				require.Equal(t, cfg.OutputStdout, stdoutBuf.String())
			} else {
				t.Fatal("expected either output_json or output_stdout to be set")
			}

		})
	}
}

func parseScenarioDate(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}

	return time.Parse("Mon Jan 2 03:04:05 PM MST 2006", value)
}
