package api

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/service"
)

type projectDTO struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	PrimaryService string    `json:"primary_service"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

type processDTO struct {
	ID        string    `json:"id"`
	ServiceID string    `json:"service_id"`
	Name      string    `json:"name"`
	Command   string    `json:"command"`
	Quantity  int       `json:"quantity"`
	Expose    string    `json:"expose"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type releaseDeploymentDTO struct {
	Environment string `json:"environment"`
	Status      string `json:"status"`
	ID          string `json:"id"`
}

type releaseDTO struct {
	ID              string                            `json:"id"`
	ServiceID       string                            `json:"service_id"`
	Version         int                               `json:"version"`
	ArtifactRef     string                            `json:"artifact_ref"`
	ConfigResolved  map[string]string                 `json:"config_resolved"`
	ProcessSnapshot map[string]domain.ProcessSnapshot `json:"process_snapshot"`
	Status          string                            `json:"status"`
	Description     string                            `json:"description"`
	CreatedAt       time.Time                         `json:"created_at"`
	Deployments     []releaseDeploymentDTO            `json:"deployments,omitempty"`
}

type environmentDTO struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	TargetType   string          `json:"target_type"`
	TargetConfig json.RawMessage `json:"target_config"`
	Ephemeral    bool            `json:"ephemeral"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type jobDTO struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	Status       string    `json:"status"`
	Attempt      int       `json:"attempt"`
	MaxAttempts  int       `json:"max_attempts"`
	LastError    string    `json:"last_error,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type changesetChangeDTO struct {
	ID          string          `json:"id"`
	ServiceID   *string         `json:"service_id,omitempty"`
	ServiceName string          `json:"service_name"`
	Type        string          `json:"type"`
	Payload     json.RawMessage `json:"payload"`
	CreatedAt   time.Time       `json:"created_at"`
}

type changesetDTO struct {
	ID          string               `json:"id"`
	ProjectID   string               `json:"project_id"`
	Environment string               `json:"environment,omitempty"`
	Status      string               `json:"status"`
	Description string               `json:"description"`
	Changes     []changesetChangeDTO `json:"changes"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
}

type releaseJobDTO struct {
	Deployment struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Release struct {
			Version int `json:"version"`
		} `json:"release"`
	} `json:"deployment"`
	Job struct {
		ID     string `json:"id"`
		Type   string `json:"type"`
		Status string `json:"status"`
	} `json:"job"`
}

func projectResponse(project *domain.Project) projectDTO {
	return projectDTO{
		ID:             project.ID.String(),
		Name:           project.Name,
		PrimaryService: project.PrimaryService,
		Status:         string(project.Status),
		CreatedAt:      project.CreatedAt,
	}
}

func processResponse(p domain.Process) processDTO {
	return processDTO{
		ID:        p.ID.String(),
		ServiceID: p.ServiceID.String(),
		Name:      p.Name,
		Command:   p.Command,
		Quantity:  p.Quantity,
		Expose:    p.Expose,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}

func releaseResponse(r domain.Release) releaseDTO {
	cfg := r.ConfigResolved
	if cfg == nil {
		cfg = map[string]string{}
	}
	snap := r.ProcessSnapshot
	if snap == nil {
		snap = map[string]domain.ProcessSnapshot{}
	}
	return releaseDTO{
		ID:              r.ID.String(),
		ServiceID:       r.ServiceID.String(),
		Version:         r.Version,
		ArtifactRef:     r.ArtifactRef,
		ConfigResolved:  cfg,
		ProcessSnapshot: snap,
		Status:          string(r.Status),
		Description:     r.Description,
		CreatedAt:       r.CreatedAt,
	}
}

func releaseWithDeploymentsResponse(item service.ReleaseWithDeployments) releaseDTO {
	dto := releaseResponse(item.Release)
	if len(item.Deployments) == 0 {
		return dto
	}
	dto.Deployments = make([]releaseDeploymentDTO, 0, len(item.Deployments))
	for _, d := range item.Deployments {
		dto.Deployments = append(dto.Deployments, releaseDeploymentDTO{
			Environment: d.Environment,
			Status:      d.Status,
			ID:          d.ID.String(),
		})
	}
	return dto
}

func environmentResponse(env *domain.Environment) environmentDTO {
	cfg := env.TargetConfig
	if len(cfg) == 0 {
		cfg = json.RawMessage(`{}`)
	}
	return environmentDTO{
		ID:           env.ID.String(),
		Name:         env.Name,
		TargetType:   env.TargetType,
		TargetConfig: cfg,
		Ephemeral:    env.Ephemeral,
		CreatedAt:    env.CreatedAt,
		UpdatedAt:    env.UpdatedAt,
	}
}

func jobResponse(j *domain.Job) jobDTO {
	return jobDTO{
		ID:           j.ID.String(),
		Type:         string(j.Type),
		ResourceType: j.ResourceType,
		ResourceID:   j.ResourceID.String(),
		Status:       string(j.Status),
		Attempt:      j.Attempt,
		MaxAttempts:  j.MaxAttempts,
		LastError:    j.LastError,
		CreatedAt:    j.CreatedAt,
		UpdatedAt:    j.UpdatedAt,
	}
}

func changesetResponse(cs *domain.Changeset) changesetDTO {
	if cs == nil {
		return changesetDTO{Changes: []changesetChangeDTO{}}
	}
	id := ""
	if cs.ID != uuid.Nil {
		id = cs.ID.String()
	}
	changes := make([]changesetChangeDTO, 0, len(cs.Changes))
	for _, c := range cs.Changes {
		dto := changesetChangeDTO{
			ID:          c.ID.String(),
			ServiceName: c.ServiceName,
			Type:        string(c.Type),
			Payload:     c.Payload,
			CreatedAt:   c.CreatedAt,
		}
		if c.ServiceID != nil {
			s := c.ServiceID.String()
			dto.ServiceID = &s
		}
		changes = append(changes, dto)
	}
	return changesetDTO{
		ID:          id,
		ProjectID:   cs.ProjectID.String(),
		Status:      string(cs.Status),
		Description: cs.Description,
		Changes:     changes,
		CreatedAt:   cs.CreatedAt,
		UpdatedAt:   cs.UpdatedAt,
	}
}

func changesetViewResponse(view *service.ChangesetView) changesetDTO {
	if view == nil || view.Changeset == nil {
		return changesetDTO{Changes: []changesetChangeDTO{}}
	}
	dto := changesetResponse(view.Changeset)
	dto.Environment = view.EnvironmentName
	return dto
}

func releaseJobResponse(result *service.CreateReleaseResult) releaseJobDTO {
	var out releaseJobDTO
	out.Deployment.ID = result.Deployment.ID.String()
	out.Deployment.Status = string(result.Deployment.Status)
	out.Deployment.Release.Version = result.Release.Version
	out.Job.ID = result.Job.ID.String()
	out.Job.Type = string(result.Job.Type)
	out.Job.Status = string(result.Job.Status)
	return out
}
