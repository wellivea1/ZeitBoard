package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"non24.app/server/internal/portal"
	"non24.app/server/internal/portalbridge"
	"non24.app/server/internal/store"
)

type feedPage struct {
	Events []notificationDTO `json:"events"`
	Cursor int64             `json:"cursor"`
}

func readFeed(t *testing.T, h *testHarness, token, after string) (feedPage, string) {
	t.Helper()
	status, data := h.request(t, http.MethodGet, "/v1/notifications?after="+after, token, "")
	if status != http.StatusOK {
		t.Fatalf("feed status = %d body = %s", status, data)
	}
	var page feedPage
	if err := json.Unmarshal(data, &page); err != nil {
		t.Fatalf("decode feed: %v", err)
	}
	return page, string(data)
}

// TestNotificationFeedSaysWhatHappenedNeverWhatWasWritten is C6's server half:
// a visitor's request, their message and the owner's decision each become one
// event naming only its kind and request; replays add nothing; and a device
// turning notifications on starts at the head.
func TestNotificationFeedSaysWhatHappenedNeverWhatWasWritten(t *testing.T) {
	h, portalStore := newPortalHarness(t)
	ctx := t.Context()
	profileID, _ := portal.NewProfileID()
	linkToken, _ := portal.NewLinkToken()
	if err := portalStore.CreateProfile(ctx, portal.CreateProfileInput{
		ProfileID: profileID, Token: linkToken, Passcode: "long-enough-passcode",
		Grants:    portal.Grants{WakingWindows: true, AllowRequests: true, AllowMessages: true},
		CreatedAt: portalTestNow, ExpiresAt: portalTestNow.Add(30 * 24 * time.Hour),
	}); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := h.st.PutPortalLabel(ctx, profileID, "canary-link-label", portalTestNow.Format(time.RFC3339)); err != nil {
		t.Fatalf("label: %v", err)
	}
	profile, err := portalStore.ResolveLink(ctx, linkToken, portalTestNow)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	session, err := portalStore.CreateSession(ctx, profile, portalTestNow)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	token := h.registerDevice(t, "phone")
	start, _ := readFeed(t, h, token, "latest")

	created, err := portalStore.CreateRequest(ctx, profile, session.Session, portal.RequestInput{
		WindowStart: portalTestNow.Add(30 * time.Hour), WindowEnd: portalTestNow.Add(34 * time.Hour), ZoneID: "UTC",
		Handle: "canary-handle", Message: "canary-request-text",
	}, false, portalTestNow)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	// Written before the request reaches the owner: its notice waits for it.
	if _, err := portalStore.AppendMessage(ctx, profile, created.Request.ID, portal.AuthorVisitor, "canary-message-text", portalTestNow); err != nil {
		t.Fatalf("message: %v", err)
	}
	bridge := portalbridge.RequestBridge{Portal: portalStore, Private: h.st, Now: func() time.Time { return portalTestNow }}
	for i := 0; i < 2; i++ {
		if err := bridge.Pump(ctx); err != nil {
			t.Fatalf("pump %d: %v", i, err)
		}
	}

	page, raw := readFeed(t, h, token, fmt.Sprint(start.Cursor))
	if len(page.Events) != 2 || page.Events[0].Kind != store.NotifyVisitorRequest || page.Events[1].Kind != store.NotifyVisitorMessage {
		t.Fatalf("events = %+v", page.Events)
	}
	proposalID := page.Events[0].Subject
	if page.Events[1].Subject != proposalID {
		t.Errorf("the message notice points at %q, not its request %q", page.Events[1].Subject, proposalID)
	}
	for _, canary := range []string{"canary-handle", "canary-request-text", "canary-message-text", "canary-link-label", created.Request.ID} {
		if strings.Contains(raw, canary) {
			t.Errorf("the feed carries %q", canary)
		}
	}

	var list visitorRequestListResponse
	_, listData := h.request(t, http.MethodGet, "/v1/portal/requests", token, "")
	if err := json.Unmarshal(listData, &list); err != nil || len(list.Requests) != 1 {
		t.Fatalf("list: %v", err)
	}
	if status, data := h.request(t, http.MethodPost, "/v1/portal/requests/"+proposalID+"/decision", token,
		fmt.Sprintf(`{"decision":"rejected","token":%q}`, list.Requests[0].DecisionToken)); status != http.StatusOK {
		t.Fatalf("decline status = %d body = %s", status, data)
	}
	next, _ := readFeed(t, h, token, fmt.Sprint(page.Cursor))
	if len(next.Events) != 1 || next.Events[0].Kind != store.NotifyVisitorDecided || next.Events[0].Subject != proposalID {
		t.Fatalf("after the decision = %+v", next.Events)
	}
	if again, _ := readFeed(t, h, token, fmt.Sprint(next.Cursor)); len(again.Events) != 0 || again.Cursor != next.Cursor {
		t.Errorf("a caught-up device read %+v", again)
	}

	// A device turning notifications on now is told nothing about the past.
	if latest, _ := readFeed(t, h, token, "latest"); len(latest.Events) != 0 || latest.Cursor != next.Cursor {
		t.Errorf("latest = %+v, want no events at cursor %d", latest, next.Cursor)
	}
	if status, _ := h.request(t, http.MethodGet, "/v1/notifications?after=-3", token, ""); status != http.StatusBadRequest {
		t.Errorf("negative cursor status = %d", status)
	}
	if status, _ := h.request(t, http.MethodGet, "/v1/notifications", "", ""); status != http.StatusUnauthorized {
		t.Errorf("feed without a device token status = %d", status)
	}
}

func TestNotificationsExpire(t *testing.T) {
	h, _ := newPortalHarness(t)
	ctx := t.Context()
	events, _, err := h.st.NotificationsAfter(ctx, 0, 10, portalTestNow)
	if err != nil || len(events) != 0 {
		t.Fatalf("empty feed = %+v %v", events, err)
	}
	if err := h.st.RecordVisitorMessageNotification(ctx, "no-such-request", "m1", portalTestNow); err != store.ErrNoVisitorProposal {
		t.Errorf("a message notice before its request = %v", err)
	}
	if err := h.st.PurgeNotifications(ctx, portalTestNow.Add(store.NotificationRetention+time.Hour)); err != nil {
		t.Fatal(err)
	}
}
