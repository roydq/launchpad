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
	// Process definition fields (process.set / process.unset / process.apply).
	Name     string               `json:"name,omitempty"`
	Command  *string              `json:"command,omitempty"`
	Expose   *string              `json:"expose,omitempty"`
	Procfile string               `json:"procfile,omitempty"`
	Health   *domain.ProcessHealth `json:"health,omitempty"`
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

	sharedWrites, configWrites, scales, artifactRef, processOps, err := materializeChanges(cs.Changes)
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
		if err := applyProcessOps(ctx, s.store, tx, svc.ID, processOps); err != nil {
			return err
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
	case domain.ChangeTypeProcessSet:
		name := input.Name
		if name == "" {
			name = input.Process
		}
		if name == "" {
			return domain.ChangesetChange{}, fmt.Errorf("%w: process.set requires name", launchpad.ErrBadRequest)
		}
		payload, err := json.Marshal(domain.ProcessSetPayload{
			Name: name, Command: input.Command, Quantity: input.Quantity, Expose: input.Expose, Health: input.Health,
		})
		if err != nil {
			return domain.ChangesetChange{}, err
		}
		return domain.ChangesetChange{Type: domain.ChangeTypeProcessSet, Payload: payload}, nil
	case domain.ChangeTypeProcessUnset:
		name := input.Name
		if name == "" {
			name = input.Process
		}
		if name == "" {
			return domain.ChangesetChange{}, fmt.Errorf("%w: process.unset requires name", launchpad.ErrBadRequest)
		}
		payload, err := json.Marshal(domain.ProcessUnsetPayload{Name: name})
		if err != nil {
			return domain.ChangesetChange{}, err
		}
		return domain.ChangesetChange{Type: domain.ChangeTypeProcessUnset, Payload: payload}, nil
	case domain.ChangeTypeProcessApply:
		if input.Procfile == "" {
			return domain.ChangesetChange{}, fmt.Errorf("%w: process.apply requires procfile", launchpad.ErrBadRequest)
		}
		if _, err := domain.ParseProcfile(input.Procfile); err != nil {
			return domain.ChangesetChange{}, fmt.Errorf("%w: %v", launchpad.ErrBadRequest, err)
		}
		payload, err := json.Marshal(domain.ProcessApplyPayload{Procfile: input.Procfile})
		if err != nil {
			return domain.ChangesetChange{}, err
		}
		return domain.ChangesetChange{Type: domain.ChangeTypeProcessApply, Payload: payload}, nil
	default:
		return domain.ChangesetChange{}, fmt.Errorf("%w: unknown change type %q", launchpad.ErrBadRequest, input.Type)
	}
}

// processOps are applied in order on push (before scale quantity overrides).
type processOps struct {
	sets   []domain.ProcessSetPayload
	unsets []string
	apply  *domain.ProcessApplyPayload
}

func materializeChanges(changes []domain.ChangesetChange) (shared, config map[string]store.ConfigWrite, scales map[string]int, artifactRef string, ops processOps, err error) {
	shared = make(map[string]store.ConfigWrite)
	config = make(map[string]store.ConfigWrite)
	scales = make(map[string]int)

	for _, c := range changes {
		switch c.Type {
		case domain.ChangeTypeConfig:
			var p domain.ConfigChangePayload
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				return nil, nil, nil, "", ops, err
			}
			config[p.Key] = store.ConfigWrite{Value: p.Value, Sensitivity: p.Sensitivity}
		case domain.ChangeTypeSharedConfig:
			var p domain.ConfigChangePayload
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				return nil, nil, nil, "", ops, err
			}
			shared[p.Key] = store.ConfigWrite{Value: p.Value, Sensitivity: p.Sensitivity}
		case domain.ChangeTypeScale:
			var p domain.ScaleChangePayload
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				return nil, nil, nil, "", ops, err
			}
			scales[p.Process] = p.Quantity
		case domain.ChangeTypeImage:
			var p domain.ImageChangePayload
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				return nil, nil, nil, "", ops, err
			}
			artifactRef = p.ArtifactRef
		case domain.ChangeTypeProcessSet:
			var p domain.ProcessSetPayload
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				return nil, nil, nil, "", ops, err
			}
			ops.sets = append(ops.sets, p)
		case domain.ChangeTypeProcessUnset:
			var p domain.ProcessUnsetPayload
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				return nil, nil, nil, "", ops, err
			}
			ops.unsets = append(ops.unsets, p.Name)
		case domain.ChangeTypeProcessApply:
			var p domain.ProcessApplyPayload
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				return nil, nil, nil, "", ops, err
			}
			ops.apply = &p
		default:
			return nil, nil, nil, "", ops, fmt.Errorf("%w: unknown change type %q", launchpad.ErrBadRequest, c.Type)
		}
	}
	return shared, config, scales, artifactRef, ops, nil
}

func applyProcessOps(ctx context.Context, st *store.Store, tx *sql.Tx, serviceID uuid.UUID, ops processOps) error {
	if ops.apply != nil {
		if ops.apply.Procfile != "" {
			entries, err := domain.ParseProcfile(ops.apply.Procfile)
			if err != nil {
				return fmt.Errorf("%w: %v", launchpad.ErrBadRequest, err)
			}
			// Replace: delete existing then upsert from procfile.
			existing, err := st.ListProcessesTx(ctx, tx, serviceID)
			if err != nil {
				return err
			}
			for _, p := range existing {
				if err := st.DeleteProcessTx(ctx, tx, serviceID, p.Name); err != nil {
					return err
				}
			}
			for _, e := range entries {
				cmd := e.Command
				if err := st.UpsertProcessTx(ctx, tx, &domain.Process{
					ServiceID: serviceID,
					Name:      e.Name,
					Command:   cmd,
					Quantity:  e.Quantity,
					Expose:    e.Expose,
				}); err != nil {
					return err
				}
			}
		}
	}
	for _, set := range ops.sets {
		if err := upsertProcessSet(ctx, st, tx, serviceID, set); err != nil {
			return err
		}
	}
	for _, name := range ops.unsets {
		existing, err := st.ListProcessesTx(ctx, tx, serviceID)
		if err != nil {
			return err
		}
		if len(existing) <= 1 {
			return fmt.Errorf("%w: cannot remove the last process", launchpad.ErrBadRequest)
		}
		if err := st.DeleteProcessTx(ctx, tx, serviceID, name); err != nil {
			return err
		}
	}
	return nil
}

func upsertProcessSet(ctx context.Context, st *store.Store, tx *sql.Tx, serviceID uuid.UUID, set domain.ProcessSetPayload) error {
	list, err := st.ListProcessesTx(ctx, tx, serviceID)
	if err != nil {
		return err
	}
	p := &domain.Process{ServiceID: serviceID, Name: set.Name}
	found := false
	for i := range list {
		if list[i].Name == set.Name {
			p.Command = list[i].Command
			p.Quantity = list[i].Quantity
			p.Expose = list[i].Expose
			p.Health = list[i].Health
			found = true
			break
		}
	}
	if !found {
		p.Quantity = 1
		if set.Name == "web" {
			p.Expose = "http"
		} else {
			p.Expose = "none"
		}
	}
	if set.Command != nil {
		p.Command = *set.Command
	}
	if set.Quantity != nil {
		p.Quantity = *set.Quantity
	}
	if set.Expose != nil {
		p.Expose = *set.Expose
	}
	if set.Health != nil {
		// Explicit type none clears probe.
		if set.Health.Type == "none" || set.Health.Type == "" {
			p.Health = nil
		} else {
			h := *set.Health
			if h.Type == "http" && h.Path == "" {
				h.Path = "/healthz"
			}
			p.Health = &h
		}
	}
	return st.UpsertProcessTx(ctx, tx, p)
}
