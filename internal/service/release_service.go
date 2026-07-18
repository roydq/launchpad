package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/auth"
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
	ArtifactRef       string
	Config            map[string]string
	ConfigSensitivity map[string]string
	ProcessSnapshot   map[string]domain.ProcessSnapshot
	Description       string
	AuditAction       domain.AuditAction
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

	config, configSens, err := s.store.ResolveConfigWithSensitivity(ctx, project.ID, svc.ID, env.ID)
	if err != nil {
		return nil, err
	}

	desc := input.Description
	if desc == "" {
		desc = fmt.Sprintf("Deploy %s", input.Source.Image)
	}

	return s.enqueueRelease(ctx, project, svc, env, releasePlan{
		ArtifactRef:       input.Source.Image,
		Config:            config,
		ConfigSensitivity: configSens,
		Description:       desc,
		AuditAction:       domain.AuditActionReleaseCreate,
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
	config, configSens, err := s.store.ResolveConfigWithSensitivity(ctx, project.ID, svc.ID, env.ID)
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
		ArtifactRef:       prior.ArtifactRef,
		Config:            config,
		ConfigSensitivity: configSens,
		ProcessSnapshot:   snap,
		Description:       desc,
		AuditAction:       domain.AuditActionReleaseRollback,
	})
}

// Promote copies artifact + process_snapshot from a succeeded release applied in
// fromEnv, re-resolves config layers in toEnv, creates a new release, and deploys
// to toEnv. Source config_resolved is never copied.
//
// version 0 means use the release of the latest running deployment in fromEnv.
// toEnv empty defaults to DefaultEnvironment via resolvePrimaryService.
func (s *ReleaseService) Promote(ctx context.Context, projectName, fromEnv, toEnv string, version int, description string) (*CreateReleaseResult, error) {
	fromEnv = normalizeEnvName(fromEnv)
	toEnv = normalizeEnvName(toEnv)
	if fromEnv == toEnv {
		return nil, fmt.Errorf("%w: from and to environments must differ", launchpad.ErrBadRequest)
	}
	if version < 0 {
		return nil, fmt.Errorf("%w: version must be >= 0", launchpad.ErrBadRequest)
	}

	project, svc, targetEnv, err := s.projectService.resolvePrimaryService(ctx, projectName, toEnv)
	if err != nil {
		return nil, err
	}
	sourceEnv, err := s.store.GetEnvironmentByProjectAndName(ctx, project.ID, fromEnv)
	if err != nil {
		return nil, err
	}

	source, err := s.selectPromoteSource(ctx, svc.ID, sourceEnv.ID, fromEnv, version)
	if err != nil {
		return nil, err
	}

	// Target-env layers only — never source.ConfigResolved.
	config, configSens, err := s.store.ResolveConfigWithSensitivity(ctx, project.ID, svc.ID, targetEnv.ID)
	if err != nil {
		return nil, err
	}

	desc := description
	if desc == "" {
		desc = fmt.Sprintf("Promote v%d from %s to %s", source.Version, fromEnv, toEnv)
	}
	snap := source.ProcessSnapshot
	if snap == nil {
		snap = map[string]domain.ProcessSnapshot{}
	}

	return s.enqueueRelease(ctx, project, svc, targetEnv, releasePlan{
		ArtifactRef:       source.ArtifactRef,
		Config:            config,
		ConfigSensitivity: configSens,
		ProcessSnapshot:   snap,
		Description:       desc,
		AuditAction:       domain.AuditActionReleasePromote,
	})
}

func (s *ReleaseService) selectPromoteSource(ctx context.Context, serviceID, fromEnvID uuid.UUID, fromEnvName string, version int) (*domain.Release, error) {
	if version == 0 {
		dep, err := s.store.GetLatestDeploymentForServiceEnv(ctx, serviceID, fromEnvID)
		if err != nil {
			if errors.Is(err, launchpad.ErrNotFound) {
				return nil, fmt.Errorf("%w: no running release in %s; pass version explicitly", launchpad.ErrBadRequest, fromEnvName)
			}
			return nil, err
		}
		if dep.Status != domain.DeploymentRunning {
			// Prefer a running deployment; scan list if latest is not running.
			running, err := s.findRunningDeployment(ctx, serviceID, fromEnvID)
			if err != nil {
				return nil, err
			}
			if running == nil {
				return nil, fmt.Errorf("%w: no running release in %s; pass version explicitly", launchpad.ErrBadRequest, fromEnvName)
			}
			dep = running
		}
		rel, err := s.store.GetReleaseByID(ctx, dep.ReleaseID)
		if err != nil {
			return nil, err
		}
		if rel.Status != domain.ReleaseStatusSucceeded {
			return nil, fmt.Errorf("%w: release v%d is not succeeded", launchpad.ErrBadRequest, rel.Version)
		}
		return rel, nil
	}

	rel, err := s.store.GetReleaseByVersion(ctx, serviceID, version)
	if err != nil {
		return nil, err
	}
	if rel.Status != domain.ReleaseStatusSucceeded {
		return nil, fmt.Errorf("%w: release v%d is not succeeded", launchpad.ErrBadRequest, version)
	}
	ok, err := s.releaseSuccessfullyDeployedTo(ctx, serviceID, fromEnvID, rel.ID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: release v%d was never successfully deployed to %s", launchpad.ErrBadRequest, version, fromEnvName)
	}
	return rel, nil
}

func (s *ReleaseService) findRunningDeployment(ctx context.Context, serviceID, envID uuid.UUID) (*domain.Deployment, error) {
	deps, err := s.store.ListDeploymentsForService(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	for i := range deps {
		d := &deps[i]
		if d.EnvironmentID == envID && d.Status == domain.DeploymentRunning {
			return d, nil
		}
	}
	return nil, nil
}

func (s *ReleaseService) releaseSuccessfullyDeployedTo(ctx context.Context, serviceID, envID, releaseID uuid.UUID) (bool, error) {
	deps, err := s.store.ListDeploymentsForService(ctx, serviceID)
	if err != nil {
		return false, err
	}
	for _, d := range deps {
		if d.EnvironmentID != envID || d.ReleaseID != releaseID {
			continue
		}
		if d.Status == domain.DeploymentRunning || d.Status == domain.DeploymentSuperseded {
			return true, nil
		}
	}
	return false, nil
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
	configSens := plan.ConfigSensitivity
	if configSens == nil {
		configSens = map[string]string{}
	}

	version, err := s.store.NextReleaseVersion(ctx, tx, svc.ID)
	if err != nil {
		return zero, err
	}

	release := &domain.Release{
		ServiceID:            svc.ID,
		Version:              version,
		ArtifactRef:          plan.ArtifactRef,
		ConfigResolved:       config,
		ConfigSensitivity:    configSens,
		ProcessSnapshot:      processSnapshot,
		Status:               domain.ReleaseStatusPending,
		Description:          plan.Description,
		CreatedByPrincipalID: auth.PrincipalIDFromContext(ctx),
		CreatedByTokenID:     auth.TokenIDFromContext(ctx),
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

	action := plan.AuditAction
	if action == "" {
		action = domain.AuditActionReleaseCreate
	}
	auditDetail := map[string]string{
		"version":     strconv.Itoa(release.Version),
		"environment": env.Name,
		"artifact":    release.ArtifactRef,
	}
	if err := s.store.CreateAuditEvent(ctx, tx, &domain.AuditEvent{
		WorkspaceID:  project.WorkspaceID,
		PrincipalID:  release.CreatedByPrincipalID,
		TokenID:      release.CreatedByTokenID,
		Action:       action,
		ResourceType: "release",
		ResourceID:   release.ID,
		ProjectName:  project.Name,
		Detail:       auditDetail,
	}); err != nil {
		return zero, err
	}

	return CreateReleaseResult{Release: *release, Deployment: *deployment, Job: *job}, nil
}

// ListAuditEvents returns recent workspace audit rows for the caller's workspace.
func (s *ReleaseService) ListAuditEvents(ctx context.Context, limit int) ([]domain.AuditEvent, error) {
	workspaceID := auth.TeamIDFromContext(ctx)
	if workspaceID == uuid.Nil {
		return nil, fmt.Errorf("%w: workspace required", launchpad.ErrUnauthorized)
	}
	return s.store.ListAuditEvents(ctx, workspaceID, limit)
}

// ResolveCreatedBy loads principal display info for a release (best-effort).
func (s *ReleaseService) ResolveCreatedBy(ctx context.Context, rel domain.Release) *CreatedBy {
	if rel.CreatedByPrincipalID == nil {
		return nil
	}
	p, err := s.store.GetPrincipal(ctx, *rel.CreatedByPrincipalID)
	if err != nil {
		return &CreatedBy{
			PrincipalID: rel.CreatedByPrincipalID.String(),
			TokenID:     uuidPtrString(rel.CreatedByTokenID),
		}
	}
	return &CreatedBy{
		PrincipalID: p.ID.String(),
		Kind:        string(p.Kind),
		DisplayName: p.DisplayName,
		TokenID:     uuidPtrString(rel.CreatedByTokenID),
	}
}

// CreatedBy is API-facing actor attribution on a release.
type CreatedBy struct {
	PrincipalID string `json:"principal_id"`
	Kind        string `json:"kind,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	TokenID     string `json:"token_id,omitempty"`
}

func uuidPtrString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
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
