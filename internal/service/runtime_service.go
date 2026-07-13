package service

import (
	"context"
	"fmt"
	"io"

	"github.com/launchpad/launchpad/internal/store"
	"github.com/launchpad/launchpad/internal/target"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

// RuntimeService exposes target-backed runtime ops (logs, future status).
type RuntimeService struct {
	store          *store.Store
	projectService *ProjectService
	registry       *target.Registry
}

func NewRuntimeService(s *store.Store, projectService *ProjectService, registry *target.Registry) *RuntimeService {
	return &RuntimeService{store: s, projectService: projectService, registry: registry}
}

func (s *RuntimeService) Logs(ctx context.Context, projectName, envName, processName string) (io.ReadCloser, error) {
	if processName == "" {
		processName = "web"
	}
	project, svc, env, err := s.projectService.resolvePrimaryService(ctx, projectName, envName)
	if err != nil {
		return nil, err
	}
	backend, err := s.registry.Get(env.TargetType)
	if err != nil {
		return nil, fmt.Errorf("%w: target %q: %v", launchpad.ErrBadRequest, env.TargetType, err)
	}
	return backend.Logs(ctx, target.LogsRequest{
		Project:     *project,
		Service:     *svc,
		Environment: *env,
		ProcessName: processName,
	})
}
