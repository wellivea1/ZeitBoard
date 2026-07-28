//go:build windows

package localagent

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The descriptor carries the bearer token, and ADR-0028 claims it is readable
// only by the owning user. os.Chmod cannot deliver that on Windows, so this
// asserts the real DACL: inheritance disabled, and exactly one allow entry,
// for the current user.
func TestDescriptorDaclIsOwnerOnlyAndProtected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local-agent.json")
	if err := os.WriteFile(path, []byte(`{"token":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictToCurrentUser(path); err != nil {
		t.Fatalf("restrictToCurrentUser: %v", err)
	}

	sd, err := windows.GetNamedSecurityInfo(
		path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		t.Fatalf("read security info: %v", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatalf("read DACL: %v", err)
	}
	if dacl == nil {
		t.Fatal("descriptor has no DACL, so it is world-accessible")
	}

	control, _, err := sd.Control()
	if err != nil {
		t.Fatalf("read control flags: %v", err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("DACL still inherits from the parent directory; a permissive parent would widen token access")
	}

	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	if got := dacl.AceCount; got != 1 {
		t.Fatalf("DACL has %d entries, want exactly 1 (current user only)", got)
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		t.Fatalf("read ACE: %v", err)
	}
	sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !sid.Equals(user.User.Sid) {
		t.Fatalf("sole DACL entry is %s, want the current user %s", sid, user.User.Sid)
	}
}
