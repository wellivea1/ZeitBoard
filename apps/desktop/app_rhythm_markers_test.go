package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	storage "non24.app/core/storage/sqlite"
)

func TestRhythmMarkersRoundTripExportAndHardEraseWithoutChangingEstimate(t *testing.T) {
	app := newTestApp(t)
	fixedNow := time.Date(2026, 7, 22, 16, 0, 0, 0, time.UTC)
	app.nowFn = func() time.Time { return fixedNow }

	initial, err := app.GetRhythmMarkers()
	if err != nil {
		t.Fatal(err)
	}
	if initial.Status != "empty" || !initial.Empty || initial.FixtureMode || len(initial.Markers) != 0 {
		t.Fatalf("initial markers = %#v", initial)
	}
	rhythmBefore, err := app.GetRhythm()
	if err != nil {
		t.Fatal(err)
	}

	created, err := app.AddRhythmMarker(RhythmMarkerInput{
		Kind:       storage.RhythmMarkerTravel,
		StartLocal: "2026-07-22T09:00",
		EndLocal:   "2026-07-22T11:00",
		ZoneID:     "America/New_York",
		Note:       " Private time-zone context ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != "ready" || created.Empty || len(created.Markers) != 1 {
		t.Fatalf("created markers = %#v", created)
	}
	marker := created.Markers[0]
	if marker.KindLabel != "Travel / time-zone context" || marker.Note != "Private time-zone context" || marker.CivilDate != "2026-07-22" || marker.Hour != 9 {
		t.Fatalf("marker DTO = %#v", marker)
	}
	if !strings.Contains(created.Message, "do not change") || !strings.Contains(created.Message, "establish cause") {
		t.Fatalf("marker boundary copy = %q", created.Message)
	}
	rhythmAfter, err := app.GetRhythm()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rhythmBefore, rhythmAfter) {
		t.Fatal("adding context changed the estimator projection")
	}

	exported, err := app.ExportRhythmMarkers()
	if err != nil {
		t.Fatal(err)
	}
	if exported.MarkerCount != 1 || !strings.Contains(exported.FileName, "rhythm-context-v1") {
		t.Fatalf("marker export = %#v", exported)
	}
	var payload storage.RhythmMarkerSet
	if err := json.Unmarshal([]byte(exported.JSON), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SchemaVersion != "v1" || len(payload.Markers) != 1 || payload.Markers[0].Note != marker.Note {
		t.Fatalf("marker export payload = %#v", payload)
	}

	if _, err := app.DeleteRhythmMarker(RhythmMarkerDeleteInput{MarkerID: marker.MarkerID, Confirmation: "delete"}); err == nil {
		t.Fatal("marker erasure accepted the wrong confirmation")
	}
	erased, err := app.DeleteRhythmMarker(RhythmMarkerDeleteInput{MarkerID: marker.MarkerID, Confirmation: "DELETE"})
	if err != nil {
		t.Fatal(err)
	}
	if erased.Status != "empty" || !erased.Empty || len(erased.Markers) != 0 {
		t.Fatalf("erased markers = %#v", erased)
	}
}

func TestRhythmMarkerInputRejectsFutureAndInvalidCivilTimes(t *testing.T) {
	app := newTestApp(t)
	app.nowFn = func() time.Time { return time.Date(2026, 3, 9, 16, 0, 0, 0, time.UTC) }

	tests := []struct {
		name  string
		input RhythmMarkerInput
	}{
		{
			name: "spring DST gap",
			input: RhythmMarkerInput{
				Kind:       storage.RhythmMarkerDisruption,
				StartLocal: "2026-03-08T02:30",
				ZoneID:     "America/New_York",
			},
		},
		{
			name: "future start",
			input: RhythmMarkerInput{
				Kind:       storage.RhythmMarkerTravel,
				StartLocal: "2026-03-10T12:00",
				ZoneID:     "America/New_York",
			},
		},
		{
			name: "future end",
			input: RhythmMarkerInput{
				Kind:       storage.RhythmMarkerIllness,
				StartLocal: "2026-03-09T08:00",
				EndLocal:   "2026-03-10T08:00",
				ZoneID:     "America/New_York",
			},
		},
		{
			name: "machine-local pseudo-zone",
			input: RhythmMarkerInput{
				Kind:       storage.RhythmMarkerForcedSchedule,
				StartLocal: "2026-03-09T08:00",
				ZoneID:     "Local",
			},
		},
		{
			name: "unsupported medical category",
			input: RhythmMarkerInput{
				Kind:       "diagnosis",
				StartLocal: "2026-03-09T08:00",
				ZoneID:     "America/New_York",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := app.AddRhythmMarker(test.input); err == nil {
				t.Fatalf("invalid marker input was accepted: %#v", test.input)
			}
		})
	}
}

func TestResolveRhythmMarkerCivilChoosesFirstRepeatedOccurrence(t *testing.T) {
	location := locationOrUTC("America/New_York")
	resolved, err := resolveRhythmMarkerCivil("2026-11-01T01:30", location)
	if err != nil {
		t.Fatal(err)
	}
	first := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)
	if !resolved.Equal(first) {
		t.Fatalf("repeated time resolved to %s, want first occurrence %s", resolved, first)
	}
}
