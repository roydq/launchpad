package conformance_test

import (
	"testing"

	"github.com/launchpad/launchpad/internal/target/conformance"
	"github.com/launchpad/launchpad/internal/target/stub"
)

func TestStubConformance(t *testing.T) {
	conformance.Run(t, stub.New())
}
