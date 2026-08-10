package localagent

import (
	"os"
	"path/filepath"
	"testing"

	"non24.app/core/platform/privatefile"
)

// The descriptor carries the bearer token, and ADR-0028 claims it is readable
// only by the owning user. The mechanism moved to core/platform/privatefile;
// the guarantee is still this package's to keep, so it is asserted here by
// reading the permission back rather than by trusting the call returned nil.
func TestDescriptorIsRestrictedToTheOwningUser(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local-agent.json")
	if err := os.WriteFile(path, []byte(`{"token":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictToCurrentUser(path); err != nil {
		t.Fatalf("restrictToCurrentUser: %v", err)
	}

	access, err := privatefile.Describe(path)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if !access.OwnerOnly {
		t.Errorf("the descriptor is reachable by another account: %s", access.Detail)
	}
	if access.Inherited {
		t.Errorf("a permissive parent directory can still widen the descriptor: %s", access.Detail)
	}
}
