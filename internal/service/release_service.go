package service

import (
	"context"
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

	desc := input.Description
	if desc == "" {
		desc = fmt.Sprintf("Deploy %s", input.Source.Image)
	}

	return s.enqueueRelease(ctx, app, releasePlan{
		ImageRef:    input.Source.Image,
		Config:      config,
		Description: desc,
		JobType:     domain.JobTypeDeploy,
	})
}

func (s *ReleaseService) RollbackRelease(ctx context.Context, appName string, version int) (*CreateReleaseResult, error) {
	app, err := s.appService.GetApp(ctx, appName)
	if err != nil {
		return nil, err
	}
	target, err := s.store.GetReleaseByVersion(ctx, app.ID, version)
	if err != nil {
		return nil, err
	}
	if target.Status != domain.ReleaseStatusSucceeded {
		return nil, fmt.Errorf("%w: release v%d did not succeed", launchpad.ErrBadRequest, version)
	}

	return s.enqueueRelease(ctx, app, releasePlan{
		ImageRef:     target.ImageRef,
		Config:       target.ConfigSnapshot,
		Description:  fmt.Sprintf("Rollback to v%d", version),
		JobType:      domain.JobTypeRollback,
		RollbackFrom: version,
	})
}

func (s *ReleaseService) ListReleases(ctx context.Context, appName string) ([]domain.Release, error) {
	app, err := s.appService.GetApp(ctx, appName)
	if err != nil {
		return nil, err
	}
	return s.store.ListReleases(ctx, app.ID)
}