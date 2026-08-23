package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/pkg/apiclient"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"
)

func (r *runtime) registerWriteTools(s *mcpsdk.Server) {
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "create_project", Description: "Create a project (bootstraps dev + primary service + web)"}, r.createProject)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "create_environment", Description: "Create an environment"}, r.createEnvironment)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "clone_environment", Description: "Clone an environment (secrets as needs_value)"}, r.cloneEnvironment)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "apply_manifest", Description: "Stage launchpad.yaml document; never deploys"}, r.applyManifest)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "stage_config", Description: "Stage config set/unset into the changeset"}, r.stageConfig)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "stage_process", Description: "Stage process.set or process.unset"}, r.stageProcess)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "stage_image", Description: "Stage a container image"}, r.stageImage)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "stage_scale", Description: "Stage process quantity"}, r.stageScale)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "unstage_last", Description: "Unstage the most recent changeset mutation"}, r.unstageLast)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "discard_changeset", Description: "Discard the open changeset"}, r.discardChangeset)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "deploy", Description: "Push the changeset (wait defaults true)"}, r.deploy)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "rollback", Description: "Rollback to a prior release version"}, r.rollback)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "promote", Description: "Promote the running release from one env to another"}, r.promote)
}

type createProjectIn struct {
	Name      string `json:"name" jsonschema:"new project name"`
	Target    string `json:"target,omitempty" jsonschema:"target type (default stub)"`
	Namespace string `json:"namespace,omitempty" jsonschema:"target namespace (default default)"`
}

func (r *runtime) createProject(ctx context.Context, _ *mcpsdk.CallToolRequest, in createProjectIn) (*mcpsdk.CallToolResult, *apiclient.Project, error) {
	if err := r.cfg.RequireToken(); err != nil {
		return nil, nil, err
	}
	if in.Name == "" {
		return nil, nil, errJSON("name is required", nil)
	}
	target := in.Target
	if target == "" {
		target = "stub"
	}
	ns := in.Namespace
	if ns == "" {
		ns = "default"
	}
	p, err := r.cfg.Client("").CreateProject(ctx, in.Name, target, ns)
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	return nil, p, nil
}

type createEnvironmentIn struct {
	Project     string `json:"project,omitempty"`
	Environment string `json:"environment,omitempty"`
	Name        string `json:"name" jsonschema:"new environment name"`
	Target      string `json:"target,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
	Ephemeral   bool   `json:"ephemeral,omitempty"`
}

func (r *runtime) createEnvironment(ctx context.Context, _ *mcpsdk.CallToolRequest, in createEnvironmentIn) (*mcpsdk.CallToolResult, any, error) {
	project, _, cl, err := r.scoped(in.Project, in.Environment)
	if err != nil {
		return nil, nil, err
	}
	if in.Name == "" {
		return nil, nil, errJSON("name is required", nil)
	}
	target := in.Target
	if target == "" {
		target = "stub"
	}
	ns := in.Namespace
	if ns == "" {
		ns = "default"
	}
	env, err := cl.CreateEnvironment(ctx, project, in.Name, target, ns, in.Ephemeral)
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	v, err := jsonAny(env)
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	return nil, v, nil
}

type cloneEnvironmentIn struct {
	Project     string `json:"project,omitempty"`
	Environment string `json:"environment,omitempty"`
	From        string `json:"from" jsonschema:"source environment"`
	Name        string `json:"name" jsonschema:"destination environment name"`
	Target      string `json:"target,omitempty" jsonschema:"omit to copy from source"`
	Namespace   string `json:"namespace,omitempty" jsonschema:"omit to copy from source"`
	Ephemeral   bool   `json:"ephemeral,omitempty"`
}

func (r *runtime) cloneEnvironment(ctx context.Context, _ *mcpsdk.CallToolRequest, in cloneEnvironmentIn) (*mcpsdk.CallToolResult, any, error) {
	project, _, cl, err := r.scoped(in.Project, in.Environment)
	if err != nil {
		return nil, nil, err
	}
	if in.From == "" || in.Name == "" {
		return nil, nil, errJSON("from and name are required", nil)
	}
	out, err := cl.CloneEnvironment(ctx, project, in.From, in.Name, in.Target, in.Namespace, in.Ephemeral)
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	v, err := jsonAny(out)
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	return nil, v, nil
}

type applyManifestIn struct {
	Project     string         `json:"project,omitempty"`
	Environment string         `json:"environment,omitempty"`
	Document    map[string]any `json:"document,omitempty" jsonschema:"manifest object"`
	YAML        string         `json:"yaml,omitempty" jsonschema:"manifest YAML string"`
}

func (r *runtime) applyManifest(ctx context.Context, _ *mcpsdk.CallToolRequest, in applyManifestIn) (*mcpsdk.CallToolResult, *apiclient.ApplyReport, error) {
	if err := r.cfg.RequireToken(); err != nil {
		return nil, nil, err
	}
	hasDoc := in.Document != nil
	hasYAML := strings.TrimSpace(in.YAML) != ""
	if hasDoc == hasYAML {
		return nil, nil, errJSON("apply_manifest requires exactly one of document or yaml", nil)
	}
	var document any
	if hasYAML {
		var raw map[string]any
		if err := yaml.Unmarshal([]byte(in.YAML), &raw); err != nil {
			return nil, nil, errJSON("parse yaml: "+err.Error(), nil)
		}
		if err := domain.ValidateManifestMap(raw); err != nil {
			return nil, nil, wrapErr(err)
		}
		domain.StringifyConfigMaps(raw)
		document = raw
	} else {
		document = in.Document
	}
	docProject := ""
	if m, ok := document.(map[string]any); ok {
		docProject, _ = m["project"].(string)
	}
	project, err := r.cfg.ResolveProject(in.Project)
	if err != nil {
		if docProject == "" {
			return nil, nil, err
		}
		project = docProject
	}
	if in.Project != "" && docProject != "" && in.Project != docProject {
		return nil, nil, errJSON("project does not match document.project", nil)
	}
	env := in.Environment
	if env == "" {
		env = r.cfg.ResolveEnv("")
	}
	cl := r.cfg.Client(env)
	rep, err := cl.ApplyManifest(ctx, project, env, document)
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	return nil, rep, nil
}

type stageConfigIn struct {
	Project     string            `json:"project,omitempty"`
	Environment string            `json:"environment,omitempty"`
	Set         map[string]string `json:"set,omitempty"`
	Unset       []string          `json:"unset,omitempty"`
	Layer       string            `json:"layer,omitempty" jsonschema:"service (default) or shared"`
	Secret      bool              `json:"secret,omitempty"`
}

func (r *runtime) stageConfig(ctx context.Context, _ *mcpsdk.CallToolRequest, in stageConfigIn) (*mcpsdk.CallToolResult, any, error) {
	project, _, cl, err := r.scoped(in.Project, in.Environment)
	if err != nil {
		return nil, nil, err
	}
	if len(in.Set) == 0 && len(in.Unset) == 0 {
		return nil, nil, errJSON("stage_config requires set and/or unset", nil)
	}
	var changes []map[string]any
	keys := make([]string, 0, len(in.Set))
	for k := range in.Set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := in.Set[k]
		ch := map[string]any{"type": "config", "key": k, "value": v}
		if in.Layer == "shared" {
			ch["layer"] = "shared"
		}
		if in.Secret {
			ch["sensitivity"] = "secret"
		}
		changes = append(changes, ch)
	}
	for _, k := range in.Unset {
		ch := map[string]any{"type": "config", "key": k, "value": nil}
		if in.Layer == "shared" {
			ch["layer"] = "shared"
		}
		if in.Secret {
			ch["sensitivity"] = "secret"
		}
		changes = append(changes, ch)
	}
	cs, err := cl.StageChanges(ctx, project, changes)
	return jsonResult(ctx, cl, project, cs, err)
}

type processHealthIn struct {
	Type string `json:"type" jsonschema:"http, tcp, exec, or none"`
	Path string `json:"path,omitempty"`
	Port int    `json:"port,omitempty"`
}

type stageProcessIn struct {
	Project     string           `json:"project,omitempty"`
	Environment string           `json:"environment,omitempty"`
	Name        string           `json:"name" jsonschema:"process name"`
	Unset       bool             `json:"unset,omitempty"`
	Command     *string          `json:"command,omitempty"`
	Quantity    *int             `json:"quantity,omitempty"`
	Expose      *string          `json:"expose,omitempty"`
	Health      *processHealthIn `json:"health,omitempty"`
}

func (r *runtime) stageProcess(ctx context.Context, _ *mcpsdk.CallToolRequest, in stageProcessIn) (*mcpsdk.CallToolResult, any, error) {
	project, _, cl, err := r.scoped(in.Project, in.Environment)
	if err != nil {
		return nil, nil, err
	}
	if in.Name == "" {
		return nil, nil, errJSON("name is required", nil)
	}
	var ch map[string]any
	if in.Unset {
		ch = map[string]any{"type": "process.unset", "name": in.Name}
	} else {
		ch = map[string]any{"type": "process.set", "name": in.Name}
		if in.Command != nil {
			ch["command"] = *in.Command
		}
		if in.Quantity != nil {
			ch["quantity"] = *in.Quantity
		}
		if in.Expose != nil {
			ch["expose"] = *in.Expose
		}
		if in.Health != nil {
			h := map[string]any{"type": in.Health.Type}
			if in.Health.Path != "" {
				h["path"] = in.Health.Path
			}
			if in.Health.Port > 0 {
				h["port"] = in.Health.Port
			}
			ch["health"] = h
		}
	}
	cs, err := cl.StageChanges(ctx, project, []map[string]any{ch})
	return jsonResult(ctx, cl, project, cs, err)
}

type stageImageIn struct {
	Project     string `json:"project,omitempty"`
	Environment string `json:"environment,omitempty"`
	Image       string `json:"image" jsonschema:"container image ref"`
}

func (r *runtime) stageImage(ctx context.Context, _ *mcpsdk.CallToolRequest, in stageImageIn) (*mcpsdk.CallToolResult, any, error) {
	project, _, cl, err := r.scoped(in.Project, in.Environment)
	if err != nil {
		return nil, nil, err
	}
	if in.Image == "" {
		return nil, nil, errJSON("image is required", nil)
	}
	cs, err := cl.StageChanges(ctx, project, []map[string]any{{"type": "image", "image": in.Image}})
	return jsonResult(ctx, cl, project, cs, err)
}

type stageScaleIn struct {
	Project     string `json:"project,omitempty"`
	Environment string `json:"environment,omitempty"`
	Process     string `json:"process" jsonschema:"process name"`
	Quantity    int    `json:"quantity"`
}

func (r *runtime) stageScale(ctx context.Context, _ *mcpsdk.CallToolRequest, in stageScaleIn) (*mcpsdk.CallToolResult, any, error) {
	project, _, cl, err := r.scoped(in.Project, in.Environment)
	if err != nil {
		return nil, nil, err
	}
	if in.Process == "" {
		return nil, nil, errJSON("process is required", nil)
	}
	cs, err := cl.StageChanges(ctx, project, []map[string]any{
		{"type": "scale", "process": in.Process, "quantity": in.Quantity},
	})
	return jsonResult(ctx, cl, project, cs, err)
}

func (r *runtime) unstageLast(ctx context.Context, _ *mcpsdk.CallToolRequest, in projectIn) (*mcpsdk.CallToolResult, any, error) {
	project, _, cl, err := r.scoped(in.Project, in.Environment)
	if err != nil {
		return nil, nil, err
	}
	out, err := cl.UnstageLastChange(ctx, project)
	return jsonResult(ctx, cl, project, out, err)
}

type okOut struct {
	OK bool `json:"ok"`
}

func (r *runtime) discardChangeset(ctx context.Context, _ *mcpsdk.CallToolRequest, in projectIn) (*mcpsdk.CallToolResult, okOut, error) {
	project, _, cl, err := r.scoped(in.Project, in.Environment)
	if err != nil {
		return nil, okOut{}, err
	}
	if err := cl.DiscardChangeset(ctx, project); err != nil {
		return nil, okOut{}, wrapErr(err)
	}
	return nil, okOut{OK: true}, nil
}

type deployIn struct {
	Project        string            `json:"project,omitempty"`
	Environment    string            `json:"environment,omitempty"`
	Image          string            `json:"image,omitempty"`
	Set            map[string]string `json:"set,omitempty"`
	ScaleProcess   string            `json:"scale_process,omitempty"`
	ScaleQuantity  *int              `json:"scale_quantity,omitempty"`
	Message        string            `json:"message,omitempty"`
	Wait           *bool             `json:"wait,omitempty" jsonschema:"omit to wait (default true)"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
	Confirm        bool              `json:"confirm,omitempty"`
}

func (r *runtime) confirmSensitive(env string, confirm bool) error {
	if IsSensitiveEnv(env) && !confirm {
		return errJSON(fmt.Sprintf("refusing to modify sensitive environment %q without confirm", env), nil)
	}
	return nil
}

func (r *runtime) deploy(ctx context.Context, _ *mcpsdk.CallToolRequest, in deployIn) (*mcpsdk.CallToolResult, map[string]any, error) {
	project, env, cl, err := r.scoped(in.Project, in.Environment)
	if err != nil {
		return nil, nil, err
	}
	if err := r.confirmSensitive(env, in.Confirm); err != nil {
		return nil, nil, err
	}
	var changes []map[string]any
	if in.Image != "" {
		changes = append(changes, map[string]any{"type": "image", "image": in.Image})
	}
	for k, v := range in.Set {
		changes = append(changes, map[string]any{"type": "config", "key": k, "value": v})
	}
	if in.ScaleProcess != "" && in.ScaleQuantity != nil {
		changes = append(changes, map[string]any{"type": "scale", "process": in.ScaleProcess, "quantity": *in.ScaleQuantity})
	}
	if len(changes) > 0 {
		if _, err := cl.StageChanges(ctx, project, changes); err != nil {
			return nil, nil, wrapErr(err)
		}
	}
	cs, err := cl.GetChangeset(ctx, project)
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	if cs == nil || len(cs.Changes) == 0 {
		return nil, nil, errJSON("nothing to deploy", nil)
	}
	result, err := cl.PushChangeset(ctx, project, in.Message)
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	return r.afterEnqueue(ctx, cl, result, in.Wait, in.TimeoutSeconds)
}

type rollbackIn struct {
	Project        string `json:"project,omitempty"`
	Environment    string `json:"environment,omitempty"`
	Version        int    `json:"version"`
	Message        string `json:"message,omitempty"`
	Wait           *bool  `json:"wait,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
	Confirm        bool   `json:"confirm,omitempty"`
}

func (r *runtime) rollback(ctx context.Context, _ *mcpsdk.CallToolRequest, in rollbackIn) (*mcpsdk.CallToolResult, map[string]any, error) {
	project, env, cl, err := r.scoped(in.Project, in.Environment)
	if err != nil {
		return nil, nil, err
	}
	if err := r.confirmSensitive(env, in.Confirm); err != nil {
		return nil, nil, err
	}
	if in.Version <= 0 {
		return nil, nil, errJSON("version is required", nil)
	}
	result, err := cl.Rollback(ctx, project, in.Version, in.Message)
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	return r.afterEnqueue(ctx, cl, result, in.Wait, in.TimeoutSeconds)
}

type promoteIn struct {
	Project        string `json:"project,omitempty"`
	Environment    string `json:"environment,omitempty"`
	From           string `json:"from" jsonschema:"source environment"`
	To             string `json:"to,omitempty" jsonschema:"destination environment (default resolved env)"`
	Version        int    `json:"version,omitempty" jsonschema:"0 means running release in from"`
	Message        string `json:"message,omitempty"`
	Wait           *bool  `json:"wait,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
	Confirm        bool   `json:"confirm,omitempty"`
}

func (r *runtime) promote(ctx context.Context, _ *mcpsdk.CallToolRequest, in promoteIn) (*mcpsdk.CallToolResult, map[string]any, error) {
	project, env, cl, err := r.scoped(in.Project, in.Environment)
	if err != nil {
		return nil, nil, err
	}
	to := in.To
	if to == "" {
		to = env
	}
	if err := r.confirmSensitive(to, in.Confirm); err != nil {
		return nil, nil, err
	}
	if in.From == "" {
		return nil, nil, errJSON("from is required", nil)
	}
	result, err := cl.Promote(ctx, project, in.From, to, in.Version, in.Message)
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	return r.afterEnqueue(ctx, cl, result, in.Wait, in.TimeoutSeconds)
}

func (r *runtime) afterEnqueue(ctx context.Context, cl *apiclient.Client, result *apiclient.DeployResult, wait *bool, timeoutSec int) (*mcpsdk.CallToolResult, map[string]any, error) {
	if !waitEnabled(wait) {
		return nil, deployResultOut(result, nil, false), nil
	}
	seed := &apiclient.Job{}
	if result != nil {
		seed.ID = result.Job.ID
		seed.Type = result.Job.Type
		seed.Status = result.Job.Status
	}
	job, err := waitForJob(ctx, cl, seed, timeoutSec)
	if err != nil {
		return nil, nil, err
	}
	return nil, deployResultOut(result, job, true), nil
}
