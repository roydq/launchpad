package mcp

import (
	"context"
	"errors"
	"net/http"

	"github.com/launchpad/launchpad/pkg/apiclient"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type emptyIn struct{}

type healthzOut struct {
	OK bool `json:"ok"`
}

func (r *runtime) healthz(ctx context.Context, _ *mcpsdk.CallToolRequest, _ emptyIn) (*mcpsdk.CallToolResult, healthzOut, error) {
	if err := r.cfg.Client("").Healthz(ctx); err != nil {
		return nil, healthzOut{}, wrapErr(err)
	}
	return nil, healthzOut{OK: true}, nil
}

type projectIn struct {
	Project     string `json:"project,omitempty" jsonschema:"project name"`
	Environment string `json:"environment,omitempty" jsonschema:"environment name (ambient header)"`
}

func (r *runtime) scoped(explicitProject, explicitEnv string) (string, string, *apiclient.Client, error) {
	if err := r.cfg.RequireToken(); err != nil {
		return "", "", nil, err
	}
	project, err := r.cfg.ResolveProject(explicitProject)
	if err != nil {
		return "", "", nil, err
	}
	env := r.cfg.ResolveEnv(explicitEnv)
	return project, env, r.cfg.Client(env), nil
}

type listProjectsOut struct {
	Projects []apiclient.Project `json:"projects"`
}

func (r *runtime) listProjects(ctx context.Context, _ *mcpsdk.CallToolRequest, _ emptyIn) (*mcpsdk.CallToolResult, listProjectsOut, error) {
	if err := r.cfg.RequireToken(); err != nil {
		return nil, listProjectsOut{}, err
	}
	projects, err := r.cfg.Client("").ListProjects(ctx)
	if err != nil {
		return nil, listProjectsOut{}, wrapErr(err)
	}
	if projects == nil {
		projects = []apiclient.Project{}
	}
	return nil, listProjectsOut{Projects: projects}, nil
}

func (r *runtime) getProject(ctx context.Context, _ *mcpsdk.CallToolRequest, in projectIn) (*mcpsdk.CallToolResult, *apiclient.Project, error) {
	project, _, cl, err := r.scoped(in.Project, in.Environment)
	if err != nil {
		return nil, nil, err
	}
	p, err := cl.GetProject(ctx, project)
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	return nil, p, nil
}

type listEnvironmentsOut struct {
	Environments []apiclient.Environment `json:"environments"`
}

func (r *runtime) listEnvironments(ctx context.Context, _ *mcpsdk.CallToolRequest, in projectIn) (*mcpsdk.CallToolResult, any, error) {
	project, _, cl, err := r.scoped(in.Project, in.Environment)
	if err != nil {
		return nil, nil, err
	}
	envs, err := cl.ListEnvironments(ctx, project)
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	if envs == nil {
		envs = []apiclient.Environment{}
	}
	v, err := jsonAny(listEnvironmentsOut{Environments: envs})
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	return nil, v, nil
}

type getEnvironmentIn struct {
	Project     string `json:"project,omitempty"`
	Environment string `json:"environment,omitempty"`
	Name        string `json:"name,omitempty" jsonschema:"environment name (default resolved env)"`
}

func (r *runtime) getEnvironment(ctx context.Context, _ *mcpsdk.CallToolRequest, in getEnvironmentIn) (*mcpsdk.CallToolResult, any, error) {
	project, env, cl, err := r.scoped(in.Project, in.Environment)
	if err != nil {
		return nil, nil, err
	}
	name := in.Name
	if name == "" {
		name = env
	}
	out, err := cl.GetEnvironment(ctx, project, name)
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	v, err := jsonAny(out)
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	return nil, v, nil
}

type getConfigIn struct {
	Project     string `json:"project,omitempty"`
	Environment string `json:"environment,omitempty"`
	Layer       string `json:"layer,omitempty" jsonschema:"shared or service"`
}

func (r *runtime) getConfig(ctx context.Context, _ *mcpsdk.CallToolRequest, in getConfigIn) (*mcpsdk.CallToolResult, map[string]string, error) {
	project, _, cl, err := r.scoped(in.Project, in.Environment)
	if err != nil {
		return nil, nil, err
	}
	cfg, err := cl.GetConfigLayer(ctx, project, in.Layer)
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	if cfg == nil {
		cfg = map[string]string{}
	}
	return nil, cfg, nil
}

type listProcessesOut struct {
	Processes []apiclient.Process `json:"processes"`
}

func (r *runtime) listProcesses(ctx context.Context, _ *mcpsdk.CallToolRequest, in projectIn) (*mcpsdk.CallToolResult, listProcessesOut, error) {
	project, _, cl, err := r.scoped(in.Project, in.Environment)
	if err != nil {
		return nil, listProcessesOut{}, err
	}
	procs, err := cl.ListProcesses(ctx, project)
	if err != nil {
		return nil, listProcessesOut{}, wrapErr(err)
	}
	if procs == nil {
		procs = []apiclient.Process{}
	}
	return nil, listProcessesOut{Processes: procs}, nil
}

type listReleasesOut struct {
	Releases []apiclient.Release `json:"releases"`
}

func (r *runtime) listReleases(ctx context.Context, _ *mcpsdk.CallToolRequest, in projectIn) (*mcpsdk.CallToolResult, listReleasesOut, error) {
	project, _, cl, err := r.scoped(in.Project, in.Environment)
	if err != nil {
		return nil, listReleasesOut{}, err
	}
	rels, err := cl.ListReleases(ctx, project)
	if err != nil {
		return nil, listReleasesOut{}, wrapErr(err)
	}
	if rels == nil {
		rels = []apiclient.Release{}
	}
	return nil, listReleasesOut{Releases: rels}, nil
}

func (r *runtime) getChangeset(ctx context.Context, _ *mcpsdk.CallToolRequest, in projectIn) (*mcpsdk.CallToolResult, any, error) {
	project, _, cl, err := r.scoped(in.Project, in.Environment)
	if err != nil {
		return nil, nil, err
	}
	cs, err := cl.GetChangeset(ctx, project)
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	v, err := jsonAny(cs)
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	return nil, redactSecrets(v), nil
}

type previewIn struct {
	Project     string `json:"project,omitempty"`
	Environment string `json:"environment,omitempty"`
	FromRelease *int   `json:"from_release,omitempty"`
	ToRelease   *int   `json:"to_release,omitempty"`
	FromEnv     string `json:"from_env,omitempty"`
	ToEnv       string `json:"to_env,omitempty"`
}

func (r *runtime) preview(ctx context.Context, _ *mcpsdk.CallToolRequest, in previewIn) (*mcpsdk.CallToolResult, *apiclient.Preview, error) {
	project, _, cl, err := r.scoped(in.Project, in.Environment)
	if err != nil {
		return nil, nil, err
	}
	releasePair := in.FromRelease != nil || in.ToRelease != nil
	envPair := in.FromEnv != "" || in.ToEnv != ""
	if releasePair && envPair {
		return nil, nil, errJSON("preview: set either from_release/to_release or from_env/to_env, not both", nil)
	}
	if releasePair {
		if in.FromRelease == nil || in.ToRelease == nil {
			return nil, nil, errJSON("preview: from_release and to_release are both required", nil)
		}
		out, err := cl.PreviewReleases(ctx, project, *in.FromRelease, *in.ToRelease)
		if err != nil {
			return nil, nil, wrapErr(err)
		}
		return nil, out, nil
	}
	if envPair {
		if in.FromEnv == "" || in.ToEnv == "" {
			return nil, nil, errJSON("preview: from_env and to_env are both required", nil)
		}
		out, err := cl.PreviewEnvironments(ctx, project, in.FromEnv, in.ToEnv)
		if err != nil {
			return nil, nil, wrapErr(err)
		}
		return nil, out, nil
	}
	out, err := cl.PreviewPending(ctx, project)
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	return nil, out, nil
}

type getManifestIn struct {
	Project     string `json:"project,omitempty"`
	Environment string `json:"environment,omitempty" jsonschema:"optional export filter; omit for all environments"`
}

func (r *runtime) getManifest(ctx context.Context, _ *mcpsdk.CallToolRequest, in getManifestIn) (*mcpsdk.CallToolResult, map[string]any, error) {
	if err := r.cfg.RequireToken(); err != nil {
		return nil, nil, err
	}
	project, err := r.cfg.ResolveProject(in.Project)
	if err != nil {
		return nil, nil, err
	}
	cl := r.cfg.Client("")
	doc, err := cl.GetManifest(ctx, project, in.Environment)
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	return nil, doc, nil
}

type getJobIn struct {
	ID string `json:"id" jsonschema:"job id"`
}

func (r *runtime) getJob(ctx context.Context, _ *mcpsdk.CallToolRequest, in getJobIn) (*mcpsdk.CallToolResult, *apiclient.Job, error) {
	if err := r.cfg.RequireToken(); err != nil {
		return nil, nil, err
	}
	if in.ID == "" {
		return nil, nil, errJSON("id is required", nil)
	}
	job, err := r.cfg.Client("").GetJob(ctx, in.ID)
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	return nil, job, nil
}

type getLogsIn struct {
	Project     string `json:"project,omitempty"`
	Environment string `json:"environment,omitempty"`
	Process     string `json:"process,omitempty" jsonschema:"process name (default web)"`
}

type logsOut struct {
	Logs string `json:"logs"`
}

func (r *runtime) getLogs(ctx context.Context, _ *mcpsdk.CallToolRequest, in getLogsIn) (*mcpsdk.CallToolResult, logsOut, error) {
	project, _, cl, err := r.scoped(in.Project, in.Environment)
	if err != nil {
		return nil, logsOut{}, err
	}
	text, err := cl.GetLogs(ctx, project, in.Process)
	if err != nil {
		return nil, logsOut{}, wrapErr(err)
	}
	return nil, logsOut{Logs: text}, nil
}

type inspectOut struct {
	Project      string         `json:"project"`
	Environment  string         `json:"environment"`
	Status       string         `json:"status"`
	PendingCount int            `json:"pending_count"`
	LastDeploy   *lastDeployOut `json:"last_deploy"`
	Processes    []inspectProc  `json:"processes"`
}

type inspectProc struct {
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
	Expose   string `json:"expose"`
}

type lastDeployOut struct {
	Version     int    `json:"version"`
	ArtifactRef string `json:"artifact_ref"`
	Status      string `json:"status"`
}

func (r *runtime) inspect(ctx context.Context, _ *mcpsdk.CallToolRequest, in projectIn) (*mcpsdk.CallToolResult, inspectOut, error) {
	project, env, cl, err := r.scoped(in.Project, in.Environment)
	if err != nil {
		return nil, inspectOut{}, err
	}
	out := inspectOut{
		Project:     project,
		Environment: env,
		Processes:   []inspectProc{},
	}
	if p, err := cl.GetProject(ctx, project); err == nil && p != nil {
		out.Status = p.Status
	} else if err != nil {
		return nil, inspectOut{}, wrapErr(err)
	}
	cs, err := cl.GetChangeset(ctx, project)
	if err != nil {
		var ae *apiclient.APIError
		if !(errors.As(err, &ae) && ae.Status == http.StatusNotFound) {
			return nil, inspectOut{}, wrapErr(err)
		}
	} else if cs != nil {
		out.PendingCount = len(cs.Changes)
	}
	rel, err := latestReleaseForEnv(ctx, cl, project, env)
	if err != nil {
		return nil, inspectOut{}, wrapErr(err)
	}
	if rel != nil {
		out.LastDeploy = &lastDeployOut{Version: rel.Version, ArtifactRef: rel.ArtifactRef, Status: rel.Status}
	}
	procs, err := cl.ListProcesses(ctx, project)
	if err != nil {
		return nil, inspectOut{}, wrapErr(err)
	}
	for _, p := range procs {
		out.Processes = append(out.Processes, inspectProc{Name: p.Name, Quantity: p.Quantity, Expose: p.Expose})
	}
	return nil, out, nil
}

type targetCapabilitiesIn struct {
	Project     string `json:"project,omitempty"`
	Environment string `json:"environment,omitempty"`
	Type        string `json:"type,omitempty" jsonschema:"target type; if omitted, use the environment target"`
}

func (r *runtime) targetCapabilities(ctx context.Context, _ *mcpsdk.CallToolRequest, in targetCapabilitiesIn) (*mcpsdk.CallToolResult, map[string]any, error) {
	if err := r.cfg.RequireToken(); err != nil {
		return nil, nil, err
	}
	typeName := in.Type
	if typeName == "" {
		project, env, cl, err := r.scoped(in.Project, in.Environment)
		if err != nil {
			return nil, nil, err
		}
		e, err := cl.GetEnvironment(ctx, project, env)
		if err != nil {
			return nil, nil, wrapErr(err)
		}
		typeName = e.TargetType
	}
	out, err := r.cfg.Client("").TargetCapabilities(ctx, typeName)
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	return nil, out, nil
}

func latestReleaseForEnv(ctx context.Context, client *apiclient.Client, project, env string) (*apiclient.Release, error) {
	releases, err := client.ListReleases(ctx, project)
	if err != nil {
		return nil, err
	}
	for i := range releases {
		for _, d := range releases[i].Deployments {
			if d.Environment == env && d.Status == "running" {
				return &releases[i], nil
			}
		}
	}
	for i := range releases {
		for _, d := range releases[i].Deployments {
			if d.Environment == env {
				return &releases[i], nil
			}
		}
	}
	return nil, nil
}
