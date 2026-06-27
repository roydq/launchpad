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

var appNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,62}$`)

type AppService struct {
	store *store.Store
}

func NewAppService(s *store.Store) *AppService {
	return &AppService{store: s}
}

type CreateAppInput struct {
	Name   string          `json:"name"`
	Team   string          `json:"team"`
	Stack  string          `json:"stack"`
	Target TargetInput     `json:"target"`
}

type TargetInput struct {
	Type      string          `json:"type"`
	Namespace string          `json:"namespace"`
	Cluster   string          `json:"cluster"`
	Extra     json.RawMessage `json:"-"`
}

func (s *AppService) CreateApp(ctx context.Context, input CreateAppInput) (*domain.App, error) {
	teamID := auth.TeamIDFromContext(ctx)
	if input.Team != "" {
		team, err := s.store.GetTeamByName(ctx, input.Team)
		if err != nil {
			return nil, err
		}
		teamID = team.ID
	}
	if !appNamePattern.MatchString(input.Name) {
		return nil, fmt.Errorf("%w: invalid app name", launchpad.ErrBadRequest)
	}
	targetConfig, _ := json.Marshal(map[string]string{
		"namespace": input.Target.Namespace,
		"cluster":   input.Target.Cluster,
	})
	app := &domain.App{
		TeamID:       teamID,
		Name:         input.Name,
		Stack:        input.Stack,
		TargetType:   defaultString(input.Target.Type, "kubernetes"),
		TargetConfig: targetConfig,
		Status:       domain.AppStatusCreated,
	}
	if err := s.store.CreateApp(ctx, app); err != nil {
		return nil, err
	}
	return app, nil
}

func (s *AppService) GetApp(ctx context.Context, name string) (*domain.App, error) {
	teamID := auth.TeamIDFromContext(ctx)
	return s.store.GetAppByTeamAndName(ctx, teamID, name)
}

func (s *AppService) ListApps(ctx context.Context) ([]domain.App, error) {
	teamID := auth.TeamIDFromContext(ctx)
	return s.store.ListAppsByTeam(ctx, teamID)
}

func (s *AppService) GetConfigVars(ctx context.Context, appName string) (map[string]string, error) {
	app, err := s.GetApp(ctx, appName)
	if err != nil {
		return nil, err
	}
	return s.store.ListConfigVars(ctx, app.ID)
}

func (s *AppService) PatchConfigVars(ctx context.Context, appName string, updates map[string]*string) (map[string]string, error) {
	app, err := s.GetApp(ctx, appName)
	if err != nil {
		return nil, err
	}
	if err := s.store.MergeConfigVars(ctx, app.ID, updates); err != nil {
		return nil, err
	}
	return s.store.ListConfigVars(ctx, app.ID)
}

func (s *AppService) ListProcesses(ctx context.Context, appName string) ([]domain.ProcessType, error) {
	app, err := s.GetApp(ctx, appName)
	if err != nil {
		return nil, err
	}
	return s.store.ListProcessTypes(ctx, app.ID)
}

func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}