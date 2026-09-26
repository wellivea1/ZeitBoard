package portal

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func snapshotAt(generatedAt time.Time, windows ...Window) Snapshot {
	return Snapshot{
		Version:     generatedAt.UnixMilli(),
		GeneratedAt: generatedAt,
		Status:      StatusAvailable,
		Windows:     windows,
	}
}

// TestFreshnessLadder walks the three age bands in design section 1: fresh,
// stale after six hours, and withheld entirely after a day.
func TestFreshnessLadder(t *testing.T) {
	now := time.Date(2026, 8, 3, 15, 0, 0, 0, time.UTC)
	window := Window{StartAt: now.Add(-time.Hour), EndAt: now.Add(4 * time.Hour), ZoneID: "UTC"}

	fresh := BuildView(snapshotAt(now.Add(-30*time.Minute), window), now)
	if fresh.Stale || fresh.Unavailable {
		t.Errorf("30-minute-old snapshot marked stale=%v unavailable=%v", fresh.Stale, fresh.Unavailable)
	}
	if !fresh.LikelyAwake {
		t.Error("a window containing now did not produce a likely-awake state")
	}

	stale := BuildView(snapshotAt(now.Add(-7*time.Hour), window), now)
	if !stale.Stale {
		t.Error("7-hour-old snapshot was not marked stale")
	}
	if stale.Unavailable {
		t.Error("7-hour-old snapshot was withheld; it should show with a caution")
	}
	if !strings.Contains(stale.Detail, "not refreshed recently") {
		t.Errorf("stale detail does not warn the reader: %q", stale.Detail)
	}

	old := BuildView(snapshotAt(now.Add(-25*time.Hour), window), now)
	if !old.Unavailable {
		t.Error("25-hour-old snapshot was still presented as current")
	}
	if old.LikelyAwake {
		t.Error("a day-old snapshot produced an 'awake now' claim")
	}
	if len(old.Windows) != 0 {
		t.Error("a day-old snapshot still listed windows")
	}
}

// TestEveryViewCarriesTheQualifier enforces the honesty budget: no rendered
// state may omit the measured-uncertainty line.
func TestEveryViewCarriesTheQualifier(t *testing.T) {
	now := time.Date(2026, 8, 3, 15, 0, 0, 0, time.UTC)
	views := map[string]AvailabilityView{
		"empty":       BuildView(Snapshot{}, now),
		"refused":     BuildView(Snapshot{Version: 1, GeneratedAt: now, Status: StatusRefused}, now),
		"insufficent": BuildView(Snapshot{Version: 1, GeneratedAt: now, Status: StatusInsufficientData}, now),
		"available": BuildView(snapshotAt(now, Window{
			StartAt: now.Add(time.Hour), EndAt: now.Add(5 * time.Hour), ZoneID: "UTC",
		}), now),
		"stale": BuildView(snapshotAt(now.Add(-8*time.Hour), Window{
			StartAt: now.Add(-time.Hour), EndAt: now.Add(2 * time.Hour), ZoneID: "UTC",
		}), now),
	}
	for name, view := range views {
		if view.Qualifier != Qualifier {
			t.Errorf("%s view is missing the uncertainty qualifier", name)
		}
		if view.Notice != NoticeNotMedical {
			t.Errorf("%s view is missing the not-medical notice", name)
		}
		if view.Headline == "" {
			t.Errorf("%s view has no headline", name)
		}
	}
}

// TestRefusalDoesNotLeakItsReason keeps a typed estimator refusal private: a
// visitor learns that no estimate is available, never why.
func TestRefusalDoesNotLeakItsReason(t *testing.T) {
	now := time.Date(2026, 8, 3, 15, 0, 0, 0, time.UTC)
	view := BuildView(Snapshot{Version: 1, GeneratedAt: now, Status: StatusRefused}, now)
	lowered := strings.ToLower(view.Headline + " " + view.Detail)
	for _, forbidden := range []string{"sleep", "episode", "record", "refus", "ambiguous", "cycle", "estimator"} {
		if strings.Contains(lowered, forbidden) {
			t.Errorf("refusal view mentions %q: %q", forbidden, view.Detail)
		}
	}
}

// TestPastWindowsAreNotRendered stops the page from advertising availability
// that has already elapsed.
func TestPastWindowsAreNotRendered(t *testing.T) {
	now := time.Date(2026, 8, 3, 15, 0, 0, 0, time.UTC)
	view := BuildView(snapshotAt(now,
		Window{StartAt: now.Add(-10 * time.Hour), EndAt: now.Add(-4 * time.Hour), ZoneID: "UTC"},
		Window{StartAt: now.Add(3 * time.Hour), EndAt: now.Add(9 * time.Hour), ZoneID: "UTC"},
	), now)
	if len(view.Windows) != 1 {
		t.Fatalf("expected only the future window, got %d", len(view.Windows))
	}
	if view.LikelyAwake {
		t.Error("state says awake though no window contains now")
	}
	if !strings.Contains(view.Detail, "next likely waking window") {
		t.Errorf("detail does not point at the next window: %q", view.Detail)
	}
}

// TestCivilTimeIsPrimary holds the cross-phase invariant that a human reads
// clock time, not an instant, and that the zone is stated rather than assumed.
func TestCivilTimeIsPrimary(t *testing.T) {
	now := time.Date(2026, 8, 3, 15, 0, 0, 0, time.UTC)
	view := BuildView(snapshotAt(now, Window{
		StartAt: now.Add(2 * time.Hour),
		EndAt:   now.Add(8 * time.Hour),
		ZoneID:  "America/New_York",
	}), now)
	if len(view.Windows) != 1 {
		t.Fatalf("expected one window, got %d", len(view.Windows))
	}
	rendered := view.Windows[0].RangeLabel
	if !strings.Contains(rendered, "PM") && !strings.Contains(rendered, "AM") {
		t.Errorf("window is not rendered in civil time: %q", rendered)
	}
	if strings.Contains(rendered, "Z") || strings.Contains(rendered, "T") {
		t.Errorf("window leaks an RFC3339 instant into the visible label: %q", rendered)
	}
	if !strings.Contains(view.ZoneLabel, "America/New_York") {
		t.Errorf("zone is not stated: %q", view.ZoneLabel)
	}
}

// TestJSONAppliesTheSameAgeRules stops a JSON consumer from presenting an
// out-of-date "awake now" the HTML page would have withheld.
func TestJSONAppliesTheSameAgeRules(t *testing.T) {
	h := newHarness(t)
	now := h.clock.Now()
	h.publish(h.profile.ID, snapshotAt(now.Add(-25*time.Hour), Window{
		StartAt: now.Add(-time.Hour), EndAt: now.Add(3 * time.Hour), ZoneID: "UTC",
	}))
	cookie := h.sessionCookie(h.login(h.token, testPasscode, testOrigin))

	recorder := h.get("/p/"+h.token+"/availability", cookie)
	var decoded availabilityDTO
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Status != StatusInsufficientData {
		t.Errorf("status = %q, want %q for a day-old snapshot", decoded.Status, StatusInsufficientData)
	}
	if len(decoded.Windows) != 0 {
		t.Errorf("day-old snapshot still returned %d windows", len(decoded.Windows))
	}
}

// TestMissingSnapshotRendersHonestly covers a link created before the first
// materialization: it must say so rather than 404 or show an empty page.
func TestMissingSnapshotRendersHonestly(t *testing.T) {
	h := newHarness(t)
	cookie := h.sessionCookie(h.login(h.token, testPasscode, testOrigin))

	page := h.get("/p/"+h.token, cookie)
	if page.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, "not being shared right now") {
		t.Errorf("page does not explain the empty state: %s", body)
	}

	availability := h.get("/p/"+h.token+"/availability", cookie)
	var decoded availabilityDTO
	if err := json.Unmarshal(availability.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Status != StatusInsufficientData {
		t.Errorf("status = %q, want %q", decoded.Status, StatusInsufficientData)
	}
}

// TestGrantWithoutWindowsShowsNothing proves the grant is enforced on read,
// not merely respected by the materializer.
func TestSnapshotVersionDoesNotRegress(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	now := h.clock.Now()

	newer := snapshotAt(now, Window{StartAt: now.Add(time.Hour), EndAt: now.Add(5 * time.Hour), ZoneID: "UTC"})
	h.publish(h.profile.ID, newer)

	older := snapshotAt(now.Add(-2*time.Hour), Window{StartAt: now.Add(time.Hour), EndAt: now.Add(2 * time.Hour), ZoneID: "UTC"})
	if err := h.store.PublishSnapshot(ctx, h.profile.ID, older); err != nil {
		t.Fatalf("publish older: %v", err)
	}
	stored, err := h.store.ReadSnapshot(ctx, h.profile.ID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if stored.Version != newer.Version {
		t.Errorf("an out-of-order publish overwrote a newer snapshot (version %d, want %d)", stored.Version, newer.Version)
	}
}

// The figure draws the same windows as the list, on three civil days from
// local midnight: nothing before now, a window across midnight split between
// its days, and time past the estimate shaded rather than left looking free.
func TestFigureDrawsTheWindowsOnThreeDays(t *testing.T) {
	now := time.Date(2026, 8, 3, 15, 0, 0, 0, time.UTC) // 11:00 in New York
	view := BuildView(Snapshot{
		Version: 1, GeneratedAt: now, Status: StatusAvailable, HorizonEnd: now.Add(30 * time.Hour),
		Windows: []Window{
			{StartAt: now.Add(-time.Hour), EndAt: now.Add(6 * time.Hour), ZoneID: "America/New_York"},
			{StartAt: now.Add(10 * time.Hour), EndAt: now.Add(15 * time.Hour), ZoneID: "America/New_York"},
		},
	}, now)
	if len(view.Figure) != 3 || view.Figure[0].Label != "Today" || view.Figure[1].Label != "Tomorrow" {
		t.Fatalf("figure days = %#v", view.Figure)
	}
	today, tomorrow, third := view.Figure[0], view.Figure[1], view.Figure[2]
	// From now (11:00, 110 of 240) to 17:00, then 21:00 to midnight.
	if !today.HasNow || today.Now != 110 {
		t.Fatalf("now on today = %v %v", today.HasNow, today.Now)
	}
	if len(today.Bands) != 2 || today.Bands[0] != (FigureBand{X: 110, Width: 60}) || today.Bands[1] != (FigureBand{X: 210, Width: 30}) {
		t.Fatalf("today's bands = %#v", today.Bands)
	}
	// The second window runs on past midnight to 02:00.
	if len(tomorrow.Bands) != 1 || tomorrow.Bands[0] != (FigureBand{X: 0, Width: 20}) || tomorrow.HasNow {
		t.Fatalf("tomorrow = %#v", tomorrow)
	}
	// The estimate reaches 17:00 tomorrow; after that the rows are shaded.
	if !tomorrow.HasBeyond || tomorrow.Beyond != 170 || tomorrow.BeyondWidth != 70 {
		t.Fatalf("tomorrow's shading = %#v", tomorrow)
	}
	if !third.HasBeyond || third.Beyond != 0 || third.BeyondWidth != FigureWidth || len(third.Bands) != 0 {
		t.Fatalf("third day = %#v", third)
	}
	if !view.FigureBeyond() {
		t.Fatal("legend would not explain the shading")
	}
}

// When the estimate covers all three days nothing is shaded, and the legend
// does not offer a key for shading the figure does not show.
func TestFigureWithoutShadingHasNoBeyondKey(t *testing.T) {
	now := time.Date(2026, 8, 3, 15, 0, 0, 0, time.UTC)
	view := BuildView(Snapshot{
		Version: 1, GeneratedAt: now, Status: StatusAvailable, HorizonEnd: now.Add(96 * time.Hour),
		Windows: []Window{{StartAt: now.Add(time.Hour), EndAt: now.Add(5 * time.Hour), ZoneID: "UTC"}},
	}, now)
	for _, day := range view.Figure {
		if day.HasBeyond {
			t.Fatalf("a day inside the estimate is shaded: %#v", day)
		}
	}
	if view.FigureBeyond() {
		t.Fatal("legend offers a key for shading that is not drawn")
	}
}
