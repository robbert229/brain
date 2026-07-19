package tnservice

import (
	"strings"
	"testing"

	"github.com/robbert229/brain/internal/tnmodel"
	"github.com/stretchr/testify/require"
)

func TestListEndpoint_DispatchesToService(t *testing.T) {
	service := NewService(stubTaskNoteRepository{
		notes: []*tnmodel.TaskNote{
			{
				File: &tnmodel.TaskNoteFile{Path: stringPtr("from-endpoint.md")},
				Frontmatter: tnmodel.TaskFrontmatter{
					Title:  "From endpoint",
					Status: "open",
					Tags:   []string{"task"},
				},
			},
		},
	})
	endpoint := MakeListEndpoint(service)

	response, err := endpoint(t.Context(), ListRequest{})
	require.NoError(t, err)

	result, ok := response.(ListResponse)
	require.True(t, ok)
	require.Equal(t, 1, result.FoundCount)
	require.Equal(t, "From endpoint", tnmodel.Title(result.Notes[0]))
}

func TestListEndpoint_RejectsUnexpectedRequest(t *testing.T) {
	service := NewService(stubTaskNoteRepository{})
	endpoint := MakeListEndpoint(service)

	response, err := endpoint(t.Context(), "not a list request")

	require.Nil(t, response)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "expected tnservice.ListRequest"))
}
