package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// findProjectLocalConfig walks from dir toward root for .launchpad/config.
// It skips the user home config path so ~/.launchpad/config is only loaded as
// the global layer (not accidentally as a higher-precedence "project-local").
func findProjectLocalConfig(startDir string) (localConfig, string, error) {
	homeConfig := ""
	if home, err := os.UserHomeDir(); err == nil {
		homeConfig = filepath.Clean(filepath.Join(home, ".launchpad", "config"))
	}
	dir := startDir
	for {
		path := filepath.Clean(filepath.Join(dir, ".launchpad", "config"))
		if homeConfig != "" && path == homeConfig {
			parent := filepath.Dir(dir)
			if parent == dir {
				return localConfig{}, "", nil
			}
			dir = parent
			continue
		}
		data, err := os.ReadFile(path)
		if err == nil {
			var cfg localConfig
			if err := json.Unmarshal(data, &cfg); err != nil {
				return localConfig{}, path, err
			}
			return cfg, path, nil
		}
		if !os.IsNotExist(err) {
			return localConfig{}, path, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return localConfig{}, "", nil
		}
		dir = parent
	}
}

// mergeConfigLayers applies precedence: env vars > project-local > global home.
func mergeConfigLayers(global localConfig, projectLocal localConfig, envProject, envEnv string) Config {
	cfg := Config{
		APIURL: envOr("LAUNCHPAD_API_URL", "http://localhost:8080"),
		Token:  os.Getenv("LAUNCHPAD_TOKEN"),
	}
	// Project
	cfg.Project = global.Project
	if projectLocal.Project != "" {
		cfg.Project = projectLocal.Project
	}
	if envProject != "" {
		cfg.Project = envProject
	}
	// Environment
	cfg.Environment = global.Environment
	if projectLocal.Environment != "" {
		cfg.Environment = projectLocal.Environment
	}
	if envEnv != "" {
		cfg.Environment = envEnv
	}
	if cfg.Environment == "" {
		cfg.Environment = "dev"
	}
	return cfg
}
