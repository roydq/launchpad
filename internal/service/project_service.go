package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/auth"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/store"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

const DefaultEnvironment = "dev"

var projectNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,62}$`)

// envNamePattern matches project name rules (DNS-label safe).
var envNamePattern = projectNamePattern

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

type CreateEnvironmentInput struct {
	Name      string      `json:"name"`
	Target    TargetInput `json:"target"`
	Ephemeral bool        `json:"ephemeral"`
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
		Name:         DefaultEnvironment,
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

func (s *ProjectService) ListEnvironments(ctx context.Context, projectName string) ([]domain.Environment, error) {
	project, err := s.GetProject(ctx, projectName)
	if err != nil {
		return nil, err
	}
	return s.store.ListEnvironments(ctx, project.ID)
}

func (s *ProjectService) GetEnvironment(ctx context.Context, projectName, envName string) (*domain.Environment, error) {
	project, err := s.GetProject(ctx, projectName)
	if err != nil {
		return nil, err
	}
	envName = normalizeEnvName(envName)
	return s.store.GetEnvironmentByProjectAndName(ctx, project.ID, envName)
}

func (s *ProjectService) CreateEnvironment(ctx context.Context, projectName string, input CreateEnvironmentInput) (*domain.Environment, error) {
	project, err := s.GetProject(ctx, projectName)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(input.Name)
	if !envNamePattern.MatchString(name) {
		return nil, fmt.Errorf("%w: invalid environment name", launchpad.ErrBadRequest)
	}
	targetConfig, _ := json.Marshal(map[string]string{
		"namespace": input.Target.Namespace,
		"cluster":   input.Target.Cluster,
	})
	env := &domain.Environment{
		ProjectID:    project.ID,
		Name:         name,
		TargetType:   defaultString(input.Target.Type, "stub"),
		TargetConfig: targetConfig,
		Ephemeral:    input.Ephemeral,
	}
	if err := s.store.CreateEnvironment(ctx, env); err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("%w: environment %q already exists", launchpad.ErrConflict, name)
		}
		return nil, err
	}
	return env, nil
}

// CloneEnvironmentResult reports what was copied vs secret keys that need values.
type CloneEnvironmentResult struct {
	Environment *domain.Environment `json:"environment"`
	From        string              `json:"from"`
	ClonedPlain []string            `json:"cloned_plain"`
	NeedsValue  []string            `json:"needs_value"`
	SharedKeys  int                 `json:"shared_keys"`
	ServiceKeys int                 `json:"service_keys"`
}

// CloneEnvironmentInput creates a new environment by cloning config layers from a source env.
// Secret values are never copied; their keys appear in NeedsValue.
type CloneEnvironmentInput struct {
	Name      string      `json:"name"`
	Target    TargetInput `json:"target"`
	Ephemeral bool        `json:"ephemeral"`
}

// CloneEnvironment creates env B from A per secrets clone policy.
func (s *ProjectService) CloneEnvironment(ctx context.Context, projectName, fromName string, input CloneEnvironmentInput) (*CloneEnvironmentResult, error) {
	project, err := s.GetProject(ctx, projectName)
	if err != nil {
		return nil, err
	}
	fromName = normalizeEnvName(fromName)
	toName := strings.TrimSpace(input.Name)
	if !envNamePattern.MatchString(toName) {
		return nil, fmt.Errorf("%w: invalid environment name", launchpad.ErrBadRequest)
	}
	if toName == fromName {
		return nil, fmt.Errorf("%w: clone source and destination must differ", launchpad.ErrBadRequest)
	}
	from, err := s.store.GetEnvironmentByProjectAndName(ctx, project.ID, fromName)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.GetEnvironmentByProjectAndName(ctx, project.ID, toName); err == nil {
		return nil, fmt.Errorf("%w: environment %q already exists", launchpad.ErrConflict, toName)
	} else if err != nil && !errors.Is(err, launchpad.ErrNotFound) {
		return nil, err
	}

	svc, err := s.store.GetServiceByProjectAndName(ctx, project.ID, project.PrimaryService)
	if err != nil {
		return nil, err
	}

	targetType, targetConfig := cloneTarget(from, input.Target)

	sharedVals, sharedSens, err := s.store.ListSharedConfigVarsWithSensitivityTx(ctx, nil, project.ID, from.ID)
	if err != nil {
		return nil, err
	}
	svcVals, svcSens, err := s.store.ListConfigVarsWithSensitivityTx(ctx, nil, svc.ID, from.ID)
	if err != nil {
		return nil, err
	}

	plainSet := map[string]struct{}{}
	needsSet := map[string]struct{}{}
	sharedWrites := map[string]store.ConfigWrite{}
	svcWrites := map[string]store.ConfigWrite{}
	sensPlain := domain.SensitivityPlain

	sensSecret := domain.SensitivitySecret
	empty := ""
	// Placeholders for secrets: empty value + sensitivity secret when encryption is available.
	// Without a secrets box, only report needs_value (cannot seal secret rows).
	canPlaceholder := s.store.Secrets() != nil

	for k, v := range sharedVals {
		if domain.IsSecret(sharedSens[k]) {
			needsSet[k] = struct{}{}
			if canPlaceholder {
				sharedWrites[k] = store.ConfigWrite{Value: &empty, Sensitivity: &sensSecret}
			}
			continue
		}
		val := v
		sharedWrites[k] = store.ConfigWrite{Value: &val, Sensitivity: &sensPlain}
		plainSet[k] = struct{}{}
	}
	for k, v := range svcVals {
		if domain.IsSecret(svcSens[k]) {
			needsSet[k] = struct{}{}
			if canPlaceholder {
				svcWrites[k] = store.ConfigWrite{Value: &empty, Sensitivity: &sensSecret}
			}
			continue
		}
		val := v
		svcWrites[k] = store.ConfigWrite{Value: &val, Sensitivity: &sensPlain}
		plainSet[k] = struct{}{}
	}

	to := &domain.Environment{
		ProjectID:    project.ID,
		Name:         toName,
		TargetType:   targetType,
		TargetConfig: targetConfig,
		Ephemeral:    input.Ephemeral,
	}

	err = s.store.Transact(ctx, func(tx *sql.Tx) error {
		if err := s.store.CreateEnvironmentTx(ctx, tx, to); err != nil {
			return err
		}
		if err := s.store.MergeConfigWritesTx(ctx, tx, "shared", uuid.Nil, to.ID, project.ID, sharedWrites); err != nil {
			return err
		}
		return s.store.MergeConfigWritesTx(ctx, tx, "service", svc.ID, to.ID, uuid.Nil, svcWrites)
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("%w: environment %q already exists", launchpad.ErrConflict, toName)
		}
		return nil, err
	}

	plain := setKeys(plainSet)
	needs := setKeys(needsSet)
	return &CloneEnvironmentResult{
		Environment: to,
		From:        fromName,
		ClonedPlain: plain,
		NeedsValue:  needs,
		SharedKeys:  len(sharedWrites),
		ServiceKeys: len(svcWrites),
	}, nil
}

func cloneTarget(from *domain.Environment, override TargetInput) (string, json.RawMessage) {
	if strings.TrimSpace(override.Type) == "" && strings.TrimSpace(override.Namespace) == "" && strings.TrimSpace(override.Cluster) == "" {
		return from.TargetType, from.TargetConfig
	}
	var fromTC map[string]string
	_ = json.Unmarshal(from.TargetConfig, &fromTC)
	if fromTC == nil {
		fromTC = map[string]string{}
	}
	ns := fromTC["namespace"]
	cluster := fromTC["cluster"]
	if override.Namespace != "" {
		ns = override.Namespace
	}
	if override.Cluster != "" {
		cluster = override.Cluster
	}
	tc, _ := json.Marshal(map[string]string{"namespace": ns, "cluster": cluster})
	return defaultString(override.Type, from.TargetType), tc
}

func setKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// resolvePrimaryService resolves project, primary service, and named environment.
// envName empty defaults to DefaultEnvironment ("dev").
func (s *ProjectService) resolvePrimaryService(ctx context.Context, projectName, envName string) (*domain.Project, *domain.Service, *domain.Environment, error) {
	project, err := s.GetProject(ctx, projectName)
	if err != nil {
		return nil, nil, nil, err
	}
	svc, err := s.store.GetServiceByProjectAndName(ctx, project.ID, project.PrimaryService)
	if err != nil {
		return nil, nil, nil, err
	}
	envName = normalizeEnvName(envName)
	env, err := s.store.GetEnvironmentByProjectAndName(ctx, project.ID, envName)
	if err != nil {
		return nil, nil, nil, err
	}
	return project, svc, env, nil
}

func normalizeEnvName(envName string) string {
	envName = strings.TrimSpace(envName)
	if envName == "" {
		return DefaultEnvironment
	}
	return envName
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate")
}

func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
