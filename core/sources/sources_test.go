package sources

import (
	"testing"
	"time"
)

var base = time.Date(2026, 3, 13, 9, 0, 0, 0, time.UTC)

func read(id string, at time.Time, held int) Report {
	return Report{
		ReportID: id, Source: HealthConnect, Outcome: Read, ReportedAt: at, BackgroundReads: true,
		Covered: &Window{StartAt: at.Add(-30 * 24 * time.Hour), EndAt: at}, Held: &held,
	}
}

func outage(id, outcome string, at time.Time) Report {
	return Report{ReportID: id, Source: HealthConnect, Outcome: outcome, ReportedAt: at}
}

func TestAReportStatesAReadAndNothingElse(t *testing.T) {
	if err := read("srcrep-valid-01", base, 0).Validate(); err != nil {
		t.Fatalf("a read was refused: %v", err)
	}
	if err := outage("srcrep-valid-02", PermissionMissing, base).Validate(); err != nil {
		t.Fatalf("an outage was refused: %v", err)
	}
	held := 0
	for name, bad := range map[string]Report{
		"bad id":                {ReportID: "Report 1", Source: HealthConnect, Outcome: Unavailable, ReportedAt: base},
		"unknown source":        {ReportID: "srcrep-bad-01", Source: "fitbit_cloud", Outcome: Unavailable, ReportedAt: base},
		"unknown outcome":       {ReportID: "srcrep-bad-02", Source: HealthConnect, Outcome: "maybe", ReportedAt: base},
		"no time":               {ReportID: "srcrep-bad-03", Source: HealthConnect, Outcome: Unavailable},
		"a read without window": {ReportID: "srcrep-bad-04", Source: HealthConnect, Outcome: Read, ReportedAt: base, Held: &held},
		"an outage with window": {ReportID: "srcrep-bad-05", Source: HealthConnect, Outcome: ReadFailed, ReportedAt: base,
			Covered: &Window{StartAt: base.Add(-time.Hour), EndAt: base}},
		"a window past the report": {ReportID: "srcrep-bad-06", Source: HealthConnect, Outcome: Read, ReportedAt: base,
			Covered: &Window{StartAt: base.Add(-time.Hour), EndAt: base.Add(time.Hour)}, Held: &held},
		"an empty window": {ReportID: "srcrep-bad-07", Source: HealthConnect, Outcome: Read, ReportedAt: base,
			Covered: &Window{StartAt: base, EndAt: base}, Held: &held},
	} {
		t.Run(name, func(t *testing.T) {
			if err := bad.Validate(); err == nil {
				t.Fatal("an invalid report was accepted")
			}
		})
	}
}

func TestAStatusSaysSinceWhenAndThroughWhen(t *testing.T) {
	received := []Received{
		{DeviceID: "phone", Report: read("srcrep-a", base, 0)},
		{DeviceID: "phone", Report: read("srcrep-b", base.Add(6*time.Hour), 2)},
		{DeviceID: "phone", Report: outage("srcrep-c", PermissionMissing, base.Add(7*time.Hour))},
		{DeviceID: "phone", Report: outage("srcrep-d", PermissionMissing, base.Add(13*time.Hour))},
	}
	statuses := Statuses(received)
	if len(statuses) != 1 {
		t.Fatalf("statuses = %d, want one for Health Connect", len(statuses))
	}
	status := statuses[0]
	if status.Outcome != PermissionMissing || !status.ReportedAt.Equal(base.Add(13*time.Hour)) {
		t.Fatalf("newest = %s at %s", status.Outcome, status.ReportedAt)
	}
	// The permission went at the first report that found it gone.
	if !status.Since.Equal(base.Add(7 * time.Hour)) {
		t.Fatalf("since = %s, want the first missing report", status.Since)
	}
	// The newest read still says how far the records reach, and what it held.
	if !status.ReadThrough.Equal(base.Add(6*time.Hour)) || status.Held != 2 {
		t.Fatalf("read through %s holding %d", status.ReadThrough, status.Held)
	}
}

func TestTheDeviceThatReportedLastSpeaks(t *testing.T) {
	statuses := Statuses([]Received{
		{DeviceID: "old-phone", Report: outage("srcrep-old", Unavailable, base)},
		{DeviceID: "new-phone", Report: read("srcrep-new", base.Add(time.Hour), 0)},
	})
	if len(statuses) != 1 || statuses[0].DeviceID != "new-phone" || statuses[0].Outcome != Read {
		t.Fatalf("statuses = %+v", statuses)
	}
	// Never read: nothing to read through.
	if never := Statuses([]Received{{DeviceID: "phone", Report: outage("srcrep-x", Unavailable, base)}}); !never[0].ReadThrough.IsZero() {
		t.Fatalf("an unread source reads through %s", never[0].ReadThrough)
	}
	if Statuses(nil) == nil || len(Statuses(nil)) != 0 {
		t.Fatal("no reports should give an empty list")
	}
}
