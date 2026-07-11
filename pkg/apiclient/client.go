package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	BaseURL     string
	Token       string
	Environment string // sent as X-Launchpad-Environment; empty means API defaults to dev
	HTTP        *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if c.Environment != "" {
		req.Header.Set("X-Launchpad-Environment", c.Environment)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, err
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return resp.StatusCode, err
		}
	}
	if resp.StatusCode >= 400 {
		return resp.StatusCode, fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, truncate(string(data), 200))
	}
	return resp.StatusCode, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

type Project struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	PrimaryService string `json:"primary_service"`
	Status         string `json:"status"`
}

type Process struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Command  string `json:"command"`
	Quantity int    `json:"quantity"`
	Expose   string `json:"expose"`
}

type ProcessSnapshot struct {
	Command  string `json:"command"`
	Quantity int    `json:"quantity"`
	Expose   string `json:"expose"`
}

type ReleaseDeployment struct {
	Environment string `json:"environment"`
	Status      string `json:"status"`
	ID          string `json:"id"`
}

type Release struct {
	ID              string                     `json:"id"`
	Version         int                        `json:"version"`
	ArtifactRef     string                     `json:"artifact_ref"`
	ConfigResolved  map[string]string          `json:"config_resolved"`
	ProcessSnapshot map[string]ProcessSnapshot `json:"process_snapshot"`
	Status          string                     `json:"status"`
	Description     string                     `json:"description"`
	Deployments     []ReleaseDeployment        `json:"deployments,omitempty"`
}

type Environment struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	TargetType   string          `json:"target_type"`
	TargetConfig json.RawMessage `json:"target_config"`
	Ephemeral    bool            `json:"ephemeral"`
}

type ChangesetChange struct {
	ID          string          `json:"id"`
	ServiceName string          `json:"service_name"`
	Type        string          `json:"type"`
	Payload     json.RawMessage `json:"payload"`
}

type Changeset struct {
	ID          string            `json:"id"`
	ProjectID   string            `json:"project_id"`
	Environment string            `json:"environment,omitempty"`
	Status      string            `json:"status"`
	Changes     []ChangesetChange `json:"changes"`
}

type DeployResult struct {
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

type Job struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	LastError string `json:"last_error,omitempty"`
}

func (c *Client) CreateProject(ctx context.Context, name, targetType, namespace string) (*Project, error) {
	var project Project
	_, err := c.do(ctx, http.MethodPost, "/v1/projects", map[string]any{
		"name": name,
		"target": map[string]any{
			"type":      targetType,
			"namespace": namespace,
		},
	}, &project)
	if err != nil {
		return nil, err
	}
	return &project, nil
}

func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	var projects []Project
	_, err := c.do(ctx, http.MethodGet, "/v1/projects", nil, &projects)
	return projects, err
}

func (c *Client) GetProject(ctx context.Context, name string) (*Project, error) {
	var project Project
	_, err := c.do(ctx, http.MethodGet, "/v1/projects/"+name, nil, &project)
	if err != nil {
		return nil, err
	}
	return &project, nil
}

func (c *Client) GetConfig(ctx context.Context, project string) (map[string]string, error) {
	var config map[string]string
	_, err := c.do(ctx, http.MethodGet, "/v1/projects/"+project+"/config", nil, &config)
	return config, err
}

func (c *Client) PatchConfig(ctx context.Context, project string, updates map[string]*string) (map[string]string, error) {
	var config map[string]string
	_, err := c.do(ctx, http.MethodPatch, "/v1/projects/"+project+"/config", updates, &config)
	return config, err
}

func (c *Client) ListProcesses(ctx context.Context, project string) ([]Process, error) {
	var processes []Process
	_, err := c.do(ctx, http.MethodGet, "/v1/projects/"+project+"/processes", nil, &processes)
	return processes, err
}

func (c *Client) ListReleases(ctx context.Context, project string) ([]Release, error) {
	var releases []Release
	_, err := c.do(ctx, http.MethodGet, "/v1/projects/"+project+"/releases", nil, &releases)
	return releases, err
}

func (c *Client) ListEnvironments(ctx context.Context, project string) ([]Environment, error) {
	var envs []Environment
	_, err := c.do(ctx, http.MethodGet, "/v1/projects/"+project+"/environments", nil, &envs)
	return envs, err
}

func (c *Client) GetEnvironment(ctx context.Context, project, name string) (*Environment, error) {
	var env Environment
	_, err := c.do(ctx, http.MethodGet, "/v1/projects/"+project+"/environments/"+name, nil, &env)
	if err != nil {
		return nil, err
	}
	return &env, nil
}

func (c *Client) CreateEnvironment(ctx context.Context, project, name, targetType, namespace string, ephemeral bool) (*Environment, error) {
	var env Environment
	_, err := c.do(ctx, http.MethodPost, "/v1/projects/"+project+"/environments", map[string]any{
		"name": name,
		"target": map[string]any{
			"type":      targetType,
			"namespace": namespace,
		},
		"ephemeral": ephemeral,
	}, &env)
	if err != nil {
		return nil, err
	}
	return &env, nil
}

func (c *Client) Deploy(ctx context.Context, project, image, description string) (*DeployResult, error) {
	var result DeployResult
	_, err := c.do(ctx, http.MethodPost, "/v1/projects/"+project+"/releases", map[string]any{
		"source": map[string]string{"type": "image", "image": image}, "description": description,
	}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) Rollback(ctx context.Context, project string, version int, description string) (*DeployResult, error) {
	var result DeployResult
	_, err := c.do(ctx, http.MethodPost, "/v1/projects/"+project+"/rollback", map[string]any{
		"version": version, "description": description,
	}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetChangeset(ctx context.Context, project string) (*Changeset, error) {
	var result Changeset
	_, err := c.do(ctx, http.MethodGet, "/v1/projects/"+project+"/changeset", nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) StageChanges(ctx context.Context, project string, changes []map[string]any) (*Changeset, error) {
	var result Changeset
	_, err := c.do(ctx, http.MethodPost, "/v1/projects/"+project+"/changeset/changes", map[string]any{"changes": changes}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) PushChangeset(ctx context.Context, project, description string) (*DeployResult, error) {
	var result DeployResult
	_, err := c.do(ctx, http.MethodPost, "/v1/projects/"+project+"/changeset/push", map[string]string{"description": description}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DiscardChangeset(ctx context.Context, project string) error {
	_, err := c.do(ctx, http.MethodDelete, "/v1/projects/"+project+"/changeset", nil, nil)
	return err
}

func (c *Client) GetJob(ctx context.Context, id string) (*Job, error) {
	var job Job
	_, err := c.do(ctx, http.MethodGet, "/v1/jobs/"+id, nil, &job)
	if err != nil {
		return nil, err
	}
	return &job, nil
}

type TokenCreateResult struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Workspace string   `json:"workspace"`
	Scopes    []string `json:"scopes"`
	Token     string   `json:"token"`
}

func (c *Client) Healthz(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz: status %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) CreateToken(ctx context.Context, name, workspace string, scopes []string) (*TokenCreateResult, error) {
	if workspace == "" {
		workspace = "default"
	}
	if len(scopes) == 0 {
		scopes = []string{"admin", "project:read", "project:write", "deploy"}
	}
	var out TokenCreateResult
	_, err := c.do(ctx, http.MethodPost, "/v1/tokens", map[string]any{
		"name": name, "workspace": workspace, "scopes": scopes,
	}, &out)
	if err != nil {
		return nil, err
	}
	if out.Token == "" {
		return nil, fmt.Errorf("create token: empty token in response")
	}
	return &out, nil
}
