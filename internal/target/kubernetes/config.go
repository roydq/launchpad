package kubernetes

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/launchpad/launchpad/internal/domain"
)

const (
	annotationReleaseVersion = "launchpad.dev/release-version"
	annotationAppName          = "launchpad.dev/app"
	labelApp                   = "launchpad.dev/app"
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
	Namespace string `json:"namespace"`
	Cluster   string `json:"cluster"`
}

func parseTargetConfig(app domain.App) (targetConfig, error) {
	var cfg targetConfig
	if len(app.TargetConfig) == 0 {
		return cfg, fmt.Errorf("target_config is required")
	}
	if err := json.Unmarshal(app.TargetConfig, &cfg); err != nil {
		return cfg, fmt.Errorf("parse target_config: %w", err)
	}
	if cfg.Namespace == "" {
		return cfg, fmt.Errorf("target_config.namespace is required")
	}
	return cfg, nil
}

func resourcePrefix(appName string) string {
	return "launchpad-" + appName
}

func deploymentName(appName, process string) string {
	return resourcePrefix(appName) + "-" + process
}

func secretName(appName string) string {
	return resourcePrefix(appName) + "-config"
}

func serviceName(appName, process string) string {
	return resourcePrefix(appName) + "-" + process
}

func appLabels(appName, component string) map[string]string {
	return map[string]string{
		labelApp:       appName,
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