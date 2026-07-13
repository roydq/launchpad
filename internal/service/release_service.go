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
	ArtifactRef     string
	Config          map[string]string
	ProcessSnapshot map[string]domain.ProcessSnapshot
	Description     string
}

func (s *ReleaseService) CreateRelease(ctx context.Context, projectName, envName string, input CreateReleaseInput) (*CreateReleaseResult, error) {
	project, svc, env, err := s.projectService.resolvePrimaryService(ctx, projectName, envName)
	if err != nil {
		return nil, err
	}
	if input.Source.Type != "image" {
		return nil, fmt.Errorf("%w: only image source supported in v1", launchpad.ErrNotImplemented)
	}
	if input.Source.Image == "" {
		return nil, fmt.Errorf("%w: image is required", launchpad.ErrBadRequest)
	}

	config, err := s.store.ResolveConfig(ctx, project.ID, svc.ID, env.ID)
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

// Rollback creates a new release copying artifact + process_snapshot from version,
// re-resolves config for the target environment, and deploys.
func (s *ReleaseService) Rollback(ctx context.Context, projectName, envName string, version int, description string) (*CreateReleaseResult, error) {
	if version < 1 {
		return nil, fmt.Errorf("%w: version must be >= 1", launchpad.ErrBadRequest)
	}
	project, svc, env, err := s.projectService.resolvePrimaryService(ctx, projectName, envName)
	if err != nil {
		return nil, err
	}
	prior, err := s.store.GetReleaseByVersion(ctx, svc.ID, version)
	if err != nil {
		return nil, err
	}
	config, err := s.store.ResolveConfig(ctx, project.ID, svc.ID, env.ID)
	if err != nil {
		return nil, err
	}
	desc := description
	if desc == "" {
		desc = fmt.Sprintf("Rollback to v%d", version)
	}
	snap := prior.ProcessSnapshot
	if snap == nil {
		snap = map[string]domain.ProcessSnapshot{}
	}
	return s.enqueueRelease(ctx, project, svc, env, releasePlan{
		ArtifactRef:     prior.ArtifactRef,
		Config:          config,
		ProcessSnapshot: snap,
		Description:     desc,
	})
}

// ReleaseDeploymentRef is a deployment of a release into an environment.
type ReleaseDeploymentRef struct {
	Environment string
	Status      string
	ID          uuid.UUID
}

// ReleaseWithDeployments is a release plus where it has been deployed.
type ReleaseWithDeployments struct {
	Release     domain.Release
	Deployments []ReleaseDeploymentRef
}

func (s *ReleaseService) ListReleases(ctx context.Context, projectName, envName string) ([]ReleaseWithDeployments, error) {
	_, svc, _, err := s.projectService.resolvePrimaryService(ctx, projectName, envName)
	if err != nil {
		return nil, err
	}
	releases, err := s.store.ListReleases(ctx, svc.ID)
	if err != nil {
		return nil, err
	}
	deploys, err := s.store.ListDeploymentsForService(ctx, svc.ID)
	if err != nil {
		return nil, err
	}
	envNames := map[uuid.UUID]string{}
	for _, d := range deploys {
		if _, ok := envNames[d.EnvironmentID]; !ok {
			env, err := s.store.GetEnvironmentByID(ctx, d.EnvironmentID)
			if err == nil {
				envNames[d.EnvironmentID] = env.Name
			}
		}
	}
	byRelease := map[uuid.UUID][]ReleaseDeploymentRef{}
	for _, d := range deploys {
		name := envNames[d.EnvironmentID]
		if name == "" {
			name = d.EnvironmentID.String()
		}
		byRelease[d.ReleaseID] = append(byRelease[d.ReleaseID], ReleaseDeploymentRef{
			Environment: name,
			Status:      string(d.Status),
			ID:          d.ID,
		})
	}
	out := make([]ReleaseWithDeployments, 0, len(releases))
	for _, r := range releases {
		out = append(out, ReleaseWithDeployments{
			Release:     r,
			Deployments: byRelease[r.ID],
		})
	}
	return out, nil
}

// GetLatestReleaseForEnvironment returns the release for the latest deployment in env, or nil if none.
func (s *ReleaseService) GetLatestReleaseForEnvironment(ctx context.Context, projectName, envName string) (*domain.Release, error) {
	_, svc, env, err := s.projectService.resolvePrimaryService(ctx, projectName, envName)
	if err != nil {
		return nil, err
	}
	dep, err := s.store.GetLatestDeploymentForServiceEnv(ctx, svc.ID, env.ID)
	if err != nil {
		if errors.Is(err, launchpad.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return s.store.GetReleaseByID(ctx, dep.ReleaseID)
}

func (s *ReleaseService) enqueueRelease(ctx context.Context, project *domain.Project, svc *domain.Service, env *domain.Environment, plan releasePlan) (*CreateReleaseResult, error) {
	var result CreateReleaseResult
	err := s.store.Transact(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = s.enqueueReleaseTx(ctx, tx, project, svc, env, plan)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *ReleaseService) enqueueReleaseTx(ctx context.Context, tx *sql.Tx, project *domain.Project, svc *domain.Service, env *domain.Environment, plan releasePlan) (CreateReleaseResult, error) {
	var zero CreateReleaseResult

	active, err := s.store.HasActiveDeploymentTx(ctx, tx, svc.ID, env.ID)
	if err != nil {
		return zero, err
	}
	if active {
		return zero, fmt.Errorf("%w: deployment already in progress", launchpad.ErrConflict)
	}
	if plan.ArtifactRef == "" {
		return zero, fmt.Errorf("%w: artifact is required", launchpad.ErrBadRequest)
	}

	processSnapshot := plan.ProcessSnapshot
	if processSnapshot == nil {
		processSnapshot, err = s.buildProcessSnapshotTx(ctx, tx, svc.ID)
		if err != nil {
			return zero, err
		}
	}

	config := plan.Config
	if config == nil {
		config = map[string]string{}
	}

	version, err := s.store.NextReleaseVersion(ctx, tx, svc.ID)
	if err != nil {
		return zero, err
	}

	release := &domain.Release{
		ServiceID:       svc.ID,
		Version:         version,
		ArtifactRef:     plan.ArtifactRef,
		ConfigResolved:  config,
		ProcessSnapshot: processSnapshot,
		Status:          domain.ReleaseStatusPending,
		Description:     plan.Description,
	}
	if err := s.store.CreateRelease(ctx, tx, release); err != nil {
		return zero, err
	}

	deployment := &domain.Deployment{
		ServiceID:     svc.ID,
		EnvironmentID: env.ID,
		ReleaseID:     release.ID,
		Status:        domain.DeploymentPending,
	}
	if err := s.store.CreateDeployment(ctx, tx, deployment); err != nil {
		return zero, err
	}

	payload, err := json.Marshal(domain.DeployPayload{
		DeploymentID:  deployment.ID,
		ServiceID:     svc.ID,
		EnvironmentID: env.ID,
		ReleaseID:     release.ID,
	})
	if err != nil {
		return zero, err
	}
	job := &domain.Job{
		Type:         domain.JobTypeDeploy,
		ResourceType: "deployment",
		ResourceID:   deployment.ID,
		Payload:      payload,
	}
	if err := s.store.EnqueueJob(ctx, tx, job); err != nil {
		return zero, err
	}

	if err := s.store.UpdateProjectStatusTx(ctx, tx, project.ID, domain.ProjectStatusDeploying); err != nil {
		return zero, err
	}

	return CreateReleaseResult{Release: *release, Deployment: *deployment, Job: *job}, nil
}

func (s *ReleaseService) buildProcessSnapshot(ctx context.Context, serviceID uuid.UUID) (map[string]domain.ProcessSnapshot, error) {
	return s.buildProcessSnapshotTx(ctx, nil, serviceID)
}

func (s *ReleaseService) buildProcessSnapshotTx(ctx context.Context, tx *sql.Tx, serviceID uuid.UUID) (map[string]domain.ProcessSnapshot, error) {
	processes, err := s.store.ListProcessesTx(ctx, tx, serviceID)
	if err != nil {
		return nil, err
	}
	snapshot := make(map[string]domain.ProcessSnapshot, len(processes))
	for _, p := range processes {
		snapshot[p.Name] = domain.ProcessSnapshot{
			Command:  p.Command,
			Quantity: p.Quantity,
			Expose:   p.Expose,
		}
	}
	return snapshot, nil
}
