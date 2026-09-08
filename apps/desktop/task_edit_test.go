package main

import (
	"testing"
	"time"
)

func TestTaskEditPreservesExactConstraintsAcrossDST(t *testing.T) {
	app := newTestApp(t)
	input := TaskInput{Title: "Synthetic appointment preparation", DurationMinutes: 30,
		ZoneID: "America/New_York", EarliestStartAt: "2026-11-01T06:30:12Z",
		LatestFinishAt: "2026-11-01T09:00:00Z", PreferredAfterWakeMinutes: 90, MinimumConfidence: "medium"}
	added, err := app.AddTask(input)
	if err != nil {
		t.Fatal(err)
	}
	dto := added.Tasks[0]
	if dto.EarliestStartAt != input.EarliestStartAt || dto.LatestFinishAt != input.LatestFinishAt || dto.PreferredAfterWakeMinutes != 90 || dto.MinimumConfidence != "medium" {
		t.Fatalf("editing fields not preserved: %+v", dto)
	}
	input.TaskID = dto.TaskID
	input.Revision = dto.Revision
	input.Title = "Synthetic preparation, revised"
	input.ZoneID = "Europe/London"
	changed, err := app.UpdateTask(input)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Tasks[0].EarliestStartAt != dto.EarliestStartAt || changed.Tasks[0].LatestFinishAt != dto.LatestFinishAt || changed.Tasks[0].Revision <= dto.Revision {
		t.Fatalf("edit moved constraints or did not revise task: %+v", changed)
	}
	if _, err := app.UpdateTask(input); err == nil {
		t.Fatal("stale revision accepted")
	}
}

func TestTaskConstraintInputRejectsAmbiguousRepresentations(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range [][2]string{
		{"2026-09-08T12:00", "2026-09-08T16:00:00Z"},
		{"", "2026-09-08T12:00:00"},
		{"2026-03-08T02:30", ""},
	} {
		if _, err := parseTaskConstraintTime(input[0], input[1], location); err == nil {
			t.Fatalf("invalid input accepted: %v", input)
		}
	}
}
