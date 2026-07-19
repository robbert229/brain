package tnstorage

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func fixtureRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(wd, "testdata")
}

func fixturePaths(t *testing.T) []string {
	t.Helper()
	root := fixtureRoot(t)
	paths := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".yaml") {
			paths = append(paths, path)
		}
		return nil
	})
	require.NoError(t, err)

	sort.Strings(paths)
	return paths
}

func TestFindVault_FixturesArePresent(t *testing.T) {
	root := fixtureRoot(t)

	_, err := os.Stat(root)
	require.NoError(t, err)

	paths := fixturePaths(t)
	require.NotEmpty(t, paths)
}

type Scenario struct {
	Vault            string `yaml:"vault"`
	WorkingDirectory string `yaml:"workingDirectory"`
}

func TestFindVault(t *testing.T) {
	root := fixtureRoot(t)

	_, err := os.Stat(root)
	require.NoError(t, err)

	paths := fixturePaths(t)

	wd, err := os.Getwd()
	require.NoError(t, err)

	for _, path := range paths {
		testCaseFilePath, err := filepath.Rel(wd, path)
		require.NoError(t, err)

		t.Run(filepath.Dir(testCaseFilePath), func(t *testing.T) {
			testCaseYAML, err := os.ReadFile(testCaseFilePath)
			require.NoError(t, err)

			var scenario Scenario
			err = yaml.Unmarshal(testCaseYAML, &scenario)
			require.NoError(t, err)

			require.NotEmpty(t, scenario.WorkingDirectory)

			workingDirectoryABS, err := filepath.Abs(filepath.Join(filepath.Dir(testCaseFilePath), scenario.WorkingDirectory))
			require.NoError(t, err)

			scenarioVaultABS, err := filepath.Abs(filepath.Join(filepath.Dir(testCaseFilePath), scenario.Vault))
			require.NoError(t, err)

			vaultDir, err := FindVault(workingDirectoryABS)
			if scenario.Vault != "" {
				require.NoError(t, err)
				require.Equal(t, scenarioVaultABS, vaultDir)
			} else {
				require.Error(t, err, "since there is no vault FindVault should fail: %s", vaultDir)
			}
		})
	}
}
