package service

import (
	"context"
	"fmt"

	"github.com/launchpad/launchpad/internal/domain"
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

// TypedConfigEntry is the view=typed config response shape.
type TypedConfigEntry struct {
	Value       *string `json:"value"`
	Sensitivity string  `json:"sensitivity"`
	Set         bool    `json:"set"`
}

// GetConfig returns resolved or layer config with secret values redacted (***).
func (s *ConfigService) GetConfig(ctx context.Context, projectName, envName, layer string) (map[string]string, error) {
	vals, sens, err := s.getConfigRaw(ctx, projectName, envName, layer)
	if err != nil {
		return nil, err
	}
	return domain.RedactConfigMap(vals, sens), nil
}

// GetConfigTyped returns structured entries with redacted secret values.
func (s *ConfigService) GetConfigTyped(ctx context.Context, projectName, envName, layer string) (map[string]TypedConfigEntry, error) {
	vals, sens, err := s.getConfigRaw(ctx, projectName, envName, layer)
	if err != nil {
		return nil, err
	}
	out := make(map[string]TypedConfigEntry, len(vals))
	for k, v := range vals {
		sensitivity := sens[k]
		if sensitivity == "" {
			sensitivity = domain.SensitivityPlain
		}
		entry := TypedConfigEntry{Sensitivity: sensitivity, Set: true}
		if domain.IsSecret(sensitivity) {
			entry.Value = nil // write-only; presence via set=true
		} else {
			val := v
			entry.Value = &val
		}
		out[k] = entry
	}
	return out, nil
}

func (s *ConfigService) getConfigRaw(ctx context.Context, projectName, envName, layer string) (map[string]string, map[string]string, error) {
	project, svc, env, err := s.projectService.resolvePrimaryService(ctx, projectName, envName)
	if err != nil {
		return nil, nil, err
	}
	switch layer {
	case "", "resolved":
		return s.store.ResolveConfigWithSensitivity(ctx, project.ID, svc.ID, env.ID)
	case "shared":
		return s.store.ListSharedConfigVarsWithSensitivityTx(ctx, nil, project.ID, env.ID)
	case "service":
		return s.store.ListConfigVarsWithSensitivityTx(ctx, nil, svc.ID, env.ID)
	default:
		return nil, nil, fmt.Errorf("%w: layer must be shared, service, or resolved", launchpad.ErrBadRequest)
	}
}

// PatchConfig writes the service layer (sticky sensitivity) and returns redacted resolved config.
func (s *ConfigService) PatchConfig(ctx context.Context, projectName, envName string, updates map[string]*string) (map[string]string, error) {
	project, svc, env, err := s.projectService.resolvePrimaryService(ctx, projectName, envName)
	if err != nil {
		return nil, err
	}
	if err := s.store.MergeConfigVars(ctx, svc.ID, env.ID, updates); err != nil {
		return nil, err
	}
	vals, sens, err := s.store.ResolveConfigWithSensitivity(ctx, project.ID, svc.ID, env.ID)
	if err != nil {
		return nil, err
	}
	return domain.RedactConfigMap(vals, sens), nil
}
