package service

import (
	"context"

	"github.com/launchpad/launchpad/internal/store"
)

type ConfigService struct {
	store          *store.Store
	projectService *ProjectService
}

func NewConfigService(s *store.Store, projectService *ProjectService) *ConfigService {
	return &ConfigService{store: s, projectService: projectService}
}

func (s *ConfigService) GetConfig(ctx context.Context, projectName string) (map[string]string, error) {
	_, svc, env, err := s.projectService.resolvePrimaryService(ctx, projectName)
	if err != nil {
		return nil, err
	}
	return s.store.ListConfigVars(ctx, svc.ID, env.ID)
}

func (s *ConfigService) PatchConfig(ctx context.Context, projectName string, updates map[string]*string) (map[string]string, error) {
	_, svc, env, err := s.projectService.resolvePrimaryService(ctx, projectName)
	if err != nil {
		return nil, err
	}
	if err := s.store.MergeConfigVars(ctx, svc.ID, env.ID, updates); err != nil {
		return nil, err
	}
	return s.store.ListConfigVars(ctx, svc.ID, env.ID)
}