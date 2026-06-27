package kubernetes

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/target"
)

func testApp() domain.App {
	return domain.App{
		ID:           uuid.New(),
		Name:         "my-api",
		TargetType:   "kubernetes",
		TargetConfig: json.RawMessage(`{"namespace":"launchpad-test","cluster":"default"}`),
	}
}

func markDeploymentReady(dep *appsv1.Deployment) {
	replicas := int32(1)
	if dep.Spec.Replicas != nil {
		replicas = *dep.Spec.Replicas
	}
	dep.Status.ReadyReplicas = replicas
	dep.Status.AvailableReplicas = replicas
	dep.Status.Conditions = []appsv1.DeploymentCondition{{
		Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue,
	}}
}

func TestApplyAndWaitForReady(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	app := testApp()
	release := domain.Release{Version: 3, ImageRef: "nginx:1.25"}
	config := map[string]string{"PORT": "3000"}

	if err := upsertSecret(ctx, client, buildSecret(app, config)); err != nil {
		t.Fatal(err)
	}
	dep, err := upsertDeployment(ctx, client, buildDeployment(app, release, domain.ProcessType{Name: "web", Quantity: 1}, config))
	if err != nil {
		t.Fatal(err)
	}
	if err := upsertService(ctx, client, buildService(app, domain.ProcessType{Name: "web"}, config)); err != nil {
		t.Fatal(err)
	}

	markDeploymentReady(dep)
	_, err = client.AppsV1().Deployments(dep.Namespace).UpdateStatus(ctx, dep, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	deployments, err := waitForDeployments(ctx, client, "launchpad-test", []string{dep.Name}, 2*time.Second, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if !deploymentReady(deployments[dep.Name]) {
		t.Fatal("expected deployment to be ready")
	}
}

func TestDeployAppliesManifests(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()

	// Seed a ready deployment before Deploy runs wait loop by using a custom reactor is complex;
	// instead verify apply path by checking resources exist after a failed-wait deploy is not ideal.
	// Test apply functions via secret/deployment/service getters after manual apply.
	app := testApp()
	release := domain.Release{Version: 1, ImageRef: "nginx:1.25"}
	config := map[string]string{"PORT": "8080"}

	targetBackend := New(client, Options{DeployTimeout: time.Millisecond, PollInterval: time.Millisecond})

	_, err := targetBackend.Deploy(ctx, target.DeployRequest{
		App: app, Release: release,
		Processes: []domain.ProcessType{{Name: "web", Quantity: 1}},
		Config:    config,
	})
	if err == nil {
		t.Fatal("expected timeout error for unready deployment")
	}

	secret, err := client.CoreV1().Secrets("launchpad-test").Get(ctx, secretName("my-api"), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if string(secret.Data["PORT"]) != "8080" {
		t.Fatalf("unexpected secret data: %v", secret.Data)
	}

	dep, err := client.AppsV1().Deployments("launchpad-test").Get(ctx, deploymentName("my-api", "web"), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if dep.Spec.Template.Spec.Containers[0].Image != "nginx:1.25" {
		t.Fatalf("unexpected image: %s", dep.Spec.Template.Spec.Containers[0].Image)
	}
	if dep.Annotations[annotationReleaseVersion] != "1" {
		t.Fatalf("missing release annotation: %v", dep.Annotations)
	}
}

func TestDeploymentReady(t *testing.T) {
	replicas := int32(2)
	dep := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: 2, AvailableReplicas: 2,
			Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue}},
		},
	}
	if !deploymentReady(dep) {
		t.Fatal("expected deployment ready")
	}
}

func TestParseTargetConfigRequiresNamespace(t *testing.T) {
	_, err := parseTargetConfig(domain.App{TargetConfig: json.RawMessage(`{}`)})
	if err == nil {
		t.Fatal("expected error for missing namespace")
	}
}

func TestScaleUpdatesReplicas(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	app := testApp()
	release := domain.Release{Version: 1, ImageRef: "nginx:1.25"}
	dep := buildDeployment(app, release, domain.ProcessType{Name: "web", Quantity: 1}, nil)
	_, err := client.AppsV1().Deployments(dep.Namespace).Create(ctx, dep, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	targetBackend := New(client, Options{})
	if err := targetBackend.Scale(ctx, target.ScaleRequest{App: app, ProcessName: "web", Quantity: 3}); err != nil {
		t.Fatal(err)
	}

	updated, err := client.AppsV1().Deployments("launchpad-test").Get(ctx, deploymentName("my-api", "web"), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if *updated.Spec.Replicas != 3 {
		t.Fatalf("expected 3 replicas, got %d", *updated.Spec.Replicas)
	}
}