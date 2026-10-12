package portal

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"html"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Doses logged as skipped, on a link that shows them (ADR-0058).

func (h *harness) dosesSnapshot(doses ...SkippedDose) Snapshot {
	snapshot := h.sampleSnapshot()
	snapshot.DosesShared = true
	snapshot.SkippedDoses = doses
	return snapshot
}

func (h *harness) pageAndAvailability(token string) (string, map[string]any) {
	h.t.Helper()
	cookie := h.sessionCookie(h.login(token, testPasscode, testOrigin))
	page := h.get("/p/"+token, cookie)
	if page.Code != http.StatusOK {
		h.t.Fatalf("page status = %d", page.Code)
	}
	availability := h.get("/p/"+token+"/availability", cookie)
	var decoded map[string]any
	if err := json.Unmarshal(availability.Body.Bytes(), &decoded); err != nil {
		h.t.Fatalf("decode availability: %v", err)
	}
	return page.Body.String(), decoded
}

// Only the doses logged as skipped in the last 72 hours are shown, newest
// first, by time alone; one in the future or older than that is not.
func TestALinkShowsTheDosesLoggedAsSkippedInTheLastThreeDays(t *testing.T) {
	h := newHarness(t)
	token, profile := h.createProfile(Grants{WakingWindows: true, SkippedDoses: true})
	now := h.clock.Now() // 15:00 UTC, 11:00 in New York
	h.publish(profile.ID, h.dosesSnapshot(
		SkippedDose{At: now.Add(-26 * time.Hour), ZoneID: "America/New_York"},
		SkippedDose{At: now.Add(-2 * time.Hour), ZoneID: "America/New_York"},
		SkippedDose{At: now.Add(-80 * time.Hour), ZoneID: "America/New_York"},
		SkippedDose{At: now.Add(time.Hour), ZoneID: "America/New_York"},
	))

	page, availability := h.pageAndAvailability(token)
	if !strings.Contains(page, html.EscapeString(NoticeWithDoses)) || strings.Contains(page, NoticeNotMedical) {
		t.Error("a page that shows doses still says it holds no medical information")
	}
	section := page[strings.Index(page, "Doses logged as skipped"):]
	today := strings.Index(section, "Today")
	yesterday := strings.Index(section, "Yesterday")
	if today < 0 || yesterday < 0 || today > yesterday || strings.Count(section, "9:00 AM") != 2 {
		t.Fatalf("the two recent doses are not listed newest first:\n%s", section)
	}
	for _, hidden := range []string{"12:00 PM", "Fri, Jul 31"} {
		if strings.Contains(section, hidden) {
			t.Errorf("a dose outside the window is shown (%s)", hidden)
		}
	}

	doses, ok := availability["skippedDoses"].([]any)
	if !ok || len(doses) != 2 {
		t.Fatalf("availability skippedDoses = %v", availability["skippedDoses"])
	}
	for _, entry := range doses {
		for key := range entry.(map[string]any) {
			if key != "at" && key != "zoneId" {
				t.Errorf("a dose carries %q", key)
			}
		}
	}
}

// A link without the grant is exactly what it was: no section, no key.
func TestALinkWithoutTheGrantShowsNoDoses(t *testing.T) {
	h := newHarness(t)
	h.publish(h.profile.ID, h.sampleSnapshot())
	page, availability := h.pageAndAvailability(h.token)
	if strings.Contains(page, "Doses logged as skipped") || !strings.Contains(page, NoticeNotMedical) {
		t.Error("a link without the grant shows a doses section or a different notice")
	}
	if _, present := availability["skippedDoses"]; present {
		t.Error("a link without the grant has a skippedDoses key")
	}
}

// "None" is only said where doses are shared; a list is always present there,
// even empty, so a reader can tell none from not shared.
func TestASharedLinkWithNoneSaysSo(t *testing.T) {
	h := newHarness(t)
	token, profile := h.createProfile(Grants{WakingWindows: true, SkippedDoses: true})
	h.publish(profile.ID, h.dosesSnapshot())
	page, availability := h.pageAndAvailability(token)
	if !strings.Contains(page, "None in the last three days.") {
		t.Error("a shared link with no skipped dose does not say so")
	}
	if doses, ok := availability["skippedDoses"].([]any); !ok || len(doses) != 0 {
		t.Errorf("skippedDoses = %v, want an empty list", availability["skippedDoses"])
	}
}

// Skipped doses do not depend on the forecast, so a link shows them while it
// withholds the windows.
func TestDosesAreShownWhateverTheForecastSays(t *testing.T) {
	h := newHarness(t)
	token, profile := h.createProfile(Grants{SkippedDoses: true})
	snapshot := h.dosesSnapshot(SkippedDose{At: h.clock.Now().Add(-time.Hour), ZoneID: "UTC"})
	snapshot.Status = StatusRefused
	snapshot.Windows = nil
	h.publish(profile.ID, snapshot)
	page, availability := h.pageAndAvailability(token)
	if !strings.Contains(page, "Availability cannot be estimated") || !strings.Contains(page, "2:00 PM") {
		t.Errorf("the refused page does not list the dose:\n%s", page)
	}
	if doses, ok := availability["skippedDoses"].([]any); !ok || len(doses) != 1 {
		t.Errorf("skippedDoses = %v", availability["skippedDoses"])
	}
}

// An open page refreshes when a dose leaves it.
func TestAnOpenPageRefreshesWhenADoseLeavesIt(t *testing.T) {
	now := time.Date(2026, 8, 3, 15, 0, 0, 0, time.UTC)
	snapshot := Snapshot{
		GeneratedAt: now, Status: StatusRefused, DosesShared: true,
		SkippedDoses: []SkippedDose{{At: now.Add(-70 * time.Hour), ZoneID: "UTC"}},
	}
	if got, want := nextChange(snapshot, now), now.Add(2*time.Hour); !got.Equal(want) {
		t.Errorf("next change at %s, want %s", got, want)
	}
}

func TestTheSkippedDosesGrantIsStored(t *testing.T) {
	h := newHarness(t)
	_, profile := h.createProfile(Grants{SkippedDoses: true})
	stored, _, _, err := h.store.LookupProfile(context.Background(), profile.ID, h.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !stored.Grants.SkippedDoses || stored.Grants.WakingWindows {
		t.Errorf("grants stored as %+v", stored.Grants)
	}
}

// A snapshot cannot carry doses to a link that does not share them, so a
// mistake upstream cannot leak them.
func TestASnapshotCannotCarryDosesALinkDoesNotShare(t *testing.T) {
	h := newHarness(t)
	snapshot := h.sampleSnapshot()
	snapshot.SkippedDoses = []SkippedDose{{At: h.clock.Now(), ZoneID: "UTC"}}
	if err := h.store.PublishSnapshot(context.Background(), h.profile.ID, snapshot); err == nil {
		t.Fatal("doses were published to a link that does not share them")
	}
}

// A snapshot written before doses existed is a bare list of windows, and it
// still reads.
func TestASnapshotFromBeforeDosesStillReads(t *testing.T) {
	h := newHarness(t)
	now := h.clock.Now()
	windows, err := json.Marshal([]Window{{StartAt: now, EndAt: now.Add(time.Hour), ZoneID: "UTC"}})
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, h.store.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	version := now.UnixMilli()
	sealed := h.store.aead.Seal(nil, nonce, windows, snapshotAAD(h.profile.ID, version))
	if _, err := h.store.db.Exec(`INSERT INTO portal_snapshots
		(profile_id, version, generated_at, horizon_end, status, nonce, ciphertext)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, h.profile.ID, version, formatTime(now), formatTime(now.Add(time.Hour)),
		StatusAvailable, nonce, sealed); err != nil {
		t.Fatal(err)
	}
	snapshot, err := h.store.ReadSnapshot(context.Background(), h.profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Windows) != 1 || snapshot.DosesShared || len(snapshot.SkippedDoses) != 0 {
		t.Errorf("read %+v", snapshot)
	}
}
