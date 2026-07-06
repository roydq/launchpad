package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/launchpad/launchpad/internal/auth"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/store"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

const devEnvironment = "dev"

var projectNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,62}$`)

type ProjectService struct {
	store *store.Store
}

func NewProjectService(s *store.Store) *ProjectService {
	return &ProjectService{store: s}
}

type CreateProjectInput struct {
	Name   string      `json:"name"`
	Target TargetInput `json:"target"`
}

type TargetInput struct {
	Type      string          `json:"type"`
	Namespace string          `json:"namespace"`
	Cluster   string          `json:"cluster"`
	Extra     json.RawMessage `json:"-"`
}

func (s *ProjectService) CreateProject(ctx context.Context, input CreateProjectInput) (*domain.Project, error) {
	workspaceID := auth.TeamIDFromContext(ctx)
	if !projectNamePattern.MatchString(input.Name) {
		return nil, fmt.Errorf("%w: invalid project name", launchpad.ErrBadRequest)
	}

	targetConfig, _ := json.Marshal(map[string]string{
		"namespace": input.Target.Namespace,
		"cluster":   input.Target.Cluster,
	})
	project := &domain.Project{
		WorkspaceID:    workspaceID,
		Name:           input.Name,
		PrimaryService: input.Name,
		Status:         domain.ProjectStatusCreated,
	}
	env := &domain.Environment{
		Name:         devEnvironment,
		TargetType:   defaultString(input.Target.Type, "kubernetes"),
		TargetConfig: targetConfig,
	}
	if err := s.store.CreateProject(ctx, project, env); err != nil {
		return nil, err
	}
	return project, nil
}

func (s *ProjectService) GetProject(ctx context.Context, name string) (*domain.Project, error) {
	workspaceID := auth.TeamIDFromContext(ctx)
	return s.store.GetProjectByWorkspaceAndName(ctx, workspaceID, name)
}

func (s *ProjectService) ListProjects(ctx context.Context) ([]domain.Project, error) {
	workspaceID := auth.TeamIDFromContext(ctx)
	return s.store.ListProjectsByWorkspace(ctx, workspaceID)
}

func (s *ProjectService) ListProcesses(ctx context.Context, projectName string) ([]domain.Process, error) {
	project, err := s.GetProject(ctx, projectName)
	if err != nil {
		return nil, err
	}
	svc, err := s.store.GetServiceByProjectAndName(ctx, project.ID, project.PrimaryService)
	if err != nil {
		return nil, err
	}
	return s.store.ListProcesses(ctx, svc.ID)
}

func (s *ProjectService) resolvePrimaryService(ctx context.Context, projectName string) (*domain.Project, *domain.Service, *domain.Environment, error) {
	project, err := s.GetProject(ctx, projectName)
	if err != nil {
		return nil, nil, nil, err
	}
	svc, err := s.store.GetServiceByProjectAndName(ctx, project.ID, project.PrimaryService)
	if err != nil {
		return nil, nil, nil, err
	}
	env, err := s.store.GetEnvironmentByProjectAndName(ctx, project.ID, devEnvironment)
	if err != nil {
		return nil, nil, nil, err
	}
	return project, svc, env, nil
}

func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}