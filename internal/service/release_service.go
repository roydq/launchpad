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

type ReleaseService struct {
	store      *store.Store
	appService *AppService
}

func NewReleaseService(s *store.Store, appService *AppService) *ReleaseService {
	return &ReleaseService{store: s, appService: appService}
}

type CreateReleaseInput struct {
	Source      SourceInput `json:"source"`
	Description string      `json:"description"`
}

type SourceInput struct {
	Type  string `json:"type"`
	Image string `json:"image"`
}

type CreateReleaseResult struct {
	Release    domain.Release    `json:"release"`
	Deployment domain.Deployment `json:"deployment"`
	Job        domain.Job        `json:"job"`
}

func (s *ReleaseService) CreateRelease(ctx context.Context, appName string, input CreateReleaseInput) (*CreateReleaseResult, error) {
	app, err := s.appService.GetApp(ctx, appName)
	if err != nil {
		return nil, err
	}
	if app.ActiveDeploymentID != nil {
		return nil, fmt.Errorf("%w: deployment already in progress", launchpad.ErrConflict)
	}
	if input.Source.Type != "image" {
		return nil, fmt.Errorf("%w: only image source supported in v1", launchpad.ErrNotImplemented)
	}
	if input.Source.Image == "" {
		return nil, fmt.Errorf("%w: image is required", launchpad.ErrBadRequest)
	}

	config, err := s.store.ListConfigVars(ctx, app.ID)
	if err != nil {
		return nil, err
	}

	var result CreateReleaseResult
	err = s.store.Transact(ctx, func(tx *sql.Tx) error {
		version, err := s.store.NextReleaseVersion(ctx, tx, app.ID)
		if err != nil {
			return err
		}

		release := &domain.Release{
			AppID:          app.ID,
			Version:        version,
			ConfigSnapshot: config,
			ImageRef:       input.Source.Image,
			Status:         domain.ReleaseStatusPending,
			Description:    input.Description,
		}
		if err := s.store.CreateRelease(ctx, tx, release); err != nil {
			return err
		}

		deployment := &domain.Deployment{
			AppID:     app.ID,
			ReleaseID: release.ID,
			Status:    domain.DeploymentPending,
		}
		if err := s.store.CreateDeployment(ctx, tx, deployment); err != nil {
			return err
		}
		if err := s.store.SetActiveDeployment(ctx, tx, app.ID, deployment.ID); err != nil {
			return err
		}

		payload, _ := json.Marshal(domain.DeployPayload{
			DeploymentID: deployment.ID,
			AppID:        app.ID,
			ReleaseID:    release.ID,
		})
		job := &domain.Job{
			Type:         domain.JobTypeDeploy,
			ResourceType: "deployment",
			ResourceID:   deployment.ID,
			Payload:      payload,
		}
		if err := s.store.EnqueueJob(ctx, tx, job); err != nil {
			return err
		}

		result = CreateReleaseResult{Release: *release, Deployment: *deployment, Job: *job}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *ReleaseService) ListReleases(ctx context.Context, appName string) ([]domain.Release, error) {
	app, err := s.appService.GetApp(ctx, appName)
	if err != nil {
		return nil, err
	}
	return s.store.ListReleases(ctx, app.ID)
}