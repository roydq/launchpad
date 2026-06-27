package kubernetes

import (
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/target"
)

func buildSecret(app domain.App, config map[string]string) *corev1.Secret {
	data := make(map[string][]byte, len(config))
	for k, v := range config {
		data[k] = []byte(v)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName(app.Name),
			Namespace: mustNamespace(app),
			Labels:    appLabels(app.Name, "config"),
			Annotations: map[string]string{
				annotationAppName: app.Name,
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}
}

func buildDeployment(app domain.App, release domain.Release, process domain.ProcessType, config map[string]string) *appsv1.Deployment {
	replicas := int32(process.Quantity)
	if replicas < 1 {
		replicas = 1
	}
	port := containerPort(config)
	labels := appLabels(app.Name, process.Name)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName(app.Name, process.Name),
			Namespace: mustNamespace(app),
			Labels:    labels,
			Annotations: map[string]string{
				annotationAppName:          app.Name,
				annotationReleaseVersion:   strconv.Itoa(release.Version),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
					Annotations: map[string]string{
						annotationReleaseVersion: strconv.Itoa(release.Version),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  process.Name,
							Image: release.ImageRef,
							Ports: []corev1.ContainerPort{
								{Name: "http", ContainerPort: port, Protocol: corev1.ProtocolTCP},
							},
							EnvFrom: []corev1.EnvFromSource{
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: secretName(app.Name),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func buildService(app domain.App, process domain.ProcessType, config map[string]string) *corev1.Service {
	port := containerPort(config)
	labels := appLabels(app.Name, process.Name)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName(app.Name, process.Name),
			Namespace: mustNamespace(app),
			Labels:    labels,
			Annotations: map[string]string{
				annotationAppName: app.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       port,
					TargetPort: intstr.FromString("http"),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

func mustNamespace(app domain.App) string {
	cfg, err := parseTargetConfig(app)
	if err != nil {
		return "default"
	}
	return cfg.Namespace
}

func processesOrDefault(processes []domain.ProcessType) []domain.ProcessType {
	if len(processes) == 0 {
		return []domain.ProcessType{{Name: "web", Quantity: 1}}
	}
	return processes
}

func processStateFromDeployment(dep *appsv1.Deployment) target.ProcessState {
	desired := int32(0)
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}
	return target.ProcessState{
		Desired: int(desired),
		Ready:   int(dep.Status.ReadyReplicas),
	}
}