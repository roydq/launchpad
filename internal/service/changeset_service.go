package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/store"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

type ChangesetService struct {
	store         *store.Store
	appService    *AppService
	releaseService *ReleaseService
}

func NewChangesetService(s *store.Store, appService *AppService, releaseService *ReleaseService) *ChangesetService {
	return &ChangesetService{store: s, appService: appService, releaseService: releaseService}
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
	Changes []StageChangeInput `json:"changes"`
}

type PushChangesetInput struct {
	Description string `json:"description"`
}

func (s *ChangesetService) GetChangeset(ctx context.Context, appName string) (*domain.Changeset, error) {
	app, err := s.appService.GetApp(ctx, appName)
	if err != nil {
		return nil, err
	}
	cs, err := s.store.GetOpenChangeset(ctx, app.ID)
	if err != nil {
		if err == launchpad.ErrNotFound {
			return &domain.Changeset{AppID: app.ID, Status: domain.ChangesetOpen, Changes: []domain.ChangesetChange{}}, nil
		}
		return nil, err
	}
	return cs, nil
}

func (s *ChangesetService) StageChanges(ctx context.Context, appName string, input StageChangesInput) (*domain.Changeset, error) {
	if len(input.Changes) == 0 {
		return nil, fmt.Errorf("%w: at least one change required", launchpad.ErrBadRequest)
	}
	app, err := s.appService.GetApp(ctx, appName)
	if err != nil {
		return nil, err
	}

	err = s.store.Transact(ctx, func(tx *sql.Tx) error {
		open, err := s.store.GetOrCreateOpenChangeset(ctx, tx, app.ID)
		if err != nil {
			return err
		}

		changes := make([]domain.ChangesetChange, 0, len(input.Changes))
		for _, c := range input.Changes {
			change, err := toChangesetChange(c)
			if err != nil {
				return err
			}
			changes = append(changes, change)
		}
		return s.store.AddChangesetChanges(ctx, tx, open.ID, changes)
	})
	if err != nil {
		return nil, err
	}
	return s.store.GetOpenChangeset(ctx, app.ID)
}

func (s *ChangesetService) DiscardChangeset(ctx context.Context, appName string) error {
	app, err := s.appService.GetApp(ctx, appName)
	if err != nil {
		return err
	}
	return s.store.DiscardOpenChangeset(ctx, app.ID)
}

func (s *ChangesetService) PushChangeset(ctx context.Context, appName string, input PushChangesetInput) (*CreateReleaseResult, error) {
	app, err := s.appService.GetApp(ctx, appName)
	if err != nil {
		return nil, err
	}
	cs, err := s.store.GetOpenChangeset(ctx, app.ID)
	if err != nil {
		return nil, fmt.Errorf("%w: no open changeset to push", launchpad.ErrNotFound)
	}
	if len(cs.Changes) == 0 {
		return nil, fmt.Errorf("%w: changeset is empty", launchpad.ErrBadRequest)
	}

	configUpdates, scales, image, err := materializeChanges(cs.Changes)
	if err != nil {
		return nil, err
	}

	liveConfig, err := s.store.ListConfigVars(ctx, app.ID)
	if err != nil {
		return nil, err
	}
	for k, v := range configUpdates {
		if v == nil {
			delete(liveConfig, k)
		} else {
			liveConfig[k] = *v
		}
	}

	err = s.store.Transact(ctx, func(tx *sql.Tx) error {
		if len(configUpdates) > 0 {
			if err := s.store.MergeConfigVarsTx(ctx, tx, app.ID, configUpdates); err != nil {
				return err
			}
		}
		for process, qty := range scales {
			if err := s.store.UpdateProcessQuantity(ctx, tx, app.ID, process, qty); err != nil {
				return err
			}
		}
		if err := s.store.CommitChangeset(ctx, tx, cs.ID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	imageRef := image
	if imageRef == "" {
		latest, err := s.store.GetLatestSucceededRelease(ctx, app.ID)
		if err != nil {
			return nil, fmt.Errorf("%w: no image staged and no previous release to redeploy", launchpad.ErrBadRequest)
		}
		imageRef = latest.ImageRef
	}

	desc := input.Description
	if desc == "" {
		desc = fmt.Sprintf("Push changeset (%d changes)", len(cs.Changes))
	}

	return s.releaseService.enqueueRelease(ctx, app, releasePlan{
		ImageRef:    imageRef,
		Config:      liveConfig,
		Description: desc,
		JobType:     domain.JobTypeDeploy,
	})
}

func toChangesetChange(input StageChangeInput) (domain.ChangesetChange, error) {
	switch domain.ChangeType(input.Type) {
	case domain.ChangeTypeConfig:
		if input.Key == "" {
			return domain.ChangesetChange{}, fmt.Errorf("%w: config change requires key", launchpad.ErrBadRequest)
		}
		payload, _ := json.Marshal(domain.ConfigChangePayload{Key: input.Key, Value: input.Value})
		return domain.ChangesetChange{Type: domain.ChangeTypeConfig, Payload: payload}, nil
	case domain.ChangeTypeScale:
		if input.Process == "" || input.Quantity == nil {
			return domain.ChangesetChange{}, fmt.Errorf("%w: scale change requires process and quantity", launchpad.ErrBadRequest)
		}
		payload, _ := json.Marshal(domain.ScaleChangePayload{Process: input.Process, Quantity: *input.Quantity})
		return domain.ChangesetChange{Type: domain.ChangeTypeScale, Payload: payload}, nil
	case domain.ChangeTypeImage:
		if input.Image == "" {
			return domain.ChangesetChange{}, fmt.Errorf("%w: image change requires image", launchpad.ErrBadRequest)
		}
		payload, _ := json.Marshal(domain.ImageChangePayload{Image: input.Image})
		return domain.ChangesetChange{Type: domain.ChangeTypeImage, Payload: payload}, nil
	default:
		return domain.ChangesetChange{}, fmt.Errorf("%w: unknown change type %q", launchpad.ErrBadRequest, input.Type)
	}
}

func materializeChanges(changes []domain.ChangesetChange) (map[string]*string, map[string]int, string, error) {
	configUpdates := make(map[string]*string)
	scales := make(map[string]int)
	var image string

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
			image = p.Image
		}
	}
	return configUpdates, scales, image, nil
}