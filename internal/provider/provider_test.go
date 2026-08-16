package provider_test

import (
	"testing"

	"github.com/pleumcloud/pleumcloud/internal/provider"

	// Connectors self-register into the provider registry.
	_ "github.com/pleumcloud/pleumcloud/internal/provider/gdrive"
	_ "github.com/pleumcloud/pleumcloud/internal/provider/mybox"
)

// A nil secret store must fail the build instead of producing a connector
// that panics on first credential lookup (the M5 syncLoop crash).
func TestBuildRejectsNilSecrets(t *testing.T) {
	for _, id := range []string{"gdrive", "mybox"} {
		if _, ok := provider.Build(id, provider.Deps{}); ok {
			t.Fatalf("Build(%q, Deps{}) must report not-ok", id)
		}
	}
}
