package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

type releasePlan struct {
	ImageRef    string
	Config      map[string]string
	Description string
	JobType     domain.JobType
	RollbackFrom int
}

func (s *ReleaseService) enqueueRelease(ctx context.Context, app *domain.App, plan releasePlan) (*CreateReleaseResult, error) {
	if app.ActiveDeploymentID != nil {
		return nil, fmt.Errorf("%w: deployment already in progress", launchpad.ErrConflict)
	}
	if plan.ImageRef == "" {
		return nil, fmt.Errorf("%w: image is required", launchpad.ErrBadRequest)
	}

	var result CreateReleaseResult
	err := s.store.Transact(ctx, func(tx *sql.Tx) error {
		version, err := s.store.NextReleaseVersion(ctx, tx, app.ID)
		if err != nil {
			return err
		}

		release := &domain.Release{
			AppID:          app.ID,
			Version:        version,
			ConfigSnapshot: plan.Config,
			ImageRef:       plan.ImageRef,
			Status:         domain.ReleaseStatusPending,
			Description:    plan.Description,
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

		jobType := plan.JobType
		if jobType == "" {
			jobType = domain.JobTypeDeploy
		}

		var payload []byte
		switch jobType {
		case domain.JobTypeRollback:
			payload, _ = json.Marshal(domain.RollbackPayload{
				DeploymentID:         deployment.ID,
				AppID:                app.ID,
				ReleaseID:            release.ID,
				TargetReleaseVersion: plan.RollbackFrom,
			})
		default:
			payload, _ = json.Marshal(domain.DeployPayload{
				DeploymentID: deployment.ID,
				AppID:        app.ID,
				ReleaseID:    release.ID,
			})
		}

		job := &domain.Job{
			Type:         jobType,
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