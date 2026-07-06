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
	BaseURL string
	Token   string
	HTTP    *http.Client
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
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

type Project struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	PrimaryService string `json:"primary_service"`
	Status         string `json:"status"`
}

func (c *Client) CreateProject(ctx context.Context, name, targetType, namespace string) (*Project, error) {
	var project Project
	status, err := c.do(ctx, http.MethodPost, "/v1/projects", map[string]any{
		"name": name,
		"target": map[string]any{
			"type":      targetType,
			"namespace": namespace,
		},
	}, &project)
	if err != nil {
		return nil, err
	}
	if status != http.StatusCreated {
		return nil, fmt.Errorf("create project: status %d", status)
	}
	return &project, nil
}

func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	var projects []Project
	status, err := c.do(ctx, http.MethodGet, "/v1/projects", nil, &projects)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("list projects: status %d", status)
	}
	return projects, nil
}

func (c *Client) GetProject(ctx context.Context, name string) (*Project, error) {
	var project Project
	status, err := c.do(ctx, http.MethodGet, "/v1/projects/"+name, nil, &project)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("get project: status %d", status)
	}
	return &project, nil
}

func (c *Client) GetConfig(ctx context.Context, project string) (map[string]string, error) {
	var config map[string]string
	status, err := c.do(ctx, http.MethodGet, "/v1/projects/"+project+"/config", nil, &config)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("get config: status %d", status)
	}
	return config, nil
}

func (c *Client) PatchConfig(ctx context.Context, project string, updates map[string]*string) (map[string]string, error) {
	var config map[string]string
	status, err := c.do(ctx, http.MethodPatch, "/v1/projects/"+project+"/config", updates, &config)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("patch config: status %d", status)
	}
	return config, nil
}

func (c *Client) ListProcesses(ctx context.Context, project string) ([]map[string]any, error) {
	var processes []map[string]any
	status, err := c.do(ctx, http.MethodGet, "/v1/projects/"+project+"/processes", nil, &processes)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("list processes: status %d", status)
	}
	return processes, nil
}

func (c *Client) ListReleases(ctx context.Context, project string) ([]map[string]any, error) {
	var releases []map[string]any
	status, err := c.do(ctx, http.MethodGet, "/v1/projects/"+project+"/releases", nil, &releases)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("list releases: status %d", status)
	}
	return releases, nil
}

func (c *Client) Deploy(ctx context.Context, project, image, description string) (map[string]any, error) {
	var result map[string]any
	status, err := c.do(ctx, http.MethodPost, "/v1/projects/"+project+"/releases", map[string]any{
		"source": map[string]string{"type": "image", "image": image}, "description": description,
	}, &result)
	if err != nil {
		return nil, err
	}
	if status != http.StatusAccepted {
		return nil, fmt.Errorf("deploy: status %d", status)
	}
	return result, nil
}

func (c *Client) GetChangeset(ctx context.Context, project string) (map[string]any, error) {
	var result map[string]any
	_, err := c.do(ctx, http.MethodGet, "/v1/projects/"+project+"/changeset", nil, &result)
	return result, err
}

func (c *Client) StageChanges(ctx context.Context, project string, changes []map[string]any) (map[string]any, error) {
	var result map[string]any
	status, err := c.do(ctx, http.MethodPost, "/v1/projects/"+project+"/changeset/changes", map[string]any{"changes": changes}, &result)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("stage: status %d", status)
	}
	return result, nil
}

func (c *Client) PushChangeset(ctx context.Context, project, description string) (map[string]any, error) {
	var result map[string]any
	status, err := c.do(ctx, http.MethodPost, "/v1/projects/"+project+"/changeset/push", map[string]string{"description": description}, &result)
	if err != nil {
		return nil, err
	}
	if status != http.StatusAccepted {
		return nil, fmt.Errorf("push: status %d", status)
	}
	return result, nil
}

func (c *Client) DiscardChangeset(ctx context.Context, project string) error {
	status, err := c.do(ctx, http.MethodDelete, "/v1/projects/"+project+"/changeset", nil, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("discard changeset: status %d", status)
	}
	return nil
}

func (c *Client) GetJob(ctx context.Context, id string) (map[string]any, error) {
	var job map[string]any
	_, err := c.do(ctx, http.MethodGet, "/v1/jobs/"+id, nil, &job)
	return job, err
}