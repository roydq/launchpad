package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/pkg/apiclient"
	"gopkg.in/yaml.v3"
)

func defaultApplyPath(dir string) (string, error) {
	yamlPath := filepath.Join(dir, "launchpad.yaml")
	if _, err := os.Stat(yamlPath); err == nil {
		return yamlPath, nil
	}
	ymlPath := filepath.Join(dir, "launchpad.yml")
	if _, err := os.Stat(ymlPath); err == nil {
		return ymlPath, nil
	}
	return yamlPath, os.ErrNotExist
}

func decodeManifestYAML(data []byte) (*domain.Manifest, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	if err := domain.ValidateManifestMap(raw); err != nil {
		return nil, err
	}
	domain.StringifyConfigMaps(raw)
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var doc domain.Manifest
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	if err := domain.ValidateManifest(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func encodeManifestYAML(doc *domain.Manifest) ([]byte, error) {
	b, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	var generic any
	if err := json.Unmarshal(b, &generic); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(generic); err != nil {
		return nil, err
	}
	_ = enc.Close()
	return buf.Bytes(), nil
}

func printApplyReport(rep *apiclient.ApplyReport) {
	if rep.CreatedProject {
		fmt.Printf("created project %s\n", rep.Project)
	}
	if rep.CreatedEnvironment {
		fmt.Printf("created environment %s\n", rep.Environment)
	}
	if len(rep.Staged) > 0 {
		fmt.Printf("staged: %s\n", strings.Join(rep.Staged, ", "))
	} else {
		fmt.Println("staged: none")
	}
	if len(rep.Unchanged) > 0 {
		fmt.Printf("unchanged: %s\n", strings.Join(rep.Unchanged, ", "))
	}
	for _, w := range rep.Warnings {
		fmt.Printf("warning: %s\n", w)
	}
	if len(rep.NeedsValue) > 0 {
		fmt.Printf("needs_value: %s\n", strings.Join(rep.NeedsValue, ", "))
	}
	if len(rep.Staged) > 0 {
		fmt.Println("next: launchpad diff && launchpad deploy --wait")
	}
}
