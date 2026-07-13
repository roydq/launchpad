package service

import (
	"context"
	"fmt"

	"github.com/launchpad/launchpad/internal/store"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

type ConfigService struct {
	store          *store.Store
	projectService *ProjectService
}

func NewConfigService(s *store.Store, projectService *ProjectService) *ConfigService {
	return &ConfigService{store: s, projectService: projectService}
}

// GetConfig returns resolved config (shared then service) unless layer is "shared" or "service".
func (s *ConfigService) GetConfig(ctx context.Context, projectName, envName, layer string) (map[string]string, error) {
	project, svc, env, err := s.projectService.resolvePrimaryService(ctx, projectName, envName)
	if err != nil {
		return nil, err
	}
	switch layer {
	case "", "resolved":
		return s.store.ResolveConfig(ctx, project.ID, svc.ID, env.ID)
	case "shared":
		return s.store.ListSharedConfigVars(ctx, project.ID, env.ID)
	case "service":
		return s.store.ListConfigVars(ctx, svc.ID, env.ID)
	default:
		return nil, fmt.Errorf("%w: layer must be shared, service, or resolved", launchpad.ErrBadRequest)
	}
}

// PatchConfig writes the service layer and returns resolved config.
func (s *ConfigService) PatchConfig(ctx context.Context, projectName, envName string, updates map[string]*string) (map[string]string, error) {
	project, svc, env, err := s.projectService.resolvePrimaryService(ctx, projectName, envName)
	if err != nil {
		return nil, err
	}
	if err := s.store.MergeConfigVars(ctx, svc.ID, env.ID, updates); err != nil {
		return nil, err
	}
	return s.store.ResolveConfig(ctx, project.ID, svc.ID, env.ID)
}
