package domain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/launchpad/launchpad/pkg/launchpad"
)

// DNSLabelName is the v1 name rule for projects, environments, and processes.
var DNSLabelName = regexp.MustCompile(`^[a-z][a-z0-9-]{1,62}$`)

const ManifestVersionV1 = 1

// Manifest is the launchpad.yaml v1 document (JSON on the wire, YAML on disk).
type Manifest struct {
	Version      int                            `json:"version" yaml:"version"`
	Project      string                         `json:"project" yaml:"project"`
	Processes    map[string]ManifestProcess     `json:"processes,omitempty" yaml:"processes,omitempty"`
	Environments map[string]ManifestEnvironment `json:"environments,omitempty" yaml:"environments,omitempty"`
}

// ManifestProcess is one process in the document.
type ManifestProcess struct {
	Command          string                     `json:"command" yaml:"command"`
	Quantity         *int                       `json:"quantity,omitempty" yaml:"quantity,omitempty"`
	Expose           string                     `json:"expose,omitempty" yaml:"expose,omitempty"`
	Health           *ProcessHealth             `json:"health,omitempty" yaml:"health,omitempty"`
	TargetExtensions map[string]json.RawMessage `json:"target_extensions,omitempty" yaml:"target_extensions,omitempty"`
}

// ManifestEnvironment is one environment block.
type ManifestEnvironment struct {
	Target    string         `json:"target" yaml:"target"`
	Namespace string         `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Cluster   string         `json:"cluster,omitempty" yaml:"cluster,omitempty"`
	Ephemeral bool           `json:"ephemeral,omitempty" yaml:"ephemeral,omitempty"`
	Image     string         `json:"image,omitempty" yaml:"image,omitempty"`
	Config    ManifestConfig `json:"config,omitempty" yaml:"config,omitempty"`
}

// ManifestConfig is layered plain values plus secret key names (never values).
type ManifestConfig struct {
	Shared     map[string]string `json:"shared,omitempty" yaml:"shared,omitempty"`
	Service    map[string]string `json:"service,omitempty" yaml:"service,omitempty"`
	SecretKeys ManifestSecretKeys `json:"secret_keys,omitempty" yaml:"secret_keys,omitempty"`
	Workspace  json.RawMessage   `json:"workspace,omitempty" yaml:"workspace,omitempty"`
}

// ManifestSecretKeys lists secret names per layer.
type ManifestSecretKeys struct {
	Shared  []string `json:"shared,omitempty" yaml:"shared,omitempty"`
	Service []string `json:"service,omitempty" yaml:"service,omitempty"`
}

var (
	manifestDeferredTop = map[string]struct{}{
		"services": {},
		"bindings": {},
	}
	manifestAllowedTop = map[string]struct{}{
		"version": {}, "project": {}, "processes": {}, "environments": {},
		"primary_service": {}, "service": {},
	}
)

// ValidateManifestMap rejects unknown/deferred keys and illegal config value shapes
// on a generic decoded object (JSON or YAML). Call before unmarshalling into Manifest.
func ValidateManifestMap(raw map[string]any) error {
	if raw == nil {
		return fmt.Errorf("%w: empty manifest", launchpad.ErrBadRequest)
	}
	for k, v := range raw {
		if _, def := manifestDeferredTop[k]; def {
			return fmt.Errorf("%w: manifest field is deferred: %s", launchpad.ErrBadRequest, k)
		}
		if k == "primary_service" || k == "service" {
			proj, _ := raw["project"].(string)
			s, _ := v.(string)
			if s != "" && s != proj {
				return fmt.Errorf("%w: manifest field is deferred: %s", launchpad.ErrBadRequest, k)
			}
			continue
		}
		if _, ok := manifestAllowedTop[k]; !ok {
			return fmt.Errorf("%w: unknown manifest field %s", launchpad.ErrBadRequest, k)
		}
	}
	envs, _ := raw["environments"].(map[string]any)
	for envName, envVal := range envs {
		envMap, ok := envVal.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: environment %s must be a mapping", launchpad.ErrBadRequest, envName)
		}
		cfg, _ := envMap["config"].(map[string]any)
		if cfg == nil {
			continue
		}
		if _, ok := cfg["workspace"]; ok {
			return fmt.Errorf("%w: manifest field is deferred: config.workspace", launchpad.ErrBadRequest)
		}
		if err := validateConfigValueMap(cfg, "shared"); err != nil {
			return err
		}
		if err := validateConfigValueMap(cfg, "service"); err != nil {
			return err
		}
	}
	return nil
}

func validateConfigValueMap(cfg map[string]any, layer string) error {
	vals, ok := cfg[layer]
	if !ok || vals == nil {
		return nil
	}
	m, ok := vals.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: config.%s must be a mapping", launchpad.ErrBadRequest, layer)
	}
	for k, v := range m {
		switch t := v.(type) {
		case nil:
			return fmt.Errorf("%w: config.%s.%s must not be null", launchpad.ErrBadRequest, layer, k)
		case string:
			if strings.Contains(t, "${{") {
				return fmt.Errorf("%w: manifest field is deferred: bindings in config.%s.%s", launchpad.ErrBadRequest, layer, k)
			}
		case bool, int, int64, float64, json.Number:
			// stringify happens in the CLI YAML path; JSON numbers are accepted and stringified later
		case map[string]any:
			if _, secret := t["secret"]; secret {
				return fmt.Errorf("%w: manifest must not contain a secret value", launchpad.ErrBadRequest)
			}
			return fmt.Errorf("%w: config.%s.%s must be a scalar string", launchpad.ErrBadRequest, layer, k)
		default:
			return fmt.Errorf("%w: config.%s.%s must be a scalar string", launchpad.ErrBadRequest, layer, k)
		}
	}
	return nil
}

// StringifyConfigMaps converts numeric/bool JSON values in a raw document's
// config.shared/service maps to strings. Call after ValidateManifestMap.
func StringifyConfigMaps(raw map[string]any) {
	envs, _ := raw["environments"].(map[string]any)
	for _, envVal := range envs {
		envMap, ok := envVal.(map[string]any)
		if !ok {
			continue
		}
		cfg, _ := envMap["config"].(map[string]any)
		if cfg == nil {
			continue
		}
		stringifyLayer(cfg, "shared")
		stringifyLayer(cfg, "service")
	}
}

func stringifyLayer(cfg map[string]any, layer string) {
	vals, ok := cfg[layer].(map[string]any)
	if !ok {
		return
	}
	out := make(map[string]any, len(vals))
	for k, v := range vals {
		switch t := v.(type) {
		case string:
			out[k] = t
		case bool:
			if t {
				out[k] = "true"
			} else {
				out[k] = "false"
			}
		case json.Number:
			out[k] = t.String()
		case float64:
			if t == float64(int64(t)) {
				out[k] = fmt.Sprintf("%d", int64(t))
			} else {
				out[k] = fmt.Sprintf("%v", t)
			}
		case int:
			out[k] = fmt.Sprintf("%d", t)
		case int64:
			out[k] = fmt.Sprintf("%d", t)
		default:
			out[k] = fmt.Sprintf("%v", t)
		}
	}
	cfg[layer] = out
}

// ValidateManifest checks a typed v1 document (after map validation).
func ValidateManifest(m *Manifest) error {
	if m == nil {
		return fmt.Errorf("%w: empty manifest", launchpad.ErrBadRequest)
	}
	if m.Version != ManifestVersionV1 {
		return fmt.Errorf("%w: unsupported manifest version %d", launchpad.ErrBadRequest, m.Version)
	}
	if !DNSLabelName.MatchString(m.Project) {
		return fmt.Errorf("%w: invalid project name", launchpad.ErrBadRequest)
	}
	if len(m.Environments) == 0 {
		return fmt.Errorf("%w: manifest environments is required", launchpad.ErrBadRequest)
	}
	for name, p := range m.Processes {
		if !DNSLabelName.MatchString(name) {
			return fmt.Errorf("%w: invalid process name %q", launchpad.ErrBadRequest, name)
		}
		if p.Expose != "" && p.Expose != "http" && p.Expose != "tcp" && p.Expose != "none" {
			return fmt.Errorf("%w: invalid process expose %q", launchpad.ErrBadRequest, p.Expose)
		}
		if p.Quantity != nil && *p.Quantity < 0 {
			return fmt.Errorf("%w: process %s quantity must be >= 0", launchpad.ErrBadRequest, name)
		}
	}
	for name, env := range m.Environments {
		if !DNSLabelName.MatchString(name) {
			return fmt.Errorf("%w: invalid environment name %q", launchpad.ErrBadRequest, name)
		}
		if err := validateManifestConfig(env.Config); err != nil {
			return err
		}
	}
	return nil
}

func validateManifestConfig(cfg ManifestConfig) error {
	if len(cfg.Workspace) > 0 && string(cfg.Workspace) != "null" {
		return fmt.Errorf("%w: manifest field is deferred: config.workspace", launchpad.ErrBadRequest)
	}
	if err := rejectSecretOverlap(cfg.Shared, cfg.SecretKeys.Shared); err != nil {
		return err
	}
	if err := rejectSecretOverlap(cfg.Service, cfg.SecretKeys.Service); err != nil {
		return err
	}
	for _, m := range []map[string]string{cfg.Shared, cfg.Service} {
		for k, v := range m {
			if strings.Contains(v, "${{") {
				return fmt.Errorf("%w: manifest field is deferred: bindings in %s", launchpad.ErrBadRequest, k)
			}
		}
	}
	return nil
}

func rejectSecretOverlap(values map[string]string, secretKeys []string) error {
	if len(values) == 0 || len(secretKeys) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(secretKeys))
	for _, k := range secretKeys {
		set[k] = struct{}{}
	}
	for k := range values {
		if _, ok := set[k]; ok {
			return fmt.Errorf("%w: manifest must not contain a secret value", launchpad.ErrBadRequest)
		}
	}
	return nil
}

// ApplyProcessDefaults fills omitted apply fields per the v1 spec.
func ApplyProcessDefaults(name string, p ManifestProcess) ManifestProcess {
	if p.Quantity == nil {
		q := 1
		p.Quantity = &q
	}
	if p.Expose == "" {
		if name == "web" {
			p.Expose = "http"
		} else {
			p.Expose = "none"
		}
	}
	if p.Health != nil && (p.Health.Type == "" || p.Health.Type == "none") {
		p.Health = nil
	}
	if p.TargetExtensions == nil {
		p.TargetExtensions = map[string]json.RawMessage{}
	}
	return p
}

// RequireDevEnvironment is the create-project bootstrap rule.
func RequireDevEnvironment(m *Manifest) error {
	if m == nil || m.Environments == nil {
		return fmt.Errorf("%w: creating a project requires environments.dev", launchpad.ErrBadRequest)
	}
	if _, ok := m.Environments["dev"]; !ok {
		return fmt.Errorf("%w: creating a project requires environments.dev", launchpad.ErrBadRequest)
	}
	return nil
}

// RequireSelectedEnvironment ensures apply's selected env is in the document.
func RequireSelectedEnvironment(m *Manifest, env string) error {
	if m == nil || m.Environments == nil {
		return fmt.Errorf("%w: selected environment is not in the manifest", launchpad.ErrBadRequest)
	}
	if _, ok := m.Environments[env]; !ok {
		return fmt.Errorf("%w: selected environment %q is not in the manifest", launchpad.ErrBadRequest, env)
	}
	return nil
}
