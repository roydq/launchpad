package mcp

import (
	"log/slog"
	"os"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const instructions = `Launchpad control plane for one primary service. apply_manifest stages desired state and does not deploy. Call preview then deploy (wait defaults true when omitted). Secret values are omitted by the API. Ambient default environment is dev. get_manifest without an environment argument returns all environments — do not pass environment=dev unless you intend to filter.`

type runtime struct {
	cfg Config
}

// NewServer builds the Launchpad MCP server (tools only).
func NewServer(cfg Config) *mcpsdk.Server {
	r := &runtime{cfg: cfg}
	s := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "launchpad", Version: "v0.1.0"}, &mcpsdk.ServerOptions{
		Instructions: instructions,
		Logger:       slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})),
	})
	r.registerReadTools(s)
	return s
}

func (r *runtime) registerReadTools(s *mcpsdk.Server) {
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "healthz", Description: "Check API liveness (no token required)"}, r.healthz)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "list_projects", Description: "List projects in the workspace"}, r.listProjects)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "get_project", Description: "Get a project by name"}, r.getProject)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "list_environments", Description: "List environments for a project"}, r.listEnvironments)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "get_environment", Description: "Get one environment"}, r.getEnvironment)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "get_config", Description: "Get resolved or layer config (secrets redacted)"}, r.getConfig)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "list_processes", Description: "List process definitions"}, r.listProcesses)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "list_releases", Description: "List releases for a project"}, r.listReleases)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "get_changeset", Description: "Get the open changeset"}, r.getChangeset)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "preview", Description: "Preview pending vs last deploy, or two releases/envs"}, r.preview)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "get_manifest", Description: "Export live launchpad.yaml document (omit environment for all envs)"}, r.getManifest)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "get_job", Description: "Get a deploy job by id"}, r.getJob)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "get_logs", Description: "Get process logs (default web)"}, r.getLogs)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "inspect", Description: "Snapshot project, pending count, last deploy, processes"}, r.inspect)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "target_capabilities", Description: "Target health and extension schema"}, r.targetCapabilities)
}

func (r *runtime) registerWriteTools(s *mcpsdk.Server) {
	// Registered in tools_write.go; stubbed until that file exists.
}
