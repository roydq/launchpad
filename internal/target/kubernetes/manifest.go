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

func buildSecret(project domain.Project, service domain.Service, env domain.Environment, config map[string]string) *corev1.Secret {
	data := make(map[string][]byte, len(config))
	for k, v := range config {
		data[k] = []byte(v)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName(project.Name, service.Name),
			Namespace: mustNamespace(env),
			Labels:    resourceLabels(project.Name, service.Name, "config"),
			Annotations: map[string]string{
				annotationProjectName: project.Name,
				annotationServiceName: service.Name,
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}
}

func buildDeployment(project domain.Project, service domain.Service, env domain.Environment, release domain.Release, process domain.Process, config map[string]string) *appsv1.Deployment {
	replicas := int32(process.Quantity)
	if replicas < 1 {
		replicas = 1
	}
	port := containerPort(config)
	labels := resourceLabels(project.Name, service.Name, process.Name)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName(project.Name, service.Name, process.Name),
			Namespace: mustNamespace(env),
			Labels:    labels,
			Annotations: map[string]string{
				annotationProjectName:    project.Name,
				annotationServiceName:    service.Name,
				annotationReleaseVersion: strconv.Itoa(release.Version),
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
						containerFromProcess(process, release.ArtifactRef, port, project.Name, service.Name),
					},
				},
			},
		},
	}
}

func buildService(project domain.Project, service domain.Service, env domain.Environment, process domain.Process, config map[string]string) *corev1.Service {
	port := containerPort(config)
	labels := resourceLabels(project.Name, service.Name, process.Name)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName(project.Name, service.Name, process.Name),
			Namespace: mustNamespace(env),
			Labels:    labels,
			Annotations: map[string]string{
				annotationProjectName: project.Name,
				annotationServiceName: service.Name,
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

func containerFromProcess(process domain.Process, image string, port int32, projectName, serviceName string) corev1.Container {
	c := corev1.Container{
		Name:  process.Name,
		Image: image,
		Ports: []corev1.ContainerPort{
			{Name: "http", ContainerPort: port, Protocol: corev1.ProtocolTCP},
		},
		EnvFrom: []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretName(projectName, serviceName),
					},
				},
			},
		},
	}
	// Shell form: multi-word Procfile-style commands need /bin/sh (portable DX).
	if process.Command != "" {
		c.Command = []string{"/bin/sh", "-c", process.Command}
	}
	if probe := readinessProbe(process, port); probe != nil {
		c.ReadinessProbe = probe
	}
	return c
}

func readinessProbe(process domain.Process, defaultPort int32) *corev1.Probe {
	h := process.Health
	if h == nil || h.Type == "" || h.Type == "none" {
		return nil
	}
	period := int32(h.PeriodSeconds)
	if period <= 0 {
		period = 10
	}
	timeout := int32(h.TimeoutSeconds)
	if timeout <= 0 {
		timeout = 2
	}
	initial := int32(h.InitialDelaySeconds)
	if initial < 0 {
		initial = 0
	}
	if initial == 0 && h.InitialDelaySeconds == 0 {
		initial = 5
	}
	failure := int32(h.FailureThreshold)
	if failure <= 0 {
		failure = 3
	}
	success := int32(h.SuccessThreshold)
	if success <= 0 {
		success = 1
	}
	probe := &corev1.Probe{
		InitialDelaySeconds: initial,
		PeriodSeconds:       period,
		TimeoutSeconds:      timeout,
		FailureThreshold:    failure,
		SuccessThreshold:    success,
	}
	port := defaultPort
	if h.Port != nil && *h.Port > 0 {
		port = int32(*h.Port)
	}
	switch h.Type {
	case "http":
		path := h.Path
		if path == "" {
			path = "/healthz"
		}
		probe.ProbeHandler = corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: path,
				Port: intstr.FromInt32(port),
			},
		}
	case "tcp":
		probe.ProbeHandler = corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.FromInt32(port),
			},
		}
	case "exec":
		// Exec uses command string as shell form when present.
		cmd := process.Command
		if cmd == "" {
			return nil
		}
		probe.ProbeHandler = corev1.ProbeHandler{
			Exec: &corev1.ExecAction{Command: []string{"/bin/sh", "-c", cmd}},
		}
	default:
		return nil
	}
	return probe
}

func mustNamespace(env domain.Environment) string {
	cfg, err := parseTargetConfig(env)
	if err != nil {
		return "default"
	}
	return cfg.Namespace
}

func processesOrDefault(processes []domain.Process) []domain.Process {
	if len(processes) == 0 {
		return []domain.Process{{Name: "web", Quantity: 1, Expose: "http"}}
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