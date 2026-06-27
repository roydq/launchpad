package domain

import "testing"

func TestDeploymentTransitions(t *testing.T) {
	cases := []struct {
		from, to DeploymentStatus
		ok       bool
	}{
		{DeploymentPending, DeploymentDeploying, true},
		{DeploymentDeploying, DeploymentRunning, true},
		{DeploymentDeploying, DeploymentFailed, true},
		{DeploymentRunning, DeploymentSuperseded, true},
		{DeploymentPending, DeploymentRunning, false},
	}
	for _, tc := range cases {
		got := CanTransitionDeployment(tc.from, tc.to)
		if got != tc.ok {
			t.Fatalf("%s -> %s: got %v want %v", tc.from, tc.to, got, tc.ok)
		}
	}
}