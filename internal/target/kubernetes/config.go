package kubernetes

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/launchpad/launchpad/internal/domain"
)

const (
	annotationReleaseVersion = "launchpad.dev/release-version"
	annotationProjectName    = "launchpad.dev/project"
	annotationServiceName      = "launchpad.dev/service"
	labelProject               = "launchpad.dev/project"
	labelService               = "launchpad.dev/service"
	labelComponent             = "launchpad.dev/component"
	labelManagedBy             = "app.kubernetes.io/managed-by"

	managedByValue = "launchpad"
	defaultPort    = 8080
)

type Options struct {
	DeployTimeout time.Duration
	PollInterval  time.Duration
	Kubeconfig    string
}

type targetConfig struct {
	Namespace     string `json:"namespace"`
	Cluster       string `json:"cluster"`
	DeployTimeout string `json:"deploy_timeout,omitempty"` // e.g. "20m"
}

func (c targetConfig) deployTimeoutOr(fallback time.Duration) time.Duration {
	if c.DeployTimeout == "" {
		return fallback
	}
	d, err := time.ParseDuration(c.DeployTimeout)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

func parseTargetConfig(env domain.Environment) (targetConfig, error) {
	var cfg targetConfig
	if len(env.TargetConfig) == 0 {
		return cfg, fmt.Errorf("target_config is required")
	}
	if err := json.Unmarshal(env.TargetConfig, &cfg); err != nil {
		return cfg, fmt.Errorf("parse target_config: %w", err)
	}
	if cfg.Namespace == "" {
		return cfg, fmt.Errorf("target_config.namespace is required")
	}
	return cfg, nil
}

func resourcePrefix(projectName, serviceName string) string {
	return "launchpad-" + projectName + "-" + serviceName
}

func deploymentName(projectName, serviceName, process string) string {
	return resourcePrefix(projectName, serviceName) + "-" + process
}

func secretName(projectName, serviceName string) string {
	// Legacy stable name (Destroy still cleans it).
	return resourcePrefix(projectName, serviceName) + "-config"
}

// configSecretName returns a content-addressed immutable config Secret name.
func configSecretName(projectName, serviceName, configHash string) string {
	return resourcePrefix(projectName, serviceName) + "-cfg-" + configHash
}

func serviceName(projectName, serviceName, process string) string {
	return resourcePrefix(projectName, serviceName) + "-" + process
}

func resourceLabels(projectName, serviceName, component string) map[string]string {
	return map[string]string{
		labelProject:   projectName,
		labelService:   serviceName,
		labelComponent: component,
		labelManagedBy: managedByValue,
	}
}

func containerPort(config map[string]string) int32 {
	if port, ok := config["PORT"]; ok && port != "" {
		var p int
		if _, err := fmt.Sscanf(port, "%d", &p); err == nil && p > 0 {
			return int32(p)
		}
	}
	return defaultPort
}