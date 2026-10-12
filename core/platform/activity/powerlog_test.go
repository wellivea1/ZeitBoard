package activity

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"non24.app/core/domain"
)

// A rendered entry as Windows writes it, with event data that must be passed
// over: none of it comes back.
const renderedSleep = `<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System>
<Provider Name='Microsoft-Windows-Kernel-Power' Guid='{331c3b3a-2005-44c2-ac5e-77220c37d6b4}'/>
<EventID>42</EventID><Version>6</Version><Level>4</Level><Task>64</Task><Opcode>0</Opcode>
<TimeCreated SystemTime='2026-10-11T06:12:33.1234567Z'/><EventRecordID>48211</EventRecordID>
<Execution ProcessID='4' ThreadID='9012'/><Channel>System</Channel><Computer>canary-computer-name</Computer>
<Security UserID='S-1-5-18'/></System><EventData><Data Name='TargetState'>4</Data>
<Data Name='EffectiveState'>4</Data><Data Name='Reason'>canary-reason</Data></EventData></Event>`

func TestTheQueryAsksForSixEntriesAfterATime(t *testing.T) {
	query := powerLogQuery(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	want := "*[System[((Provider[@Name='Microsoft-Windows-Kernel-Power'] and (EventID=42 or EventID=506 or EventID=107 or EventID=507)) or " +
		"(Provider[@Name='Microsoft-Windows-Kernel-General'] and (EventID=13 or EventID=12))) and TimeCreated[@SystemTime>'2026-01-01T00:00:00.000Z']]]"
	if query != want {
		t.Fatalf("query\n  %s\nwant\n  %s", query, want)
	}
}

func TestAnEntryGivesUpItsTimeAndRecordOnly(t *testing.T) {
	event, ok := parsePowerEntry([]byte(renderedSleep))
	if !ok {
		t.Fatal("a sleep entry was not read")
	}
	if event.State != StateSuspended || event.RecordID != 48211 ||
		!event.At.Equal(time.Date(2026, 10, 11, 6, 12, 33, 123456700, time.UTC)) {
		t.Fatalf("read %+v", event)
	}
	for _, other := range []string{
		strings.Replace(renderedSleep, "<EventID>42</EventID>", "<EventID>41</EventID>", 1),
		strings.Replace(renderedSleep, "Kernel-Power", "Kernel-Processor-Power", 1),
		strings.Replace(renderedSleep, "2026-10-11T06:12:33.1234567Z", "yesterday", 1),
		"<Event><System>",
	} {
		if _, ok := parsePowerEntry([]byte(other)); ok {
			t.Errorf("read an entry that is not one of the six: %.80s", other)
		}
	}
}

func TestPowerEntriesBecomeObservedActivityOnce(t *testing.T) {
	recorded := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	events := []PowerEvent{
		{At: time.Date(2026, 10, 11, 6, 12, 0, 0, time.UTC), State: StateSuspended, RecordID: 48211},
		{At: time.Date(2026, 10, 11, 14, 2, 0, 0, time.UTC), State: StateResumed, RecordID: 48230},
	}
	first, err := PowerObservations(events, "UTC", recorded)
	if err != nil {
		t.Fatal(err)
	}
	again, err := PowerObservations(events[:1], "UTC", recorded.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if first[0].ID != again[0].ID || first[0].ExternalID != again[0].ExternalID {
		t.Fatal("the same entry read twice is not the same observation")
	}
	for _, observation := range first {
		if observation.SourceID != SourceID || observation.Evidence.Status != domain.StatusObserved ||
			observation.Evidence.Algorithm != PowerLogVersion || observation.ExternalID == "" {
			t.Fatalf("observation %+v", observation)
		}
		var body map[string]any
		if err := json.Unmarshal(observation.Payload, &body); err != nil {
			t.Fatal(err)
		}
		// An observed power event carries no unknown confidence, and nothing
		// beyond the closed shape.
		if _, has := body["confidence"]; has || len(body) > 3 {
			t.Fatalf("payload %v", body)
		}
	}

	transitions := Transitions(append(first, collectorObservation(t, time.Date(2026, 10, 11, 5, 0, 0, 0, time.UTC), StateIdle)))
	if len(transitions) != 3 || transitions[0].State != StateIdle || transitions[0].FromPowerLog ||
		!transitions[1].FromPowerLog || transitions[2].State != StateResumed {
		t.Fatalf("transitions %+v", transitions)
	}
}

func collectorObservation(t *testing.T, at time.Time, state State) domain.SourceObservation {
	t.Helper()
	body, err := Transition{At: at, State: state}.encode("windows")
	if err != nil {
		t.Fatal(err)
	}
	return domain.SourceObservation{
		ID: "activity-synthetic-1", SourceID: SourceID, Kind: "activity",
		ObservedAt: domain.ZonedInstant{UTC: at, ZoneID: "UTC"},
		Evidence:   domain.Evidence{Algorithm: CollectorVersion},
		Payload:    body,
	}
}
