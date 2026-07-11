package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

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

	configUpdates, scales, artifactRef, err := materializeChanges(cs.Changes)
	if err != nil {
		return nil, err
	}

	desc := input.Description
	if desc == "" {
		desc = fmt.Sprintf("Push changeset (%d changes)", len(cs.Changes))
	}

	var result CreateReleaseResult
	err = s.store.Transact(ctx, func(tx *sql.Tx) error {
		if len(configUpdates) > 0 {
			if err := s.store.MergeConfigVarsTx(ctx, tx, svc.ID, env.ID, configUpdates); err != nil {
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

		config, err := s.store.ListConfigVarsTx(ctx, tx, svc.ID, env.ID)
		if err != nil {
			return err
		}
		processSnapshot, err := s.releaseService.buildProcessSnapshotTx(ctx, tx, svc.ID)
		if err != nil {
			return err
		}

		result, err = s.releaseService.enqueueReleaseTx(ctx, tx, project, svc, env, releasePlan{
			ArtifactRef:     resolvedArtifact,
			Config:          config,
			ProcessSnapshot: processSnapshot,
			Description:     desc,
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
	switch domain.ChangeType(input.Type) {
	case domain.ChangeTypeConfig:
		if input.Key == "" {
			return domain.ChangesetChange{}, fmt.Errorf("%w: config change requires key", launchpad.ErrBadRequest)
		}
		payload, err := json.Marshal(domain.ConfigChangePayload{Key: input.Key, Value: input.Value})
		if err != nil {
			return domain.ChangesetChange{}, err
		}
		return domain.ChangesetChange{Type: domain.ChangeTypeConfig, Payload: payload}, nil
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

func materializeChanges(changes []domain.ChangesetChange) (map[string]*string, map[string]int, string, error) {
	configUpdates := make(map[string]*string)
	scales := make(map[string]int)
	var artifactRef string

	for _, c := range changes {
		switch c.Type {
		case domain.ChangeTypeConfig:
			var p domain.ConfigChangePayload
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				return nil, nil, "", err
			}
			configUpdates[p.Key] = p.Value
		case domain.ChangeTypeScale:
			var p domain.ScaleChangePayload
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				return nil, nil, "", err
			}
			scales[p.Process] = p.Quantity
		case domain.ChangeTypeImage:
			var p domain.ImageChangePayload
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				return nil, nil, "", err
			}
			artifactRef = p.ArtifactRef
		default:
			return nil, nil, "", fmt.Errorf("%w: unknown change type %q", launchpad.ErrBadRequest, c.Type)
		}
	}
	return configUpdates, scales, artifactRef, nil
}
