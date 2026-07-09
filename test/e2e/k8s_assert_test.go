//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func deploymentName(project, service, process string) string {
	return fmt.Sprintf("launchpad-%s-%s-%s", project, service, process)
}

func secretName(project, service string) string {
	return fmt.Sprintf("launchpad-%s-%s-config", project, service)
}

func kubeClient(t *testing.T) kubernetes.Interface {
	t.Helper()
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		kubeconfig = filepath.Join(home, ".kube", "config")
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Fatalf("kubeconfig: %v", err)
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("kubernetes client: %v", err)
	}
	return client
}

func waitDeploymentReady(t *testing.T, ctx context.Context, client kubernetes.Interface, namespace, name string, timeout time.Duration) *appsv1.Deployment {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		dep, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			var desired int32 = 1
			if dep.Spec.Replicas != nil {
				desired = *dep.Spec.Replicas
			}
			if dep.Status.ReadyReplicas >= desired && desired > 0 {
				return dep
			}
			t.Logf("deployment %s ready=%d desired=%d", name, dep.Status.ReadyReplicas, desired)
		} else {
			t.Logf("deployment %s: %v", name, err)
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("deployment %s/%s not ready within %s", namespace, name, timeout)
	return nil
}

func TestKindClusterResources(t *testing.T) {
	requireE2E(t)
	if envOr("LAUNCHPAD_E2E_TARGET", "stub") != "kubernetes" {
		t.Skip("kind resource assertions require LAUNCHPAD_E2E_TARGET=kubernetes")
	}

	ctx := context.Background()
	apiURL, bootstrap, target, image, namespace, timeout := e2eConfig(t)
	client := newAuthedClient(t, ctx, apiURL, bootstrap)

	name := uniqueProjectName()
	if _, err := client.CreateProject(ctx, name, target, namespace); err != nil {
		t.Fatalf("create project: %v", err)
	}
	result, err := client.Deploy(ctx, name, image, "kind e2e")
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	pollJobSucceeded(t, ctx, client, result.Job.ID, timeout)

	k8s := kubeClient(t)
	depName := deploymentName(name, name, "web")
	waitDeploymentReady(t, ctx, k8s, namespace, depName, timeout)

	secName := secretName(name, name)
	if _, err := k8s.CoreV1().Secrets(namespace).Get(ctx, secName, metav1.GetOptions{}); err != nil {
		t.Fatalf("expected secret %s: %v", secName, err)
	}
}
