package domain

import "fmt"

var deploymentTransitions = map[DeploymentStatus][]DeploymentStatus{
	DeploymentPending:   {DeploymentDeploying, DeploymentCancelled},
	DeploymentDeploying: {DeploymentRunning, DeploymentFailed, DeploymentCancelled, DeploymentSuperseded},
	DeploymentRunning:   {DeploymentSuperseded},
	DeploymentFailed:    {},
	DeploymentCancelled: {},
	DeploymentSuperseded: {},
}

func CanTransitionDeployment(from, to DeploymentStatus) bool {
	allowed, ok := deploymentTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

func ValidateDeploymentTransition(from, to DeploymentStatus) error {
	if CanTransitionDeployment(from, to) {
		return nil
	}
	return fmt.Errorf("invalid deployment transition: %s -> %s", from, to)
}