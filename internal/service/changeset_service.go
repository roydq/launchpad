package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/store"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

type ChangesetService struct {
	store          *store.Store
	projectService *ProjectService
	releaseService *ReleaseService
}

func NewChangesetService(s *store.Store, projectService *ProjectService, releaseService *ReleaseService) *ChangesetService {
	return &ChangesetService{store: s, projectService: projectService, releaseService: releaseService}
}

type StageChangeInput struct {
	Type     string  `json:"type"`
	Key      string  `json:"key,omitempty"`
	Value    *string `json:"value,omitempty"`
	Process  string  `json:"process,omitempty"`
	Quantity *int    `json:"quantity,omitempty"`
	Image    string  `json:"image,omitempty"`
	// Layer is "service" (default) or "shared" for config-type changes.
	Layer string `json:"layer,omitempty"`
	// Sensitivity is optional "plain" | "secret" for config changes.
	Sensitivity *string `json:"sensitivity,omitempty"`
}

type StageChangesInput struct {
	Service string             `json:"service,omitempty"`
	Changes []StageChangeInput `json:"changes"`
}

type PushChangesetInput struct {
	Description string `json:"description"`
}

// ChangesetView is the open changeset plus resolved environment name for API/CLI.
type ChangesetView struct {
	*domain.Changeset
	EnvironmentName string
}

func (s *ChangesetService) GetChangeset(ctx context.Context, projectName, envName string) (*ChangesetView, error) {
	project, err := s.projectService.GetProject(ctx, projectName)
	if err != nil {
		return nil, err
	}
	cs, err := s.store.GetOpenChangeset(ctx, project.ID)
	if err != nil {
		if errors.Is(err, launchpad.ErrNotFound) {
			return &ChangesetView{
				Changeset: &domain.Changeset{
					ProjectID: project.ID,
					Status:    domain.ChangesetOpen,
					Changes:   []domain.ChangesetChange{},
				},
			}, nil
		}
		return nil, err
	}
	return s.viewFor(ctx, cs)
}

func (s *ChangesetService) StageChanges(ctx context.Context, projectName, envName string, input StageChangesInput) (*ChangesetView, error) {
	if len(input.Changes) == 0 {
		return nil, fmt.Errorf("%w: at least one change required", launchpad.ErrBadRequest)
	}
	project, svc, env, err := s.projectService.resolvePrimaryService(ctx, projectName, envName)
	if err != nil {
		return nil, err
	}
	if input.Service != "" && input.Service != project.PrimaryService {
		return nil, fmt.Errorf("%w: service must match primary service %q", launchpad.ErrBadRequest, project.PrimaryService)
	}

	err = s.store.Transact(ctx, func(tx *sql.Tx) error {
		open, err := s.store.GetOrCreateOpenChangeset(ctx, tx, project.ID)
		if err != nil {
			return err
		}
		if open.EnvironmentID != nil && *open.EnvironmentID != env.ID {
			pinned, _ := s.store.GetEnvironmentByID(ctx, *open.EnvironmentID)
			pinName := "unknown"
			if pinned != nil {
				pinName = pinned.Name
			}
			return fmt.Errorf("%w: changeset is pinned to environment %q; current context is %q",
				launchpad.ErrConflict, pinName, env.Name)
		}
		if open.EnvironmentID == nil {
			if err := s.store.SetChangesetEnvironment(ctx, tx, open.ID, env.ID); err != nil {
				return err
			}
		}

		changes := make([]domain.ChangesetChange, 0, len(input.Changes))
		for _, c := range input.Changes {
			change, err := toChangesetChange(c)
			if err != nil {
				return err
			}
			change.ServiceID = &svc.ID
			change.ServiceName = svc.Name
			changes = append(changes, change)
		}
		return s.store.AddChangesetChanges(ctx, tx, open.ID, changes)
	})
	if err != nil {
		return nil, err
	}
	cs, err := s.store.GetOpenChangeset(ctx, project.ID)
	if err != nil {
		return nil, err
	}
	return s.viewFor(ctx, cs)
}

func (s *ChangesetService) DiscardChangeset(ctx context.Context, projectName string) error {
	project, err := s.projectService.GetProject(ctx, projectName)
	if err != nil {
		return err
	}
	return s.store.DiscardOpenChangeset(ctx, project.ID)
}

// UnstageLastResult is the deleted change plus remaining open-changeset size.
type UnstageLastResult struct {
	Change          domain.ChangesetChange `json:"change"`
	RemainingCount  int                    `json:"remaining_count"`
	EnvironmentName string                 `json:"environment,omitempty"`
}

// UnstageLastChange removes the most recently staged change from the open changeset.
func (s *ChangesetService) UnstageLastChange(ctx context.Context, projectName string) (*UnstageLastResult, error) {
	project, err := s.projectService.GetProject(ctx, projectName)
	if err != nil {
		return nil, err
	}
	open, err := s.store.GetOpenChangeset(ctx, project.ID)
	if err != nil {
		return nil, fmt.Errorf("%w: no open changeset", launchpad.ErrNotFound)
	}
	if len(open.Changes) == 0 {
		return nil, fmt.Errorf("%w: no staged changes to unstage", launchpad.ErrNotFound)
	}
	var deleted *domain.ChangesetChange
	err = s.store.Transact(ctx, func(tx *sql.Tx) error {
		var err error
		deleted, err = s.store.DeleteLastChangesetChange(ctx, tx, open.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	result := &UnstageLastResult{
		Change:         *deleted,
		RemainingCount: len(open.Changes) - 1,
	}
	if open.EnvironmentID != nil {
		if env, err := s.store.GetEnvironmentByID(ctx, *open.EnvironmentID); err == nil && env != nil {
			result.EnvironmentName = env.Name
		}
	}
	return result, nil
}

func (s *ChangesetService) PushChangeset(ctx context.Context, projectName, envName string, input PushChangesetInput) (*CreateReleaseResult, error) {
	project, svc, reqEnv, err := s.projectService.resolvePrimaryService(ctx, projectName, envName)
	if err != nil {
		return nil, err
	}
	cs, err := s.store.GetOpenChangeset(ctx, project.ID)
	if err != nil {
		return nil, fmt.Errorf("%w: no open changeset to push", launchpad.ErrNotFound)
	}
	if len(cs.Changes) == 0 {
		return nil, fmt.Errorf("%w: changeset is empty", launchpad.ErrBadRequest)
	}
	if cs.EnvironmentID == nil {
		return nil, fmt.Errorf("%w: changeset has no environment pin", launchpad.ErrBadRequest)
	}
	if *cs.EnvironmentID != reqEnv.ID {
		pinned, _ := s.store.GetEnvironmentByID(ctx, *cs.EnvironmentID)
		pinName := "unknown"
		if pinned != nil {
			pinName = pinned.Name
		}
		return nil, fmt.Errorf("%w: changeset is pinned to environment %q; current context is %q",
			launchpad.ErrConflict, pinName, reqEnv.Name)
	}
	env := reqEnv

	sharedWrites, configWrites, scales, artifactRef, err := materializeChanges(cs.Changes)
	if err != nil {
		return nil, err
	}

	desc := input.Description
	if desc == "" {
		desc = fmt.Sprintf("Push changeset (%d changes)", len(cs.Changes))
	}

	var result CreateReleaseResult
	err = s.store.Transact(ctx, func(tx *sql.Tx) error {
		if len(sharedWrites) > 0 {
			if err := s.store.MergeConfigWritesTx(ctx, tx, "shared", uuid.Nil, env.ID, project.ID, sharedWrites); err != nil {
				return err
			}
		}
		if len(configWrites) > 0 {
			if err := s.store.MergeConfigWritesTx(ctx, tx, "service", svc.ID, env.ID, uuid.Nil, configWrites); err != nil {
				return err
			}
		}
		for process, qty := range scales {
			if err := s.store.UpdateProcessQuantity(ctx, tx, svc.ID, process, qty); err != nil {
				return err
			}
		}

		resolvedArtifact := artifactRef
		if resolvedArtifact == "" {
			latest, err := s.store.GetLatestSucceededRelease(ctx, svc.ID)
			if err != nil {
				return fmt.Errorf("%w: no image staged and no previous release to redeploy", launchpad.ErrBadRequest)
			}
			resolvedArtifact = latest.ArtifactRef
		}

		config, configSens, err := s.store.ResolveConfigWithSensitivityTx(ctx, tx, project.ID, svc.ID, env.ID)
		if err != nil {
			return err
		}
		processSnapshot, err := s.releaseService.buildProcessSnapshotTx(ctx, tx, svc.ID)
		if err != nil {
			return err
		}

		result, err = s.releaseService.enqueueReleaseTx(ctx, tx, project, svc, env, releasePlan{
			ArtifactRef:       resolvedArtifact,
			Config:            config,
			ConfigSensitivity: configSens,
			ProcessSnapshot:   processSnapshot,
			Description:       desc,
			AuditAction:       domain.AuditActionChangesetPush,
		})
		if err != nil {
			return err
		}
		return s.store.CommitChangeset(ctx, tx, cs.ID)
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *ChangesetService) viewFor(ctx context.Context, cs *domain.Changeset) (*ChangesetView, error) {
	view := &ChangesetView{Changeset: cs}
	if cs.EnvironmentID != nil {
		env, err := s.store.GetEnvironmentByID(ctx, *cs.EnvironmentID)
		if err == nil {
			view.EnvironmentName = env.Name
		}
	}
	return view, nil
}

func toChangesetChange(input StageChangeInput) (domain.ChangesetChange, error) {
	changeType := domain.ChangeType(input.Type)
	if changeType == domain.ChangeTypeConfig && input.Layer == "shared" {
		changeType = domain.ChangeTypeSharedConfig
	}
	switch changeType {
	case domain.ChangeTypeConfig, domain.ChangeTypeSharedConfig:
		if input.Key == "" {
			return domain.ChangesetChange{}, fmt.Errorf("%w: config change requires key", launchpad.ErrBadRequest)
		}
		if input.Sensitivity != nil {
			if domain.NormalizeSensitivity(*input.Sensitivity) == "" {
				return domain.ChangesetChange{}, fmt.Errorf("%w: sensitivity must be plain or secret", launchpad.ErrBadRequest)
			}
		}
		payload, err := json.Marshal(domain.ConfigChangePayload{
			Key:         input.Key,
			Value:       input.Value,
			Sensitivity: input.Sensitivity,
		})
		if err != nil {
			return domain.ChangesetChange{}, err
		}
		return domain.ChangesetChange{Type: changeType, Payload: payload}, nil
	case domain.ChangeTypeScale:
		if input.Process == "" || input.Quantity == nil {
			return domain.ChangesetChange{}, fmt.Errorf("%w: scale change requires process and quantity", launchpad.ErrBadRequest)
		}
		payload, err := json.Marshal(domain.ScaleChangePayload{Process: input.Process, Quantity: *input.Quantity})
		if err != nil {
			return domain.ChangesetChange{}, err
		}
		return domain.ChangesetChange{Type: domain.ChangeTypeScale, Payload: payload}, nil
	case domain.ChangeTypeImage:
		if input.Image == "" {
			return domain.ChangesetChange{}, fmt.Errorf("%w: image change requires image", launchpad.ErrBadRequest)
		}
		payload, err := json.Marshal(domain.ImageChangePayload{ArtifactRef: input.Image})
		if err != nil {
			return domain.ChangesetChange{}, err
		}
		return domain.ChangesetChange{Type: domain.ChangeTypeImage, Payload: payload}, nil
	default:
		return domain.ChangesetChange{}, fmt.Errorf("%w: unknown change type %q", launchpad.ErrBadRequest, input.Type)
	}
}

func materializeChanges(changes []domain.ChangesetChange) (shared, config map[string]store.ConfigWrite, scales map[string]int, artifactRef string, err error) {
	shared = make(map[string]store.ConfigWrite)
	config = make(map[string]store.ConfigWrite)
	scales = make(map[string]int)

	for _, c := range changes {
		switch c.Type {
		case domain.ChangeTypeConfig:
			var p domain.ConfigChangePayload
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				return nil, nil, nil, "", err
			}
			config[p.Key] = store.ConfigWrite{Value: p.Value, Sensitivity: p.Sensitivity}
		case domain.ChangeTypeSharedConfig:
			var p domain.ConfigChangePayload
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				return nil, nil, nil, "", err
			}
			shared[p.Key] = store.ConfigWrite{Value: p.Value, Sensitivity: p.Sensitivity}
		case domain.ChangeTypeScale:
			var p domain.ScaleChangePayload
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				return nil, nil, nil, "", err
			}
			scales[p.Process] = p.Quantity
		case domain.ChangeTypeImage:
			var p domain.ImageChangePayload
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				return nil, nil, nil, "", err
			}
			artifactRef = p.ArtifactRef
		default:
			return nil, nil, nil, "", fmt.Errorf("%w: unknown change type %q", launchpad.ErrBadRequest, c.Type)
		}
	}
	return shared, config, scales, artifactRef, nil
}
