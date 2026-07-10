//go:build e2e

// Package e2e holds end-to-end tests against a running Launchpad API and worker.
//
// Run via scripts/e2e-stub.sh or scripts/e2e-kind.sh (sets LAUNCHPAD_E2E=1).
// Default "go test ./..." excludes this package because of the e2e build tag.
package e2e
