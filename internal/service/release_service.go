package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/store"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

type ReleaseService struct {
	store          *store.Store
	projectService *ProjectService
}

func NewReleaseService(s *store.Store, projectService *ProjectService) *ReleaseService {
	return &ReleaseService{store: s, projectService: projectService}
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

type releasePlan struct {
	ArtifactRef string
	Config      map[string]string
	Description string
}

func (s *ReleaseService) CreateRelease(ctx context.Context, projectName string, input CreateReleaseInput) (*CreateReleaseResult, error) {
	project, svc, env, err := s.projectService.resolvePrimaryService(ctx, projectName)
	if err != nil {
		return nil, err
	}
	if input.Source.Type != "image" {
		return nil, fmt.Errorf("%w: only image source supported in v1", launchpad.ErrNotImplemented)
	}
	if input.Source.Image == "" {
		return nil, fmt.Errorf("%w: image is required", launchpad.ErrBadRequest)
	}

	config, err := s.store.ListConfigVars(ctx, svc.ID, env.ID)
	if err != nil {
		return nil, err
	}

	desc := input.Description
	if desc == "" {
		desc = fmt.Sprintf("Deploy %s", input.Source.Image)
	}

	return s.enqueueRelease(ctx, project, svc, env, releasePlan{
		ArtifactRef: input.Source.Image,
		Config:      config,
		Description: desc,
	})
}

func (s *ReleaseService) ListReleases(ctx context.Context, projectName string) ([]domain.Release, error) {
	_, svc, _, err := s.projectService.resolvePrimaryService(ctx, projectName)
	if err != nil {
		return nil, err
	}
	return s.store.ListReleases(ctx, svc.ID)
}

func (s *ReleaseService) enqueueRelease(ctx context.Context, project *domain.Project, svc *domain.Service, env *domain.Environment, plan releasePlan) (*CreateReleaseResult, error) {
	active, err := s.store.HasActiveDeployment(ctx, svc.ID, env.ID)
	if err != nil {
		return nil, err
	}
	if active {
		return nil, fmt.Errorf("%w: deployment already in progress", launchpad.ErrConflict)
	}
	if plan.ArtifactRef == "" {
		return nil, fmt.Errorf("%w: artifact is required", launchpad.ErrBadRequest)
	}

	processSnapshot, err := s.buildProcessSnapshot(ctx, svc.ID)
	if err != nil {
		return nil, err
	}

	var result CreateReleaseResult
	err = s.store.Transact(ctx, func(tx *sql.Tx) error {
		version, err := s.store.NextReleaseVersion(ctx, tx, svc.ID)
		if err != nil {
			return err
		}

		release := &domain.Release{
			ServiceID:       svc.ID,
			Version:         version,
			ArtifactRef:     plan.ArtifactRef,
			ConfigResolved:  plan.Config,
			ProcessSnapshot: processSnapshot,
			Status:          domain.ReleaseStatusPending,
			Description:     plan.Description,
		}
		if err := s.store.CreateRelease(ctx, tx, release); err != nil {
			return err
		}

		deployment := &domain.Deployment{
			ServiceID:     svc.ID,
			EnvironmentID: env.ID,
			ReleaseID:     release.ID,
			Status:        domain.DeploymentPending,
		}
		if err := s.store.CreateDeployment(ctx, tx, deployment); err != nil {
			return err
		}

		payload, _ := json.Marshal(domain.DeployPayload{
			DeploymentID:  deployment.ID,
			ServiceID:     svc.ID,
			EnvironmentID: env.ID,
			ReleaseID:     release.ID,
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

		if err := s.updateProjectStatusTx(ctx, tx, project.ID, domain.ProjectStatusDeploying); err != nil {
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

func (s *ReleaseService) buildProcessSnapshot(ctx context.Context, serviceID uuid.UUID) (map[string]domain.ProcessSnapshot, error) {
	processes, err := s.store.ListProcesses(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	snapshot := make(map[string]domain.ProcessSnapshot, len(processes))
	for _, p := range processes {
		snapshot[p.Name] = domain.ProcessSnapshot{Quantity: p.Quantity}
	}
	return snapshot, nil
}

func (s *ReleaseService) updateProjectStatusTx(ctx context.Context, tx *sql.Tx, projectID uuid.UUID, status domain.ProjectStatus) error {
	return s.store.UpdateProjectStatusTx(ctx, tx, projectID, status)
}