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
	"github.com/robbert229/brain/internal/tnservice"
	"github.com/robbert229/brain/internal/tnstorage"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type CLIE2EScenario struct {
	Date   string `yaml:"date"`
	Cmd    string `yaml:"cmd"`
	Output string `yaml:"output"`
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

			buf := bytes.NewBuffer(nil)
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

			err = tncli.PrintList(buf, req, result)
			require.NoError(t, err)

			require.Equal(t, cfg.Output, buf.String())
		})
	}
}

func parseScenarioDate(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}

	return time.Parse("Mon Jan 2 03:04:05 PM MST 2006", value)
}
