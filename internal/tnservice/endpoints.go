package tnservice

import (
	"context"
	"fmt"

	"github.com/go-kit/kit/endpoint"
)

// Endpoints collect the go-kit endpoints exposed by the TaskNotes service.
type Endpoints struct {
	List   endpoint.Endpoint
	Create endpoint.Endpoint
}

type taskNoteService interface {
	List(ctx context.Context, req ListRequest) (ListResponse, error)
	Create(ctx context.Context, req CreateRequest) (CreateResponse, error)
}

// NewTaskNoteEndpoints creates go-kit endpoints for the TaskNotes service.
func NewTaskNoteEndpoints(service taskNoteService) Endpoints {
	return Endpoints{
		List:   MakeListEndpoint(service),
		Create: MakeCreateEndpoint(service),
	}
}

// MakeListEndpoint adapts Service.List to a go-kit endpoint.
func MakeListEndpoint(service taskNoteService) endpoint.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req, ok := request.(ListRequest)
		if !ok {
			return nil, fmt.Errorf("expected %T, got %T", ListRequest{}, request)
		}

		return service.List(ctx, req)
	}
}

func MakeCreateEndpoint(service taskNoteService) endpoint.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req, ok := request.(CreateRequest)
		if !ok {
			return nil, fmt.Errorf("expected %T, got %T", CreateRequest{}, request)
		}

		return service.Create(ctx, req)
	}
}
