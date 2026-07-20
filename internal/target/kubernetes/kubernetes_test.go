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

func testProject() domain.Project {
	return domain.Project{
		ID:             uuid.New(),
		Name:           "my-api",
		PrimaryService: "my-api",
	}
}

func testService(project domain.Project) domain.Service {
	return domain.Service{
		ID:        uuid.New(),
		ProjectID: project.ID,
		Name:      project.PrimaryService,
	}
}

func testEnvironment() domain.Environment {
	return domain.Environment{
		ID:           uuid.New(),
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
	project := testProject()
	svc := testService(project)
	env := testEnvironment()
	release := domain.Release{Version: 3, ArtifactRef: "nginx:1.25"}
	config := map[string]string{"PORT": "3000"}

	if err := upsertSecret(ctx, client, buildSecret(project, svc, env, config, "")); err != nil {
		t.Fatal(err)
	}
	dep, err := upsertDeployment(ctx, client, buildDeployment(project, svc, env, release, domain.Process{Name: "web", Quantity: 1, Expose: "http"}, config, ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := upsertService(ctx, client, buildService(project, svc, env, domain.Process{Name: "web", Expose: "http"}, config)); err != nil {
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

	project := testProject()
	svc := testService(project)
	env := testEnvironment()
	release := domain.Release{Version: 1, ArtifactRef: "nginx:1.25"}
	config := map[string]string{"PORT": "8080"}

	targetBackend := New(client, Options{DeployTimeout: time.Millisecond, PollInterval: time.Millisecond})

	_, err := targetBackend.Deploy(ctx, target.DeployRequest{
		Project:     project,
		Service:     svc,
		Environment: env,
		Release:     release,
		Processes:   []domain.Process{{Name: "web", Quantity: 1, Expose: "http"}},
		Config:      config,
	})
	if err == nil {
		t.Fatal("expected timeout error for unready deployment")
	}

	hash := configContentHash(config)
	secret, err := client.CoreV1().Secrets("launchpad-test").Get(ctx, configSecretName("my-api", "my-api", hash), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if string(secret.Data["PORT"]) != "8080" {
		t.Fatalf("unexpected secret data: %v", secret.Data)
	}

	dep, err := client.AppsV1().Deployments("launchpad-test").Get(ctx, deploymentName("my-api", "my-api", "web"), metav1.GetOptions{})
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
	_, err := parseTargetConfig(domain.Environment{TargetConfig: json.RawMessage(`{}`)})
	if err == nil {
		t.Fatal("expected error for missing namespace")
	}
}

func TestScaleUpdatesReplicas(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	project := testProject()
	svc := testService(project)
	env := testEnvironment()
	release := domain.Release{Version: 1, ArtifactRef: "nginx:1.25"}
	dep := buildDeployment(project, svc, env, release, domain.Process{Name: "web", Quantity: 1}, nil, "")
	_, err := client.AppsV1().Deployments(dep.Namespace).Create(ctx, dep, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	targetBackend := New(client, Options{})
	if err := targetBackend.Scale(ctx, target.ScaleRequest{
		Project: project, Service: svc, Environment: env, ProcessName: "web", Quantity: 3,
	}); err != nil {
		t.Fatal(err)
	}

	updated, err := client.AppsV1().Deployments("launchpad-test").Get(ctx, deploymentName("my-api", "my-api", "web"), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if *updated.Spec.Replicas != 3 {
		t.Fatalf("expected 3 replicas, got %d", *updated.Spec.Replicas)
	}
}