package kubernetes

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func newClientset(opts Options, contextName string) (kubernetes.Interface, error) {
	config, err := loadRESTConfig(opts, contextName)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

func loadRESTConfig(opts Options, contextName string) (*rest.Config, error) {
	if kubeconfig := opts.Kubeconfig; kubeconfig != "" {
		return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig},
			&clientcmd.ConfigOverrides{CurrentContext: contextName},
		).ClientConfig()
	}

	if env := os.Getenv("KUBECONFIG"); env != "" {
		return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: env},
			&clientcmd.ConfigOverrides{CurrentContext: contextName},
		).ClientConfig()
	}

	home, err := os.UserHomeDir()
	if err == nil {
		defaultPath := filepath.Join(home, ".kube", "config")
		if _, err := os.Stat(defaultPath); err == nil {
			return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
				&clientcmd.ClientConfigLoadingRules{ExplicitPath: defaultPath},
				&clientcmd.ConfigOverrides{CurrentContext: contextName},
			).ClientConfig()
		}
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("kubernetes config: no kubeconfig found and not running in-cluster: %w", err)
	}
	return config, nil
}