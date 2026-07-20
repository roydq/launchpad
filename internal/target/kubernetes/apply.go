package kubernetes

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// createSecretImmutable creates a config Secret if missing; never mutates existing (content-hash names).
func createSecretImmutable(ctx context.Context, client kubernetes.Interface, secret *corev1.Secret) error {
	_, err := client.CoreV1().Secrets(secret.Namespace).Get(ctx, secret.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = client.CoreV1().Secrets(secret.Namespace).Create(ctx, secret, metav1.CreateOptions{})
		return err
	}
	return err
}

// upsertSecret is retained for tests of non-immutable objects; config uses createSecretImmutable.
func upsertSecret(ctx context.Context, client kubernetes.Interface, secret *corev1.Secret) error {
	return createSecretImmutable(ctx, client, secret)
}

func upsertDeployment(ctx context.Context, client kubernetes.Interface, dep *appsv1.Deployment) (*appsv1.Deployment, error) {
	existing, err := client.AppsV1().Deployments(dep.Namespace).Get(ctx, dep.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return client.AppsV1().Deployments(dep.Namespace).Create(ctx, dep, metav1.CreateOptions{})
	}
	if err != nil {
		return nil, err
	}
	dep.ResourceVersion = existing.ResourceVersion
	return client.AppsV1().Deployments(dep.Namespace).Update(ctx, dep, metav1.UpdateOptions{})
}

func upsertService(ctx context.Context, client kubernetes.Interface, svc *corev1.Service) error {
	existing, err := client.CoreV1().Services(svc.Namespace).Get(ctx, svc.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = client.CoreV1().Services(svc.Namespace).Create(ctx, svc, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	svc.ResourceVersion = existing.ResourceVersion
	svc.Spec.ClusterIP = existing.Spec.ClusterIP
	svc.Spec.ClusterIPs = existing.Spec.ClusterIPs
	_, err = client.CoreV1().Services(svc.Namespace).Update(ctx, svc, metav1.UpdateOptions{})
	return err
}

func deleteServiceResources(ctx context.Context, client kubernetes.Interface, namespace, projectName, serviceName string) error {
	listOpts := metav1.ListOptions{
		LabelSelector: labelProject + "=" + projectName + "," + labelService + "=" + serviceName,
	}

	depList, err := client.AppsV1().Deployments(namespace).List(ctx, listOpts)
	if err != nil {
		return err
	}
	for _, dep := range depList.Items {
		if err := client.AppsV1().Deployments(namespace).Delete(ctx, dep.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	svcList, err := client.CoreV1().Services(namespace).List(ctx, listOpts)
	if err != nil {
		return err
	}
	for _, svc := range svcList.Items {
		if err := client.CoreV1().Services(namespace).Delete(ctx, svc.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	// Delete all Launchpad-managed config Secrets for this service (stable + hashed).
	secList, err := client.CoreV1().Secrets(namespace).List(ctx, listOpts)
	if err != nil {
		return err
	}
	for _, sec := range secList.Items {
		if err := client.CoreV1().Secrets(namespace).Delete(ctx, sec.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	// Also remove legacy stable name if unlabeled.
	legacy := secretName(projectName, serviceName)
	if err := client.CoreV1().Secrets(namespace).Delete(ctx, legacy, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	return nil
}

func deploymentReady(dep *appsv1.Deployment) bool {
	if dep.Spec.Replicas == nil {
		return false
	}
	desired := *dep.Spec.Replicas
	if dep.Status.ReadyReplicas < desired {
		return false
	}
	for _, cond := range dep.Status.Conditions {
		if cond.Type == appsv1.DeploymentAvailable && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return dep.Status.ReadyReplicas >= desired && dep.Status.AvailableReplicas >= desired
}

func deploymentTargetRef(dep *appsv1.Deployment) string {
	return fmt.Sprintf("%s/%s:%d", dep.Namespace, dep.Name, dep.Status.ObservedGeneration)
}