package presence

import (
	"testing"
	"time"
)

var noon = time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

func report(id, status string, reportedAt time.Time, lifetime time.Duration) Report {
	value := Report{StatusID: id, Status: status, ReportedAt: reportedAt, Shared: status != Cleared}
	if status != Cleared {
		expires := reportedAt.Add(lifetime)
		value.ExpiresAt = &expires
	}
	return value
}

func TestAReportNamesAStatusAndWhenItStops(t *testing.T) {
	valid := report("status_one", TryingToSleep, noon, 8*time.Hour)
	if err := valid.Validate(); err != nil {
		t.Fatalf("a plain report was refused: %v", err)
	}
	cases := map[string]func(*Report){
		"an unknown status":       func(r *Report) { r.Status = "asleep" },
		"no expiry":               func(r *Report) { r.ExpiresAt = nil },
		"an expiry in the past":   func(r *Report) { at := r.ReportedAt; r.ExpiresAt = &at },
		"longer than a day":       func(r *Report) { at := r.ReportedAt.Add(25 * time.Hour); r.ExpiresAt = &at },
		"no time of report":       func(r *Report) { r.ReportedAt = time.Time{} },
		"an id out of the format": func(r *Report) { r.StatusID = "X" },
	}
	for name, spoil := range cases {
		spoiled := valid
		spoil(&spoiled)
		if spoiled.Validate() == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	cleared := report("status_two", Cleared, noon, 0)
	if err := cleared.Validate(); err != nil {
		t.Fatalf("a cleared status was refused: %v", err)
	}
	cleared.Shared = true
	if cleared.Validate() == nil {
		t.Error("a cleared status that shares something was accepted")
	}
}

func TestTheNewestReportStandsUntilItLapsesOrIsTakenBack(t *testing.T) {
	awake := report("status_a", Awake, noon.Add(-3*time.Hour), 4*time.Hour)
	sleeping := report("status_b", TryingToSleep, noon.Add(-time.Hour), 10*time.Hour)

	if current, ok := Current([]Report{sleeping, awake}, noon); !ok || current.StatusID != "status_b" {
		t.Fatalf("current = %+v, %v; want the newer report whatever the order given", current, ok)
	}
	// An older report does not come back when the newer one lapses.
	if current, ok := Current([]Report{awake, sleeping}, noon.Add(10*time.Hour)); ok {
		t.Fatalf("after the newest lapsed, %+v was still current", current)
	}
	// Taking it back leaves nothing, and does not uncover the one before.
	cleared := report("status_c", Cleared, noon, 0)
	if current, ok := Current([]Report{awake, sleeping, cleared}, noon.Add(time.Minute)); ok {
		t.Fatalf("after clearing, %+v was still current", current)
	}
	// A report at the instant it expires no longer stands.
	if _, ok := Current([]Report{awake}, noon.Add(time.Hour)); ok {
		t.Fatal("a report stood at its own expiry")
	}
	// A clock that runs ahead does not make a report stand early.
	ahead := report("status_d", DoNotDisturb, noon.Add(time.Hour), 2*time.Hour)
	if _, ok := Current([]Report{ahead}, noon); ok {
		t.Fatal("a report dated in the future was believed")
	}
	if _, ok := Current(nil, noon); ok {
		t.Fatal("no reports gave a current status")
	}
}

func TestEveryStatusHasALifetimeWithinTheLimit(t *testing.T) {
	for _, status := range []string{Awake, TryingToSleep, DoNotDisturb} {
		if lifetime := DefaultLifetime(status); lifetime <= 0 || lifetime > MaxLifetime {
			t.Errorf("%s stands for %s by default", status, lifetime)
		}
	}
}
