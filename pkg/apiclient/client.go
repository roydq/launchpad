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

type App struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func (c *Client) CreateApp(ctx context.Context, name, team, namespace string) (*App, error) {
	var app App
	status, err := c.do(ctx, http.MethodPost, "/v1/apps", map[string]any{
		"name": name,
		"team": team,
		"target": map[string]any{
			"type":      "stub",
			"namespace": namespace,
		},
	}, &app)
	if err != nil {
		return nil, err
	}
	if status != http.StatusCreated {
		return nil, fmt.Errorf("create app: status %d", status)
	}
	return &app, nil
}

func (c *Client) Deploy(ctx context.Context, app, image, description string) (map[string]any, error) {
	var result map[string]any
	status, err := c.do(ctx, http.MethodPost, "/v1/apps/"+app+"/releases", map[string]any{
		"source":      map[string]string{"type": "image", "image": image},
		"description": description,
	}, &result)
	if err != nil {
		return nil, err
	}
	if status != http.StatusAccepted {
		return nil, fmt.Errorf("deploy: status %d", status)
	}
	return result, nil
}

func (c *Client) GetJob(ctx context.Context, id string) (map[string]any, error) {
	var job map[string]any
	_, err := c.do(ctx, http.MethodGet, "/v1/jobs/"+id, nil, &job)
	return job, err
}