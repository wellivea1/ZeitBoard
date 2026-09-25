package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestVisitorRequestActionIDMatchesServer pins the duplicated constant. The
// desktop cannot import the server module, so if the server ever renames the
// action this test is the thing that has to be updated deliberately rather
// than the filter silently ceasing to match.
func TestVisitorRequestActionIDMatchesServer(t *testing.T) {
	if visitorRequestActionID != "place_visitor_request" {
		t.Fatalf("visitorRequestActionID = %q; update it together with store.ActionVisitorRequest",
			visitorRequestActionID)
	}
}

// TestVisitorProposalsAreOmittedFromTheGenericList is the regression guard.
// The generic decision route refuses visitor proposals on purpose, so listing
// one beside an Approve button would offer a control that cannot work.
func TestVisitorProposalsAreOmittedFromTheGenericList(t *testing.T) {
	records := []backendProposalRecord{
		{ProposalID: "p1", ActionID: "propose_place_task", Status: "pending"},
		{ProposalID: "p2", ActionID: visitorRequestActionID, Status: "pending"},
		{ProposalID: "p3", ActionID: "propose_move_task", Status: "pending"},
	}
	kept := make([]string, 0, len(records))
	for _, record := range records {
		if record.ActionID == visitorRequestActionID {
			continue
		}
		kept = append(kept, record.ProposalID)
	}
	if len(kept) != 2 || kept[0] != "p1" || kept[1] != "p3" {
		t.Fatalf("generic list kept %v, want the two task proposals only", kept)
	}
}

func TestVisitorRequestDTORendersOwnerContext(t *testing.T) {
	start := time.Date(2026, 8, 4, 14, 0, 0, 0, time.UTC)
	dto := backendVisitorRequestDTO(backendVisitorRequestRecord{
		ProposalID:      "visitor-1",
		Label:           "Mum",
		Status:          "pending",
		WindowStartAt:   start,
		WindowEndAt:     start.Add(4 * time.Hour),
		ZoneID:          "America/New_York",
		DurationMinutes: 45,
		Handle:          "Sam",
		Message:         "coffee?",
		CreatedAt:       start.Add(-2 * time.Hour),
		ExpiresAt:       start.Add(72 * time.Hour),
		DecisionToken:   "token",
		Disclosure:      "Approving tells them the exact time you pick.",
	})

	if dto.LinkLabel != "Mum" {
		t.Errorf("link label = %q", dto.LinkLabel)
	}
	if dto.Handle != "Sam" || dto.Message != "coffee?" {
		t.Errorf("the owner cannot see what was asked: %+v", dto)
	}
	if dto.DurationLabel != "45 minutes" {
		t.Errorf("duration label = %q", dto.DurationLabel)
	}
	if dto.ApprovalDisclosure == "" {
		t.Error("the approval disclosure is missing")
	}
	if dto.WindowStartAt != start.Format(time.RFC3339Nano) || dto.WindowEndAt != start.Add(4*time.Hour).Format(time.RFC3339Nano) {
		t.Fatal("picker lost exact bounds")
	}

}

func TestVisitorRequestDTONamesAnUnlabelledLink(t *testing.T) {
	dto := backendVisitorRequestDTO(backendVisitorRequestRecord{
		ProposalID:    "visitor-2",
		Label:         "   ",
		WindowStartAt: time.Now().UTC(),
		WindowEndAt:   time.Now().UTC().Add(time.Hour),
	})
	if dto.LinkLabel == "" || strings.TrimSpace(dto.LinkLabel) == "" {
		t.Error("an unlabelled link rendered as blank rather than being named")
	}
}

func TestVisitorRequestDTOWarnsBeyondTheHorizon(t *testing.T) {
	dto := backendVisitorRequestDTO(backendVisitorRequestRecord{
		ProposalID:    "visitor-3",
		WindowStartAt: time.Now().UTC().Add(90 * 24 * time.Hour),
		WindowEndAt:   time.Now().UTC().Add(90*24*time.Hour + 2*time.Hour),
		BeyondHorizon: true,
	})
	if dto.BeyondHorizonNote == "" {
		t.Error("a request past the horizon carried no infeasibility note")
	}
}

func TestParseVisitorSlotRules(t *testing.T) {
	valid := "2026-08-04T09:00:00-04:00"
	validEnd := "2026-08-04T10:00:00-04:00"

	start, end, err := parseVisitorSlot(valid, validEnd)
	if err != nil {
		t.Fatalf("a valid block was refused: %v", err)
	}
	if !end.After(start) {
		t.Error("parsed block does not advance")
	}
	if start.Location() != time.UTC {
		t.Error("the block was not normalized to UTC before leaving the desktop")
	}

	cases := map[string][2]string{
		"no offset":        {"2026-08-04T09:00", validEnd},
		"empty start":      {"", validEnd},
		"empty end":        {valid, ""},
		"unparsable":       {"tomorrow morning", validEnd},
		"end before start": {validEnd, valid},
		"zero length":      {valid, valid},
	}
	for name, pair := range cases {
		if _, _, err := parseVisitorSlot(pair[0], pair[1]); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestVisitorSlotPreservesChosenOccurrence(t *testing.T) {
	start, end, err := parseVisitorSlot("2026-11-01T01:15:00.123456789-04:00", "2026-11-01T01:15:00.123456789-05:00")
	if err != nil || end.Sub(start) != time.Hour || start.Nanosecond() != 123456789 {
		t.Fatalf("repeated-hour block changed: %v", err)
	}
}

func TestVisitorQueueCurrentWireContractAndConfirmedDecision(t *testing.T) {
	app := newTestApp(t)
	var decided atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/devices":
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(registerDeviceResponse{SchemaVersion: "v1", DeviceID: "device_desktop", Token: "synthetic-token"})
		case "/v1/portal/requests":
			if decided.Load() {
				http.Error(w, "synthetic unavailable", http.StatusServiceUnavailable)
				return
			}
			if cursor := r.URL.Query().Get("cursor"); cursor != "" && cursor != "opaque visitor cursor" {
				t.Error("cursor changed")
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"schema_version":"v1","pendingCount":5,"nextExpiryAt":"2099-01-01T00:00:00Z","requests":[{"proposalId":"visitor-1","profileId":"family","label":"Family","status":"pending","windowStartAt":"2026-11-01T05:00:00Z","windowEndAt":"2026-11-01T07:00:00Z","zoneId":"America/New_York","durationMinutes":30,"beyondHorizon":false,"handle":"Sam","message":"Synthetic request","createdAt":"2026-10-31T12:00:00Z","expiresAt":"2099-01-01T00:00:00Z","decisionToken":"synthetic-decision-token","disclosure":"Exact accepted time is shared."}],"pagination":{"limit":50,"hasMore":false}}`))
		case "/v1/portal/requests/visitor-1/decision":
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			if payload["startAt"] != "2026-11-01T05:30:00Z" || payload["endAt"] != "2026-11-01T06:30:00Z" {
				t.Error("chosen repeated-hour block changed")
			}
			decided.Store(true)
			w.Write([]byte(`{"schema_version":"v1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	configureBackendForTest(t, app, server.URL)
	list, err := app.GetBackendVisitorRequestPage(BackendProposalPageInput{Cursor: "opaque visitor cursor"})
	if err != nil || list.Status != "ok" || list.PendingCount != 5 || len(list.Requests) != 1 {
		t.Fatalf("current visitor response rejected: %v / %s", err, list.Message)
	}
	encoded, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"nextCursor":""`) || !strings.Contains(string(encoded), `"status":"pending"`) {
		t.Fatal("terminal pagination or history status omitted")
	}
	result, err := app.DecideBackendVisitorRequest(DecideBackendVisitorRequestInput{ProposalID: "visitor-1", Decision: "approved", Token: "synthetic-decision-token", StartAt: "2026-11-01T01:30:00-04:00", EndAt: "2026-11-01T01:30:00-05:00"})
	if err != nil || !result.DecisionRecorded || result.Status != "error" || !strings.Contains(result.Message, "Decision recorded") {
		t.Fatalf("confirmed decision lost to failed refresh: %v", err)
	}
}
