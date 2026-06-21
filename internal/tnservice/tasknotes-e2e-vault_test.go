package tnservice_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/robbert229/brain/internal/tnmodel"
	"github.com/stretchr/testify/require"
)

func fixtureRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(wd, "testdata", "tasknotes-e2e-vault")
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
		if strings.EqualFold(filepath.Ext(path), ".md") {
			paths = append(paths, path)
		}
		return nil
	})
	require.NoError(t, err)

	sort.Strings(paths)
	return paths
}

func TestTaskNotesFixtures_ArePresent(t *testing.T) {
	root := fixtureRoot(t)

	_, err := os.Stat(root)
	require.NoError(t, err)

	paths := fixturePaths(t)
	require.NotEmpty(t, paths)
}

func TestTaskNotesFixtures_Decode(t *testing.T) {
	for _, path := range fixturePaths(t) {
		path := path
		rel := strings.TrimPrefix(path, fixtureRoot(t)+string(filepath.Separator))
		t.Run(rel, func(t *testing.T) {
			content, err := os.ReadFile(path)
			require.NoError(t, err)

			note, err := tnmodel.Decode("", string(content))
			require.NoError(t, err)
			require.NotNil(t, note)
		})
	}
}

func TestTaskNotesFixtures_EncodeDecode_RoundTrip(t *testing.T) {
	for _, path := range fixturePaths(t) {
		path := path
		rel := strings.TrimPrefix(path, fixtureRoot(t)+string(filepath.Separator))

		t.Run(rel, func(t *testing.T) {
			content, err := os.ReadFile(path)
			require.NoError(t, err)

			taskID := "TaskNotes/Tasks/TestTask.md"

			decoded, err := tnmodel.Decode(taskID, string(content))
			require.NoError(t, err)

			encoded, err := tnmodel.Encode(decoded)
			require.NoError(t, err)

			reDecoded, err := tnmodel.Decode(taskID, encoded)
			require.NoError(t, err)

			require.Equal(t, decoded.Status, reDecoded.Status)
			require.Equal(t, decoded.Priority, reDecoded.Priority)
		})
	}
}
