package portal

import (
	"strings"
	"testing"
	"time"
)

func TestPreviewIsThePageWithoutADocumentOrALiveLink(t *testing.T) {
	now := time.Date(2026, 8, 3, 15, 0, 0, 0, time.UTC)
	snapshot := snapshotAt(now, Window{StartAt: now.Add(time.Hour), EndAt: now.Add(5 * time.Hour), ZoneID: "UTC"})
	state := ProfileState{Profile: Profile{ID: "prof_preview", Grants: Grants{WakingWindows: true, AllowRequests: true}}}

	preview, err := RenderPreview(state, snapshot, now)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	page := BuildView(snapshot, now)
	for _, want := range []string{`class="masthead"`, page.Headline, "Fig. 1", "Likely waking windows", "Ask for a time", page.Qualifier} {
		if !strings.Contains(preview.HTML, want) {
			t.Errorf("preview lacks %q", want)
		}
	}
	// It is the page, not a document, and nothing in it leads anywhere: the
	// owner holds no link token, and a preview is not a visit.
	for _, banned := range []string{"<!DOCTYPE", "<html", "<head>", "<body", "<form", "href=", "/p/", "<script", `style="`} {
		if strings.Contains(preview.HTML, banned) {
			t.Errorf("preview contains %q", banned)
		}
	}
	for _, want := range []string{":host", ".portal-root"} {
		if !strings.Contains(preview.Stylesheet, want) {
			t.Errorf("stylesheet cannot style a shadow root: lacks %q", want)
		}
	}
}

func TestPreviewOfARequestFreeLinkOffersNoRequest(t *testing.T) {
	now := time.Date(2026, 8, 3, 15, 0, 0, 0, time.UTC)
	snapshot := snapshotAt(now, Window{StartAt: now.Add(time.Hour), EndAt: now.Add(5 * time.Hour), ZoneID: "UTC"})
	state := ProfileState{Profile: Profile{ID: "prof_preview", Grants: Grants{WakingWindows: true}}}

	preview, err := RenderPreview(state, snapshot, now)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(preview.HTML, "Ask for a time") {
		t.Error("a link without requests previews with a request button")
	}
}

// A revoked or expired link previews as what its recipient now gets: the
// unavailable page, not the availability it used to show.
func TestPreviewOfADeadLinkIsTheUnavailablePage(t *testing.T) {
	now := time.Date(2026, 8, 3, 15, 0, 0, 0, time.UTC)
	snapshot := snapshotAt(now, Window{StartAt: now.Add(time.Hour), EndAt: now.Add(5 * time.Hour), ZoneID: "UTC"})
	for name, state := range map[string]ProfileState{
		"revoked": {Profile: Profile{ID: "prof_revoked", Grants: Grants{WakingWindows: true}}, Revoked: true},
		"expired": {Profile: Profile{ID: "prof_expired", Grants: Grants{WakingWindows: true}}, Expired: true},
	} {
		preview, err := RenderPreview(state, snapshot, now)
		if err != nil {
			t.Fatalf("%s: render: %v", name, err)
		}
		if !strings.Contains(preview.HTML, "This link is no longer available.") {
			t.Errorf("%s link does not preview as unavailable", name)
		}
		if strings.Contains(preview.HTML, "Likely waking windows") {
			t.Errorf("%s link previews availability its recipient can no longer see", name)
		}
	}
}
