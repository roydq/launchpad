package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	if out != nil && len(data) > 0 && resp.StatusCode < 400 {
		if err := json.Unmarshal(data, out); err != nil {
			return resp.StatusCode, err
		}
	}
	if resp.StatusCode >= 400 {
		return resp.StatusCode, parseAPIError(method, path, resp.StatusCode, data)
	}
	return resp.StatusCode, nil
}

// RecoveryHint is a server-provided recovery suggestion from problem+json.
type RecoveryHint struct {
	Action  string `json:"action"`
	Message string `json:"message"`
	Command string `json:"command,omitempty"`
}

// APIError is a structured problem+json failure from the control plane.
type APIError struct {
	Method  string
	Path    string
	Status  int
	Type    string          `json:"type"`
	Title   string          `json:"title"`
	Detail  string          `json:"detail"`
	Code    string          `json:"code"`
	Hints   []RecoveryHint  `json:"hints"`
	RawBody string
}

func (e *APIError) Error() string {
	if e == nil {
		return "api error"
	}
	msg := fmt.Sprintf("%s %s: status %d", e.Method, e.Path, e.Status)
	if e.Detail != "" {
		msg += ": " + e.Detail
	} else if e.RawBody != "" {
		msg += ": " + truncate(e.RawBody, 200)
	}
	if e.Code != "" {
		msg += " [" + e.Code + "]"
	}
	if len(e.Hints) > 0 && e.Hints[0].Command != "" {
		msg += " (try: " + e.Hints[0].Command + ")"
	}
	return msg
}

func parseAPIError(method, path string, status int, data []byte) error {
	ae := &APIError{Method: method, Path: path, Status: status, RawBody: string(data)}
	if len(data) > 0 {
		_ = json.Unmarshal(data, ae)
	}
	return ae
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

type CreatedBy struct {
	PrincipalID string `json:"principal_id"`
	Kind        string `json:"kind,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	TokenID     string `json:"token_id,omitempty"`
}

type Release struct {
	ID              string                     `json:"id"`
	Version         int                        `json:"version"`
	ArtifactRef     string                     `json:"artifact_ref"`
	ConfigResolved  map[string]string          `json:"config_resolved"`
	ProcessSnapshot map[string]ProcessSnapshot `json:"process_snapshot"`
	Status          string                     `json:"status"`
	Description     string                     `json:"description"`
	CreatedBy       *CreatedBy                 `json:"created_by,omitempty"`
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
	return c.GetConfigLayer(ctx, project, "")
}

func (c *Client) GetConfigLayer(ctx context.Context, project, layer string) (map[string]string, error) {
	var config map[string]string
	path := "/v1/projects/" + project + "/config"
	if layer != "" {
		path += "?layer=" + url.QueryEscape(layer)
	}
	_, err := c.do(ctx, http.MethodGet, path, nil, &config)
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

// GetLogs returns process logs as plain text (current client Environment header).
func (c *Client) GetLogs(ctx context.Context, project, process string) (string, error) {
	if process == "" {
		process = "web"
	}
	path := "/v1/projects/" + project + "/logs?process=" + url.QueryEscape(process)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return "", err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if c.Environment != "" {
		req.Header.Set("X-Launchpad-Environment", c.Environment)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("GET %s: status %d: %s", path, resp.StatusCode, truncate(string(data), 200))
	}
	return string(data), nil
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

// CloneEnvironmentResult is the report from cloning an environment.
type CloneEnvironmentResult struct {
	Environment Environment `json:"environment"`
	From        string      `json:"from"`
	ClonedPlain []string    `json:"cloned_plain"`
	NeedsValue  []string    `json:"needs_value"`
	SharedKeys  int         `json:"shared_keys"`
	ServiceKeys int         `json:"service_keys"`
}

func (c *Client) CloneEnvironment(ctx context.Context, project, from, name, targetType, namespace string, ephemeral bool) (*CloneEnvironmentResult, error) {
	body := map[string]any{
		"name":      name,
		"ephemeral": ephemeral,
	}
	if targetType != "" || namespace != "" {
		body["target"] = map[string]any{
			"type":      targetType,
			"namespace": namespace,
		}
	}
	var result CloneEnvironmentResult
	_, err := c.do(ctx, http.MethodPost, "/v1/projects/"+project+"/environments/"+from+"/clone", body, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
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

// Promote creates a new release in the target environment from a source release.
// to empty uses the client's Environment header (or API default) as target.
// version 0 means use the running release in from.
func (c *Client) Promote(ctx context.Context, project, from, to string, version int, description string) (*DeployResult, error) {
	body := map[string]any{
		"from":        from,
		"description": description,
	}
	if to != "" {
		body["to"] = to
	}
	if version > 0 {
		body["version"] = version
	}
	var result DeployResult
	_, err := c.do(ctx, http.MethodPost, "/v1/projects/"+project+"/promote", body, &result)
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

// Preview is the server-side pending, release-compare, or env↔env diff.
type Preview struct {
	Mode            string `json:"mode"`
	Environment     string `json:"environment,omitempty"`
	FromEnvironment string `json:"from_environment,omitempty"`
	ToEnvironment   string `json:"to_environment,omitempty"`
	BaselineVersion *int   `json:"baseline_version,omitempty"`
	FromVersion     *int   `json:"from_version,omitempty"`
	ToVersion       *int   `json:"to_version,omitempty"`
	HasPending      bool   `json:"has_pending"`
	MatchesBaseline bool   `json:"matches_baseline"`
	Pending *struct {
		Image  string             `json:"image,omitempty"`
		Config map[string]*string `json:"config,omitempty"`
		Scales map[string]int     `json:"scales,omitempty"`
	} `json:"pending,omitempty"`
	Diff struct {
		Image *struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"image,omitempty"`
		Config []struct {
			Op   string  `json:"op"`
			Key  string  `json:"key"`
			From *string `json:"from,omitempty"`
			To   *string `json:"to,omitempty"`
		} `json:"config,omitempty"`
		Scale []struct {
			Process string `json:"process"`
			From    *int   `json:"from,omitempty"`
			To      int    `json:"to"`
		} `json:"scale,omitempty"`
	} `json:"diff"`
	Summary string `json:"summary"`
}

// PreviewPending returns structured pending-vs-baseline preview for the client Environment.
func (c *Client) PreviewPending(ctx context.Context, project string) (*Preview, error) {
	var out Preview
	_, err := c.do(ctx, http.MethodGet, "/v1/projects/"+project+"/preview", nil, &out)
	return &out, err
}

// PreviewReleases compares two release versions.
func (c *Client) PreviewReleases(ctx context.Context, project string, fromV, toV int) (*Preview, error) {
	path := fmt.Sprintf("/v1/projects/%s/preview?from_release=%d&to_release=%d", project, fromV, toV)
	var out Preview
	_, err := c.do(ctx, http.MethodGet, path, nil, &out)
	return &out, err
}

// PreviewEnvironments compares last deployed releases in fromEnv vs toEnv.
func (c *Client) PreviewEnvironments(ctx context.Context, project, fromEnv, toEnv string) (*Preview, error) {
	q := url.Values{}
	q.Set("from_env", fromEnv)
	q.Set("to_env", toEnv)
	path := "/v1/projects/" + project + "/preview?" + q.Encode()
	var out Preview
	_, err := c.do(ctx, http.MethodGet, path, nil, &out)
	return &out, err
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

// UnstageLastResult is the response from DELETE …/changeset/changes/last.
type UnstageLastResult struct {
	Change struct {
		ID          string          `json:"id"`
		ServiceName string          `json:"service_name"`
		Type        string          `json:"type"`
		Payload     json.RawMessage `json:"payload"`
		CreatedAt   time.Time       `json:"created_at"`
	} `json:"change"`
	RemainingCount int    `json:"remaining_count"`
	Environment    string `json:"environment,omitempty"`
}

// UnstageLastChange removes the most recently staged change from the open changeset.
func (c *Client) UnstageLastChange(ctx context.Context, project string) (*UnstageLastResult, error) {
	var out UnstageLastResult
	_, err := c.do(ctx, http.MethodDelete, "/v1/projects/"+project+"/changeset/changes/last", nil, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
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
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Workspace      string   `json:"workspace"`
	Scopes         []string `json:"scopes"`
	Token          string   `json:"token"`
	PrincipalID    string   `json:"principal_id,omitempty"`
	PrincipalKind  string   `json:"principal_kind,omitempty"`
}

type AuditEvent struct {
	ID           string            `json:"id"`
	Action       string            `json:"action"`
	ResourceType string            `json:"resource_type"`
	ResourceID   string            `json:"resource_id"`
	PrincipalID  string            `json:"principal_id,omitempty"`
	TokenID      string            `json:"token_id,omitempty"`
	ProjectName  string            `json:"project_name,omitempty"`
	Detail       map[string]string `json:"detail,omitempty"`
	CreatedAt    string            `json:"created_at"`
}

func (c *Client) ListAudit(ctx context.Context, limit int) ([]AuditEvent, error) {
	path := "/v1/audit"
	if limit > 0 {
		path += "?limit=" + url.QueryEscape(fmt.Sprintf("%d", limit))
	}
	var events []AuditEvent
	_, err := c.do(ctx, http.MethodGet, path, nil, &events)
	return events, err
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
