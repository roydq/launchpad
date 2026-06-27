package kubernetes

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/target"
)

func waitForDeployments(ctx context.Context, client kubernetes.Interface, namespace string, names []string, timeout, interval time.Duration) (map[string]*appsv1.Deployment, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for {
		ready := true
		deployments := make(map[string]*appsv1.Deployment, len(names))
		for _, name := range names {
			dep, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				lastErr = err
				ready = false
				continue
			}
			deployments[name] = dep
			if !deploymentReady(dep) {
				ready = false
				lastErr = fmt.Errorf("deployment %s not ready (%d/%d replicas)", name, dep.Status.ReadyReplicas, valueOrZero(dep.Spec.Replicas))
			}
		}
		if ready {
			return deployments, nil
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return nil, fmt.Errorf("deploy timeout: %w", lastErr)
			}
			return nil, fmt.Errorf("deploy timeout waiting for deployments")
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}

func valueOrZero(v *int32) int32 {
	if v == nil {
		return 0
	}
	return *v
}

func collectRuntimeStatus(ctx context.Context, client kubernetes.Interface, app domain.App, processes []domain.ProcessType) (*target.RuntimeStatus, error) {
	cfg, err := parseTargetConfig(app)
	if err != nil {
		return nil, err
	}

	state := make(map[string]target.ProcessState)
	ready := true
	var messages []string

	for _, process := range processesOrDefault(processes) {
		name := deploymentName(app.Name, process.Name)
		dep, err := client.AppsV1().Deployments(cfg.Namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			ready = false
			messages = append(messages, fmt.Sprintf("%s: %v", process.Name, err))
			state[process.Name] = target.ProcessState{Desired: process.Quantity}
			continue
		}
		ps := processStateFromDeployment(dep)
		state[process.Name] = ps
		if !deploymentReady(dep) {
			ready = false
			messages = append(messages, fmt.Sprintf("%s: %d/%d ready", process.Name, ps.Ready, ps.Desired))
		}
	}

	msg := "all processes ready"
	if !ready && len(messages) > 0 {
		msg = messages[0]
		for i := 1; i < len(messages); i++ {
			msg += "; " + messages[i]
		}
	}

	return &target.RuntimeStatus{
		Ready:        ready,
		ProcessState: state,
		Message:      msg,
	}, nil
}