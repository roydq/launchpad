package kubernetes

import (
	"context"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/launchpad/launchpad/internal/domain"
)

func streamPodLogs(ctx context.Context, client kubernetes.Interface, project domain.Project, service domain.Service, env domain.Environment, processName string) (io.ReadCloser, error) {
	cfg, err := parseTargetConfig(env)
	if err != nil {
		return nil, err
	}

	labels := resourceLabels(project.Name, service.Name, processName)
	pods, err := client.CoreV1().Pods(cfg.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(&metav1.LabelSelector{MatchLabels: labels}),
	})
	if err != nil {
		return nil, err
	}
	if len(pods.Items) == 0 {
		return nil, fmt.Errorf("no pods found for process %s", processName)
	}

	pod := pods.Items[0]
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodRunning {
			pod = p
			break
		}
	}

	req := client.CoreV1().Pods(cfg.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Container: processName,
		Follow:    false,
		TailLines: int64Ptr(200),
	})
	return req.Stream(ctx)
}

func int64Ptr(v int64) *int64 { return &v }