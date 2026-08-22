package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/store"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

// ManifestService exports live project state and applies a v1 document by staging.
type ManifestService struct {
	store      *store.Store
	projects   *ProjectService
	config     *ConfigService
	changesets *ChangesetService
	releases   *ReleaseService
}

func NewManifestService(st *store.Store, projects *ProjectService, config *ConfigService, changesets *ChangesetService, releases *ReleaseService) *ManifestService {
	return &ManifestService{store: st, projects: projects, config: config, changesets: changesets, releases: releases}
}

// ApplyReport is the POST /manifest/apply response.
type ApplyReport struct {
	Project            string             `json:"project"`
	Environment        string             `json:"environment"`
	CreatedProject     bool               `json:"created_project"`
	CreatedEnvironment bool               `json:"created_environment"`
	Staged             []string           `json:"staged"`
	Unchanged          []string           `json:"unchanged"`
	Warnings           []string           `json:"warnings"`
	NeedsValue         []string           `json:"needs_value"`
	Changeset          *ApplyChangesetRef `json:"changeset"`
}

// ApplyChangesetRef is the open changeset after apply, if any.
type ApplyChangesetRef struct {
	ID          string `json:"id"`
	ChangeCount int    `json:"change_count"`
}

// Export returns the live manifest. envFilter empty = all environments.
func (s *ManifestService) Export(ctx context.Context, projectName, envFilter string) (*domain.Manifest, error) {
	project, err := s.projects.GetProject(ctx, projectName)
	if err != nil {
		return nil, err
	}
	svc, err := s.store.GetServiceByProjectAndName(ctx, project.ID, project.PrimaryService)
	if err != nil {
		return nil, err
	}
	procs, err := s.store.ListProcesses(ctx, svc.ID)
	if err != nil {
		return nil, err
	}
	doc := &domain.Manifest{
		Version:      domain.ManifestVersionV1,
		Project:      project.Name,
		Processes:    make(map[string]domain.ManifestProcess, len(procs)),
		Environments: map[string]domain.ManifestEnvironment{},
	}
	for _, p := range procs {
		q := p.Quantity
		mp := domain.ManifestProcess{
			Command:          p.Command,
			Quantity:         &q,
			Expose:           p.Expose,
			Health:           p.Health,
			TargetExtensions: p.TargetExtensions,
		}
		doc.Processes[p.Name] = mp
	}

	var envs []domain.Environment
	if envFilter != "" {
		env, err := s.projects.GetEnvironment(ctx, projectName, envFilter)
		if err != nil {
			return nil, err
		}
		envs = []domain.Environment{*env}
	} else {
		envs, err = s.projects.ListEnvironments(ctx, projectName)
		if err != nil {
			return nil, err
		}
	}
	for i := range envs {
		block, err := s.exportEnv(ctx, project, svc, &envs[i])
		if err != nil {
			return nil, err
		}
		doc.Environments[envs[i].Name] = block
	}
	return doc, nil
}

func (s *ManifestService) exportEnv(ctx context.Context, project *domain.Project, svc *domain.Service, env *domain.Environment) (domain.ManifestEnvironment, error) {
	ns, cluster := parseTargetConfig(env.TargetConfig)
	block := domain.ManifestEnvironment{
		Target:    env.TargetType,
		Namespace: ns,
		Cluster:   cluster,
		Ephemeral: env.Ephemeral,
	}
	img, err := s.imageForEnv(ctx, project.Name, env.Name)
	if err != nil {
		return block, err
	}
	block.Image = img
	sharedVals, sharedSens, err := s.store.ListSharedConfigVarsWithSensitivityTx(ctx, nil, project.ID, env.ID)
	if err != nil {
		return block, err
	}
	svcVals, svcSens, err := s.store.ListConfigVarsWithSensitivityTx(ctx, nil, svc.ID, env.ID)
	if err != nil {
		return block, err
	}
	block.Config.Shared, block.Config.SecretKeys.Shared = splitPlainAndSecrets(sharedVals, sharedSens)
	block.Config.Service, block.Config.SecretKeys.Service = splitPlainAndSecrets(svcVals, svcSens)
	return block, nil
}

func splitPlainAndSecrets(vals, sens map[string]string) (plain map[string]string, secrets []string) {
	plain = map[string]string{}
	for k, v := range vals {
		if domain.IsSecret(sens[k]) {
			secrets = append(secrets, k)
			continue
		}
		plain[k] = v
	}
	sort.Strings(secrets)
	return plain, secrets
}

// Apply creates the project/selected env as needed and stages diffs. It never pushes.
func (s *ManifestService) Apply(ctx context.Context, projectName, selectedEnv string, doc domain.Manifest) (*ApplyReport, error) {
	if err := domain.ValidateManifest(&doc); err != nil {
		return nil, err
	}
	if doc.Project != projectName {
		return nil, fmt.Errorf("%w: project name does not match manifest", launchpad.ErrBadRequest)
	}
	selectedEnv = normalizeEnvName(selectedEnv)
	if err := domain.RequireSelectedEnvironment(&doc, selectedEnv); err != nil {
		return nil, err
	}

	report := &ApplyReport{
		Project:     projectName,
		Environment: selectedEnv,
		Staged:      []string{},
		Unchanged:   []string{},
		Warnings:    []string{},
		NeedsValue:  []string{},
	}

	_, err := s.projects.GetProject(ctx, projectName)
	if errors.Is(err, launchpad.ErrNotFound) {
		if err := domain.RequireDevEnvironment(&doc); err != nil {
			return nil, err
		}
		dev := doc.Environments["dev"]
		if dev.Ephemeral {
			report.Warnings = append(report.Warnings, "ephemeral ignored on bootstrap")
		}
		if _, err := s.projects.CreateProject(ctx, CreateProjectInput{
			Name: projectName,
			Target: TargetInput{
				Type:      defaultString(dev.Target, "stub"),
				Namespace: defaultString(dev.Namespace, "default"),
				Cluster:   dev.Cluster,
			},
		}); err != nil {
			return nil, err
		}
		report.CreatedProject = true
	} else if err != nil {
		return nil, err
	}

	_, err = s.projects.GetEnvironment(ctx, projectName, selectedEnv)
	if errors.Is(err, launchpad.ErrNotFound) {
		block := doc.Environments[selectedEnv]
		if block.Target == "" {
			return nil, fmt.Errorf("%w: environments.%s.target is required when creating the environment", launchpad.ErrBadRequest, selectedEnv)
		}
		if _, err := s.projects.CreateEnvironment(ctx, projectName, CreateEnvironmentInput{
			Name: selectedEnv,
			Target: TargetInput{
				Type:      block.Target,
				Namespace: defaultString(block.Namespace, "default"),
				Cluster:   block.Cluster,
			},
			Ephemeral: block.Ephemeral,
		}); err != nil {
			return nil, err
		}
		report.CreatedEnvironment = true
	} else if err != nil {
		return nil, err
	} else {
		liveEnv, err := s.projects.GetEnvironment(ctx, projectName, selectedEnv)
		if err != nil {
			return nil, err
		}
		want := doc.Environments[selectedEnv]
		if want.Target != "" && liveEnv.TargetType != want.Target {
			report.Warnings = append(report.Warnings, fmt.Sprintf(
				"environment %s target is %s; manifest says %s (not changed)", selectedEnv, liveEnv.TargetType, want.Target))
		}
	}

	project, svc, env, err := s.projects.resolvePrimaryService(ctx, projectName, selectedEnv)
	if err != nil {
		return nil, err
	}

	open, err := s.store.GetOpenChangeset(ctx, project.ID)
	if err != nil && !errors.Is(err, launchpad.ErrNotFound) {
		return nil, err
	}
	overlay := open != nil && open.EnvironmentID != nil && *open.EnvironmentID == env.ID

	liveProcs, err := s.store.ListProcesses(ctx, svc.ID)
	if err != nil {
		return nil, err
	}
	effProcs := processMap(liveProcs)
	if overlay {
		effProcs = overlayProcesses(effProcs, open.Changes)
	}

	declaredProcs := doc.Processes
	var changes []StageChangeInput
	if declaredProcs != nil {
		declaredNames := map[string]struct{}{}
		for name, mp := range declaredProcs {
			declaredNames[name] = struct{}{}
			mp = domain.ApplyProcessDefaults(name, mp)
			live, ok := effProcs[name]
			if ok && processMatches(name, live, mp) {
				report.Unchanged = append(report.Unchanged, "process."+name)
				continue
			}
			ch := processSetChange(name, mp)
			changes = append(changes, ch)
			report.Staged = append(report.Staged, "process.set "+name)
		}
		for name := range effProcs {
			if _, ok := declaredNames[name]; !ok {
				report.Warnings = append(report.Warnings, fmt.Sprintf("live process %q is not in the manifest (not pruned)", name))
			}
		}
	}

	block := doc.Environments[selectedEnv]
	sharedVals, sharedSens, err := s.store.ListSharedConfigVarsWithSensitivityTx(ctx, nil, project.ID, env.ID)
	if err != nil {
		return nil, err
	}
	svcVals, svcSens, err := s.store.ListConfigVarsWithSensitivityTx(ctx, nil, svc.ID, env.ID)
	if err != nil {
		return nil, err
	}
	if overlay {
		sharedVals, sharedSens = overlayConfig(sharedVals, sharedSens, open.Changes, domain.ChangeTypeSharedConfig)
		svcVals, svcSens = overlayConfig(svcVals, svcSens, open.Changes, domain.ChangeTypeConfig)
	}

	cfgChanges, warnings, needs, err := diffConfigLayer("shared", block.Config.Shared, block.Config.SecretKeys.Shared, sharedVals, sharedSens)
	if err != nil {
		return nil, err
	}
	report.Warnings = append(report.Warnings, warnings...)
	report.NeedsValue = append(report.NeedsValue, needs...)
	for _, ch := range cfgChanges {
		changes = append(changes, ch)
		report.Staged = append(report.Staged, "config.shared."+ch.Key)
	}
	cfgChanges, warnings, needs, err = diffConfigLayer("service", block.Config.Service, block.Config.SecretKeys.Service, svcVals, svcSens)
	if err != nil {
		return nil, err
	}
	report.Warnings = append(report.Warnings, warnings...)
	report.NeedsValue = append(report.NeedsValue, needs...)
	for _, ch := range cfgChanges {
		changes = append(changes, ch)
		report.Staged = append(report.Staged, "config.service."+ch.Key)
	}

	if block.Image != "" {
		effImage := ""
		if overlay {
			effImage = overlayImage(open.Changes)
		}
		if effImage == "" {
			liveImg, err := s.imageForEnv(ctx, projectName, selectedEnv)
			if err != nil {
				return nil, err
			}
			effImage = liveImg
		}
		if block.Image != effImage {
			changes = append(changes, StageChangeInput{Type: "image", Image: block.Image})
			report.Staged = append(report.Staged, "image")
		} else {
			report.Unchanged = append(report.Unchanged, "image")
		}
	}

	sort.Strings(report.Staged)
	sort.Strings(report.Unchanged)
	sort.Strings(report.Warnings)
	sort.Strings(report.NeedsValue)

	if len(changes) > 0 {
		if _, err := s.changesets.StageChanges(ctx, projectName, selectedEnv, StageChangesInput{Changes: changes}); err != nil {
			return nil, err
		}
	}

	cs, err := s.changesets.GetChangeset(ctx, projectName, selectedEnv)
	if err != nil {
		return nil, err
	}
	if cs != nil && cs.ID != uuid.Nil && len(cs.Changes) > 0 {
		report.Changeset = &ApplyChangesetRef{ID: cs.ID.String(), ChangeCount: len(cs.Changes)}
	}
	return report, nil
}

func processSetChange(name string, mp domain.ManifestProcess) StageChangeInput {
	cmd := mp.Command
	qty := 1
	if mp.Quantity != nil {
		qty = *mp.Quantity
	}
	expose := mp.Expose
	ch := StageChangeInput{
		Type:     "process.set",
		Name:     name,
		Command:  &cmd,
		Quantity: &qty,
		Expose:   &expose,
	}
	if mp.Health == nil {
		ch.Health = &domain.ProcessHealth{Type: "none"}
	} else {
		ch.Health = mp.Health
	}
	if mp.TargetExtensions == nil {
		ch.TargetExtensions = map[string]json.RawMessage{}
	} else {
		ch.TargetExtensions = mp.TargetExtensions
	}
	return ch
}

func diffConfigLayer(layer string, declared map[string]string, secretKeys []string, liveVals, liveSens map[string]string) (changes []StageChangeInput, warnings, needs []string, err error) {
	secretSet := map[string]struct{}{}
	for _, k := range secretKeys {
		secretSet[k] = struct{}{}
	}
	for k, v := range declared {
		if _, isSecretName := secretSet[k]; isSecretName {
			return nil, nil, nil, fmt.Errorf("%w: manifest must not contain a secret value", launchpad.ErrBadRequest)
		}
		if domain.IsSecret(liveSens[k]) {
			return nil, nil, nil, fmt.Errorf("%w: manifest must not contain a secret value", launchpad.ErrBadRequest)
		}
		if liveVals[k] == v {
			continue
		}
		val := v
		ch := StageChangeInput{Type: "config", Key: k, Value: &val, Layer: layer}
		changes = append(changes, ch)
	}
	for _, k := range secretKeys {
		sens := liveSens[k]
		val := liveVals[k]
		if domain.IsSecret(sens) && val != "" {
			continue
		}
		if !domain.IsSecret(sens) && val != "" {
			warnings = append(warnings, fmt.Sprintf("secret key %s is plain; not changed", k))
			continue
		}
		needs = append(needs, k)
	}
	return changes, warnings, needs, nil
}

func processMap(list []domain.Process) map[string]domain.Process {
	out := make(map[string]domain.Process, len(list))
	for _, p := range list {
		out[p.Name] = p
	}
	return out
}

func overlayProcesses(live map[string]domain.Process, changes []domain.ChangesetChange) map[string]domain.Process {
	out := make(map[string]domain.Process, len(live))
	for k, v := range live {
		out[k] = v
	}
	for _, c := range changes {
		switch c.Type {
		case domain.ChangeTypeProcessSet:
			var p domain.ProcessSetPayload
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				continue
			}
			cur := out[p.Name]
			cur.Name = p.Name
			if p.Command != nil {
				cur.Command = *p.Command
			}
			if p.Quantity != nil {
				cur.Quantity = *p.Quantity
			}
			if p.Expose != nil {
				cur.Expose = *p.Expose
			}
			if p.Health != nil {
				if p.Health.Type == "none" || p.Health.Type == "" {
					cur.Health = nil
				} else {
					h := *p.Health
					cur.Health = &h
				}
			}
			if p.TargetExtensions != nil {
				cur.TargetExtensions = p.TargetExtensions
			}
			out[p.Name] = cur
		case domain.ChangeTypeProcessUnset:
			var p domain.ProcessUnsetPayload
			if json.Unmarshal(c.Payload, &p) == nil {
				delete(out, p.Name)
			}
		}
	}
	return out
}

func overlayConfig(vals, sens map[string]string, changes []domain.ChangesetChange, typ domain.ChangeType) (map[string]string, map[string]string) {
	outV := copyMap(vals)
	outS := copyMap(sens)
	for _, c := range changes {
		if c.Type != typ {
			continue
		}
		var p domain.ConfigChangePayload
		if err := json.Unmarshal(c.Payload, &p); err != nil {
			continue
		}
		if p.Value == nil {
			delete(outV, p.Key)
			delete(outS, p.Key)
			continue
		}
		outV[p.Key] = *p.Value
		if p.Sensitivity != nil {
			outS[p.Key] = domain.NormalizeSensitivity(*p.Sensitivity)
		} else if outS[p.Key] == "" {
			outS[p.Key] = domain.SensitivityPlain
		}
	}
	return outV, outS
}

func overlayImage(changes []domain.ChangesetChange) string {
	img := ""
	for _, c := range changes {
		if c.Type != domain.ChangeTypeImage {
			continue
		}
		var p domain.ImageChangePayload
		if err := json.Unmarshal(c.Payload, &p); err != nil {
			continue
		}
		img = p.ArtifactRef
	}
	return img
}

func processMatches(name string, live domain.Process, declared domain.ManifestProcess) bool {
	d := domain.ApplyProcessDefaults(name, declared)
	if live.Command != d.Command {
		return false
	}
	if d.Quantity == nil || live.Quantity != *d.Quantity {
		return false
	}
	if live.Expose != d.Expose {
		return false
	}
	if !healthEqual(live.Health, d.Health) {
		return false
	}
	return extensionsEqual(live.TargetExtensions, d.TargetExtensions)
}

func healthEqual(a, b *domain.ProcessHealth) bool {
	if a == nil || a.Type == "" || a.Type == "none" {
		a = nil
	}
	if b == nil || b.Type == "" || b.Type == "none" {
		b = nil
	}
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return bytes.Equal(ab, bb)
}

func extensionsEqual(a, b map[string]json.RawMessage) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return bytes.Equal(ab, bb)
}

// imageForEnv matches inspect: running deploy in env, else any deploy in env, else empty.
func (s *ManifestService) imageForEnv(ctx context.Context, projectName, envName string) (string, error) {
	rels, err := s.releases.ListReleases(ctx, projectName, envName)
	if err != nil {
		return "", err
	}
	for i := range rels {
		for _, d := range rels[i].Deployments {
			if d.Environment == envName && d.Status == string(domain.DeploymentRunning) {
				return rels[i].Release.ArtifactRef, nil
			}
		}
	}
	for i := range rels {
		for _, d := range rels[i].Deployments {
			if d.Environment == envName {
				return rels[i].Release.ArtifactRef, nil
			}
		}
	}
	return "", nil
}

func parseTargetConfig(raw json.RawMessage) (namespace, cluster string) {
	if len(raw) == 0 {
		return "", ""
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", ""
	}
	return m["namespace"], m["cluster"]
}

func copyMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
