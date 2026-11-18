package portal

import (
	"context"
	"strings"
	"testing"
	"time"

	"non24.app/core/presence"
)

// The portal package imports nothing of the private side, so it names the
// statuses itself. They must be the ones the owner's devices send.
func TestThePortalNamesStatusesAsTheOwnersDevicesDo(t *testing.T) {
	if presenceAwake != presence.Awake || presenceTryingToSleep != presence.TryingToSleep || presenceDoNotDisturb != presence.DoNotDisturb {
		t.Fatal("the portal's status names have drifted from core/presence")
	}
}

func TestAReportedStatusLeadsThePageUntilItLapses(t *testing.T) {
	h := newHarness(t)
	now := h.clock.Now()
	snapshot := h.sampleSnapshot()
	snapshot.Reported = &ReportedStatus{
		Status: presenceTryingToSleep, ReportedAt: now.Add(-12 * time.Minute), ExpiresAt: now.Add(8 * time.Hour),
	}

	view := BuildView(snapshot, now)
	if view.Reported == nil || view.Reported.Headline != "Trying to sleep" {
		t.Fatalf("reported = %+v", view.Reported)
	}
	if !strings.Contains(view.Reported.Detail, "12 minutes ago") || !strings.Contains(view.Reported.Detail, "not a forecast") {
		t.Errorf("detail %q does not say when it was said, or that it is a report", view.Reported.Detail)
	}
	// The forecast is still there, under it.
	if view.Headline == "" || view.Unavailable {
		t.Errorf("the forecast was lost: %+v", view)
	}
	// The page asks again at the minute the status lapses.
	if next := nextChange(snapshot, now.Add(7*time.Hour+59*time.Minute)); !next.Equal(now.Add(8 * time.Hour)) {
		t.Errorf("next change at %s, want the status's expiry", next)
	}
	// At that minute it is gone, with nobody publishing anything.
	if lapsed := BuildView(snapshot, now.Add(8*time.Hour)); lapsed.Reported != nil {
		t.Errorf("a lapsed status is still shown: %+v", lapsed.Reported)
	}
	// A clock that ran ahead on the owner's device does not show it early.
	snapshot.Reported.ReportedAt = now.Add(time.Hour)
	if early := BuildView(snapshot, now); early.Reported != nil {
		t.Errorf("a status dated in the future is shown: %+v", early.Reported)
	}
}

func TestAReportedStatusShowsThoughTheForecastIsWithheld(t *testing.T) {
	h := newHarness(t)
	now := h.clock.Now()
	// Sleep records too old to forecast from, and a status said five minutes ago.
	snapshot := Snapshot{
		Version: now.UnixMilli(), GeneratedAt: now, Status: StatusInsufficientData,
		Reported: &ReportedStatus{Status: presenceAwake, ReportedAt: now.Add(-5 * time.Minute), ExpiresAt: now.Add(4 * time.Hour)},
	}
	view := BuildView(snapshot, now)
	if view.Reported == nil || view.Reported.Headline != "Awake" {
		t.Fatalf("reported = %+v", view.Reported)
	}
	if !strings.Contains(view.Reported.Detail, "does not mean being free to answer") {
		t.Errorf("detail %q lets awake read as available", view.Reported.Detail)
	}
	if !view.Unavailable {
		t.Error("the forecast should still be withheld")
	}
}

func TestAContactOnlyLinkSaysOnlyThatNowIsNotAGoodTime(t *testing.T) {
	h := newHarness(t)
	now := h.clock.Now()
	snapshot := h.sampleSnapshot()
	snapshot.Reported = &ReportedStatus{Status: StatusUnavailable, ExpiresAt: now.Add(2 * time.Hour)}
	if err := h.store.PublishSnapshot(context.Background(), h.profile.ID, snapshot); err != nil {
		t.Fatalf("publish: %v", err)
	}
	read, err := h.store.ReadSnapshot(context.Background(), h.profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	view := BuildView(read, now)
	if view.Reported == nil || view.Reported.Headline != "Not a good time right now" {
		t.Fatalf("reported = %+v", view.Reported)
	}
	for _, word := range []string{"sleep", "disturb", "ago"} {
		if strings.Contains(strings.ToLower(view.Reported.Headline+view.Reported.Detail), word) {
			t.Errorf("a contact-only link was told %q", word)
		}
	}

	// A snapshot may not say more than its level allows.
	snapshot.Reported.ReportedAt = now
	if err := h.store.PublishSnapshot(context.Background(), h.profile.ID, snapshot); err == nil {
		t.Error("a contact-only status that says when it was said was stored")
	}
	snapshot.Reported = &ReportedStatus{Status: "asleep", ReportedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := h.store.PublishSnapshot(context.Background(), h.profile.ID, snapshot); err == nil {
		t.Error("an unknown status was stored")
	}
}

func TestWhatALinkShowsCanBeChangedAfterItIsMade(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	now := h.clock.Now()
	if h.profile.Grants.ReportedStatus != StatusGrantNone {
		t.Fatalf("a new link shows a reported status by default: %q", h.profile.Grants.ReportedStatus)
	}
	grants := h.profile.Grants
	grants.ReportedStatus = StatusGrantFull
	if err := h.store.UpdateGrants(ctx, h.profile.ID, grants); err != nil {
		t.Fatal(err)
	}
	profile, _, _, err := h.store.LookupProfile(ctx, h.profile.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Grants.ReportedStatus != StatusGrantFull {
		t.Fatalf("level = %q after the update", profile.Grants.ReportedStatus)
	}
	// What a link may do, as against what it shows, is not changed here.
	if profile.Grants.AllowRequests != h.profile.Grants.AllowRequests || profile.Grants.AllowMessages != h.profile.Grants.AllowMessages {
		t.Error("updating what a link shows changed whether it takes requests or messages")
	}
	grants.ReportedStatus = "everything"
	if err := h.store.UpdateGrants(ctx, h.profile.ID, grants); err == nil {
		t.Error("an unknown level was accepted")
	}
	if err := h.store.UpdateGrants(ctx, "no-such-profile", h.profile.Grants); err != ErrProfileNotFound {
		t.Errorf("updating a link that does not exist gave %v", err)
	}
}
