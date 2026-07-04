package tnservice

import (
	"context"
	"fmt"

	"github.com/go-kit/kit/endpoint"
)

// Endpoints collect the go-kit endpoints exposed by the TaskNotes service.
type Endpoints struct {
	List endpoint.Endpoint
}

type taskNoteLister interface {
	List(ctx context.Context, req ListRequest) (ListResult, error)
}

// NewTaskNoteEndpoints creates go-kit endpoints for the TaskNotes service.
func NewTaskNoteEndpoints(service taskNoteLister) Endpoints {
	return Endpoints{
		List: MakeListEndpoint(service),
	}
}

// MakeListEndpoint adapts Service.List to a go-kit endpoint.
func MakeListEndpoint(service taskNoteLister) endpoint.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req, ok := request.(ListRequest)
		if !ok {
			return nil, fmt.Errorf("expected %T, got %T", ListRequest{}, request)
		}

		return service.List(ctx, req)
	}
}
