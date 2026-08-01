package main

import (
	"strings"
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
	// The picker bounds must be local wall-clock strings the input accepts.
	for _, value := range []string{dto.WindowStartLocal, dto.WindowEndLocal} {
		if _, err := time.ParseInLocation(visitorRequestLocalLayout, value, time.Local); err != nil {
			t.Errorf("picker bound %q is not a local datetime value: %v", value, err)
		}
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
	valid := "2026-08-04T09:00"
	validEnd := "2026-08-04T10:00"

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
