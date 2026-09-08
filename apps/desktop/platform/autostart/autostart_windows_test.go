//go:build windows

package autostart

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestRegistrationUsesQuotedCurrentExecutableAndOnlyItsOwnValue(t *testing.T) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	// This is NOT an OS Run key. No process is scheduled by this integration test.
	path := `Software\ZeitBoard\Tests\` + hex.EncodeToString(nonce[:])
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { key.Close(); _ = registry.DeleteKey(registry.CURRENT_USER, path) })
	if err := key.SetStringValue("OtherSyntheticApp", "leave intact"); err != nil {
		t.Fatal(err)
	}
	c := registryController{path: path, name: "ZeitBoard", executable: `C:\Synthetic Build\ZeitBoard.exe`}
	if initial, err := c.Read(); err != nil || initial.Registered {
		t.Fatalf("fresh registration: %#v %v", initial, err)
	}
	if err := c.Set(true, true); err != nil {
		t.Fatal(err)
	}
	value, _, err := key.GetStringValue("ZeitBoard")
	if err != nil || value != `"C:\Synthetic Build\ZeitBoard.exe" --background` {
		t.Fatalf("unsafe startup command: %q %v", value, err)
	}
	if r, err := c.Read(); err != nil || !r.Registered || !r.MatchesCurrent || !r.StartHidden {
		t.Fatalf("read: %#v %v", r, err)
	}
	if err := c.Set(true, false); err != nil {
		t.Fatal(err)
	}
	if r, err := c.Read(); err != nil || r.StartHidden || !r.MatchesCurrent {
		t.Fatalf("visible launch: %#v %v", r, err)
	}
	moved := c
	moved.executable = `C:\New Location\ZeitBoard.exe`
	if r, err := moved.Read(); err != nil || !r.Registered || r.MatchesCurrent {
		t.Fatalf("moved executable hidden: %#v %v", r, err)
	}
	if err := moved.Set(false, false); err != nil {
		t.Fatal(err)
	}
	if err := moved.Set(false, false); err != nil {
		t.Fatal(err)
	}
	if value, _, err := key.GetStringValue("OtherSyntheticApp"); err != nil || value != "leave intact" {
		t.Fatal("modified another registration")
	}
}

func TestStartupCommandRejectsAmbiguousOrOversizedCommands(t *testing.T) {
	for _, executable := range []string{"relative.exe", `C:\Test\Zeit"Board.exe`, "C:\\Test\\line\n.exe", `C:\` + strings.Repeat("long", 70) + `.exe`, `C:\Test\script.cmd`} {
		if _, err := startupCommand(executable, true); err == nil {
			t.Fatalf("accepted %q", executable)
		}
	}
}
