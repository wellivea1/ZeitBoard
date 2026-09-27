package localzone

import (
	"testing"
	"time"
)

func TestOnlyARealZoneIsAName(t *testing.T) {
	if got := valid("America/New_York"); got != "America/New_York" {
		t.Fatalf("a real zone was refused: %q", got)
	}
	for name, id := range map[string]string{
		"Go's placeholder": "Local",
		"nothing":          "",
		"not a zone":       "Mars/Olympus_Mons",
	} {
		if got := valid(id); got != "" {
			t.Errorf("%s: named %q", name, got)
		}
	}
}

// On a real computer the system names a zone that keeps its time.
func TestThisComputerNamesItsZone(t *testing.T) {
	id := ID()
	if id == "" {
		t.Skip("this system names no zone")
	}
	location, err := time.LoadLocation(id)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	_, named := now.In(location).Zone()
	_, clock := now.Zone()
	if named != clock {
		t.Fatalf("%s is %d s from UTC; this computer's clock is %d s", id, named, clock)
	}
	t.Logf("this computer's zone: %s", id)
}
