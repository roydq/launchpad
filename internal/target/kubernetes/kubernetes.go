package kubernetes

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/launchpad/launchpad/internal/target"
)

// Target deploys Launchpad apps to a Kubernetes cluster.
type Target struct {
	clientFactory func(contextName string) (kubernetes.Interface, error)
	opts          Options
}

func New(client kubernetes.Interface, opts Options) *Target {
	if opts.DeployTimeout == 0 {
		opts.DeployTimeout = 15 * time.Minute
	}
	if opts.PollInterval == 0 {
		opts.PollInterval = 2 * time.Second
	}
	return &Target{
		clientFactory: func(string) (kubernetes.Interface, error) { return client, nil },
		opts:          opts,
	}
}

func NewFromEnv() (*Target, error) {
	opts := Options{
		DeployTimeout: envDuration("LAUNCHPAD_K8S_DEPLOY_TIMEOUT", 15*time.Minute),
		PollInterval:  envDuration("LAUNCHPAD_K8S_POLL_INTERVAL", 2*time.Second),
		Kubeconfig:    os.Getenv("LAUNCHPAD_KUBECONFIG"),
	}
	return &Target{
		clientFactory: func(contextName string) (kubernetes.Interface, error) {
			return newClientset(opts, contextName)
		},
		opts: opts,
	}, nil
}

func (t *Target) Type() string { return "kubernetes" }

func (t *Target) Deploy(ctx context.Context, req target.DeployRequest) (*target.DeployResult, error) {
	cfg, err := parseTargetConfig(req.App)
	if err != nil {
		return nil, err
	}
	client, err := t.clientFor(cfg.Cluster)
	if err != nil {
		return nil, err
	}

	if err := upsertSecret(ctx, client, buildSecret(req.App, req.Config)); err != nil {
		return nil, fmt.Errorf("apply secret: %w", err)
	}

	processes := processesOrDefault(req.Processes)
	depNames := make([]string, 0, len(processes))
	processState := make(map[string]target.ProcessState)

	for _, process := range processes {
		dep, err := upsertDeployment(ctx, client, buildDeployment(req.App, req.Release, process, req.Config))
		if err != nil {
			return nil, fmt.Errorf("apply deployment %s: %w", process.Name, err)
		}
		depNames = append(depNames, dep.Name)

		if process.Name == "web" {
			if err := upsertService(ctx, client, buildService(req.App, process, req.Config)); err != nil {
				return nil, fmt.Errorf("apply service: %w", err)
			}
		}
	}

	deployments, err := waitForDeployments(ctx, client, cfg.Namespace, depNames, t.opts.DeployTimeout, t.opts.PollInterval)
	if err != nil {
		return nil, err
	}

	var targetRef string
	for _, process := range processes {
		dep := deployments[deploymentName(req.App.Name, process.Name)]
		if dep == nil {
			continue
		}
		processState[process.Name] = processStateFromDeployment(dep)
		if process.Name == "web" || targetRef == "" {
			targetRef = deploymentTargetRef(dep)
		}
	}

	return &target.DeployResult{
		TargetRef:    targetRef,
		ProcessState: processState,
	}, nil
}

func (t *Target) Scale(ctx context.Context, req target.ScaleRequest) error {
	cfg, err := parseTargetConfig(req.App)
	if err != nil {
		return err
	}
	client, err := t.clientFor(cfg.Cluster)
	if err != nil {
		return err
	}

	name := deploymentName(req.App.Name, req.ProcessName)
	dep, err := client.AppsV1().Deployments(cfg.Namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}
	replicas := int32(req.Quantity)
	dep.Spec.Replicas = &replicas
	_, err = client.AppsV1().Deployments(cfg.Namespace).Update(ctx, dep, metav1.UpdateOptions{})
	return err
}

func (t *Target) Destroy(ctx context.Context, req target.DestroyRequest) error {
	cfg, err := parseTargetConfig(req.App)
	if err != nil {
		return err
	}
	client, err := t.clientFor(cfg.Cluster)
	if err != nil {
		return err
	}
	return deleteAppResources(ctx, client, cfg.Namespace, req.App.Name)
}

func (t *Target) Rollback(ctx context.Context, req target.RollbackRequest) (*target.DeployResult, error) {
	return t.Deploy(ctx, target.DeployRequest{
		App:       req.App,
		Release:   req.Release,
		Processes: req.Processes,
		Config:    req.Config,
		ImageRef:  req.Release.ImageRef,
	})
}

func (t *Target) Status(ctx context.Context, req target.StatusRequest) (*target.RuntimeStatus, error) {
	cfg, err := parseTargetConfig(req.App)
	if err != nil {
		return nil, err
	}
	client, err := t.clientFor(cfg.Cluster)
	if err != nil {
		return nil, err
	}
	return collectRuntimeStatus(ctx, client, req.App, req.Processes)
}

func (t *Target) Logs(ctx context.Context, req target.LogsRequest) (io.ReadCloser, error) {
	cfg, err := parseTargetConfig(req.App)
	if err != nil {
		return nil, err
	}
	client, err := t.clientFor(cfg.Cluster)
	if err != nil {
		return nil, err
	}
	process := req.ProcessName
	if process == "" {
		process = "web"
	}
	return streamPodLogs(ctx, client, req.App, process)
}

func (t *Target) clientFor(contextName string) (kubernetes.Interface, error) {
	return t.clientFactory(contextName)
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}