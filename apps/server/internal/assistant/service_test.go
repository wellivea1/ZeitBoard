package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"non24.app/core/agentpolicy"
	"non24.app/server/internal/auth"
	"non24.app/server/internal/provider"
	"non24.app/server/internal/store"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type fakeProvider struct {
	responses []provider.Response
	errs      []error
	requests  []provider.Request
}

func (f *fakeProvider) Complete(_ context.Context, req provider.Request) (provider.Response, error) {
	f.requests = append(f.requests, req)
	index := len(f.requests) - 1
	if index < len(f.errs) && f.errs[index] != nil {
		return provider.Response{}, f.errs[index]
	}
	if index < len(f.responses) {
		return f.responses[index], nil
	}
	return provider.Response{}, errors.New("unexpected provider call")
}

func (f *fakeProvider) Status() provider.Status {
	return provider.Status{Configured: true, Provider: "fake", Model: "fake-model"}
}

func TestAssistantCreatesPendingProposalOnlyThroughScheduler(t *testing.T) {
	st, device := testStoreAndDevice(t)
	fake := &fakeProvider{responses: []provider.Response{{Text: `{"schema_version":"v1","recommended_action":"propose_place_task","target":{"task_id":"task_flexible_01"},"answer":"I can queue this for approval."}`}}}
	service := New(fake, fake.Status(), st)
	service.now = fixedNow

	resp, err := service.HandleMessage(context.Background(), device, MessageRequest{
		SchemaVersion: SchemaVersion,
		Message:       "Find 30 minutes for taxes before tomorrow.",
		Context:       planningContext(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Result != ResultProposal || len(resp.Proposals) != 1 {
		t.Fatalf("response = %+v, want one pending proposal", resp)
	}
	count, err := st.CountProposals(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("proposal count = %d, want 1", count)
	}
	var payload storedProposalPayload
	if err := json.Unmarshal(resp.Proposals[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.ScheduleProposals.Proposals) != 1 ||
		payload.ScheduleProposals.Proposals[0].TaskID != "task_flexible_01" ||
		len(payload.ScheduleProposals.Proposals[0].ExplanationCodes) == 0 {
		t.Fatalf("stored payload = %+v", payload)
	}
	validateScheduleProposalsPayload(t, payload.ScheduleProposals)
}

func TestAssistantRejectsUnknownActionWithoutProposal(t *testing.T) {
	st, device := testStoreAndDevice(t)
	fake := &fakeProvider{responses: []provider.Response{
		{Text: `{"schema_version":"v1","recommended_action":"delete_calendar","target":{"task_id":"task_flexible_01"}}`},
		{Text: `{"schema_version":"v1","recommended_action":"delete_calendar","target":{"task_id":"task_flexible_01"}}`},
	}}
	service := New(fake, fake.Status(), st)
	service.now = fixedNow

	resp, err := service.HandleMessage(context.Background(), device, MessageRequest{
		SchemaVersion: SchemaVersion,
		Message:       "Move everything.",
		Context:       planningContext(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Result != ResultUnknown || len(resp.Proposals) != 0 {
		t.Fatalf("response = %+v, want unknown with zero proposals", resp)
	}
	count, err := st.CountProposals(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("proposal count = %d, want 0", count)
	}
}

func TestAssistantRedactsContextSentToProvider(t *testing.T) {
	st, device := testStoreAndDevice(t)
	fake := &fakeProvider{responses: []provider.Response{{Text: `{"schema_version":"v1","recommended_action":"answer_only","answer":"You have a predicted waking window available."}`}}}
	service := New(fake, fake.Status(), st)
	service.now = fixedNow
	planning := planningContext()
	planning.MedicationFacts = []MedicationFactContext{
		{
			MedicationID: "med_opaque_01", Active: true, ScheduleKind: "fixed_clock",
			ScheduledOccurrenceCount: 14, CollisionCount: 2,
			NextScheduledCivilDate: "2026-03-06", NextScheduledCivilTime: "20:00", ScheduleZoneID: "UTC",
			LoggedEventCount: 3, LastLoggedStatus: "taken",
			LastWakeRelation:  "2 h after recorded wake",
			LastSleepRelation: "4 h 30 min before predicted sleep", Confidence: "Medium",
		},
		{
			MedicationID: "med_opaque_02", ScheduleKind: "none",
			LastWakeRelation: "Metformin private timing", LastSleepRelation: "api-key-secret",
		},
	}
	planning.Markers = []RhythmMarkerFactContext{
		{MarkerID: "marker_opaque_01", Kind: "travel", CivilStartDate: "2026-03-01", CivilEndDate: "2026-03-03"},
		{MarkerID: "marker_opaque_02", Kind: "calendar-secret-text", CivilStartDate: "2026-03-02"},
	}

	_, err := service.HandleMessage(context.Background(), device, MessageRequest{
		SchemaVersion: SchemaVersion,
		Message:       "Can I fit the task before Friday?",
		Context:       planning,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.requests) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(fake.requests))
	}
	captured, err := json.Marshal(fake.requests[0])
	if err != nil {
		t.Fatal(err)
	}
	forbidden := [][]byte{
		[]byte("Metformin"),
		[]byte("calendar-secret-text"),
		[]byte("api-key-secret"),
		[]byte("2026-03-05T"),
		[]byte("med_opaque_01"),
		[]byte("marker_opaque_01"),
		[]byte("medication_facts"),
		[]byte(`"markers"`),
	}
	for _, item := range forbidden {
		if bytes.Contains(captured, item) {
			t.Fatalf("provider request leaked forbidden value %q: %s", item, captured)
		}
	}
	for _, required := range [][]byte{[]byte("task_flexible_01"), []byte("event_fixed_01"), []byte("fixed_events"), []byte("predicted_wake")} {
		if !bytes.Contains(captured, required) {
			t.Fatalf("provider request omitted redacted planning field %q: %s", required, captured)
		}
	}
}

func TestAssistantNoProviderUsesLocalFallbackAndNoNetwork(t *testing.T) {
	st, device := testStoreAndDevice(t)
	service := New(provider.DisabledClient{}, provider.DisabledClient{}.Status(), st)
	service.now = fixedNow

	resp, err := service.HandleMessage(context.Background(), device, MessageRequest{
		SchemaVersion: SchemaVersion,
		Message:       "Where does my data go?",
		Context:       planningContext(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Result != ResultAnswerOnly || len(resp.Proposals) != 0 {
		t.Fatalf("response = %+v, want local answer only", resp)
	}
	count, err := st.CountProposals(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("proposal count = %d, want 0", count)
	}
}

func TestAssistantUsageLimitFallsBackCalmly(t *testing.T) {
	st, device := testStoreAndDevice(t)
	fake := &fakeProvider{errs: []error{provider.ErrUsageLimit}}
	service := New(fake, fake.Status(), st)
	service.now = fixedNow

	resp, err := service.HandleMessage(context.Background(), device, MessageRequest{
		SchemaVersion: SchemaVersion,
		Message:       "Find time for the task.",
		Context:       planningContext(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Result != ResultAnswerOnly || len(resp.Proposals) != 0 {
		t.Fatalf("response = %+v, want calm fallback with zero proposals", resp)
	}
	if !bytes.Contains([]byte(resp.Answer), []byte("usage limit")) {
		t.Fatalf("fallback answer did not mention usage limit: %q", resp.Answer)
	}
}

func TestAssistantMedicalPromptRefusesWithoutProposal(t *testing.T) {
	st, device := testStoreAndDevice(t)
	fake := &fakeProvider{responses: []provider.Response{{Text: `{"schema_version":"v1","recommended_action":"answer_only","answer":"bad"}`}}}
	service := New(fake, fake.Status(), st)
	service.now = fixedNow

	resp, err := service.HandleMessage(context.Background(), device, MessageRequest{
		SchemaVersion: SchemaVersion,
		Message:       "Should I change my melatonin dose?",
		Context:       planningContext(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Result != ResultRefused || len(resp.Proposals) != 0 {
		t.Fatalf("response = %+v, want refusal with zero proposals", resp)
	}
	if resp.Answer != agentpolicy.MedicalRefusal {
		t.Fatalf("medical refusal = %q", resp.Answer)
	}
	if len(fake.requests) != 0 {
		t.Fatalf("provider was called for a medical prompt")
	}
}

func TestAssistantMedicationFactsAreDeterministicAndProviderFree(t *testing.T) {
	st, device := testStoreAndDevice(t)
	fake := &fakeProvider{responses: []provider.Response{{Text: `{"schema_version":"v1","recommended_action":"answer_only","answer":"You should take 5 mg at 8 PM."}`}}}
	service := New(fake, fake.Status(), st)
	service.now = fixedNow
	planning := planningContext()
	planning.MedicationFacts = []MedicationFactContext{
		{
			MedicationID: "med_opaque_01", Active: true, ScheduleKind: "fixed_clock",
			ScheduledOccurrenceCount: 14, CollisionCount: 2,
			NextScheduledCivilDate: "2026-03-06", NextScheduledCivilTime: "20:00", ScheduleZoneID: "UTC",
			LoggedEventCount: 3, LastLoggedStatus: "taken", LastWakeRelation: "2 h after recorded wake",
		},
		{
			MedicationID: "med_opaque_02", ScheduleKind: "none",
			LastWakeRelation: "private-medication-note", LastSleepRelation: "api-key-secret",
		},
	}

	resp, err := service.HandleMessage(context.Background(), device, MessageRequest{
		SchemaVersion: SchemaVersion,
		Message:       "Show my next scheduled medication timing fact.",
		Context:       planning,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Result != ResultAnswerOnly || len(resp.Proposals) != 0 ||
		!strings.Contains(resp.Answer, "2026-03-06 20:00 UTC") ||
		!strings.Contains(resp.Answer, "not dosing or treatment advice") {
		t.Fatalf("deterministic medication answer = %#v", resp)
	}
	for _, forbidden := range []string{"You should take", "private-medication-note", "api-key-secret"} {
		if strings.Contains(resp.Answer, forbidden) {
			t.Fatalf("deterministic medication answer leaked %q: %q", forbidden, resp.Answer)
		}
	}
	if len(fake.requests) != 0 {
		t.Fatalf("provider was called for medication facts: %#v", fake.requests)
	}
	count, err := st.CountProposals(context.Background())
	if err != nil || count != 0 {
		t.Fatalf("medication fact answer created proposals: count=%d err=%v", count, err)
	}
}

func TestAssistantMarkerFactsDoNotIncludeMedicationFacts(t *testing.T) {
	st, device := testStoreAndDevice(t)
	fake := &fakeProvider{responses: []provider.Response{{Text: `{"schema_version":"v1","recommended_action":"answer_only","answer":"provider should not run"}`}}}
	service := New(fake, fake.Status(), st)
	service.now = fixedNow
	planning := planningContext()
	planning.MedicationFacts = []MedicationFactContext{{
		MedicationID: "med_opaque_01", Active: true, ScheduleKind: "fixed_clock",
		NextScheduledCivilDate: "2026-03-06", NextScheduledCivilTime: "20:00", ScheduleZoneID: "UTC",
	}}
	planning.Markers = []RhythmMarkerFactContext{
		{MarkerID: "marker_opaque_01", Kind: "travel", CivilStartDate: "2026-03-01", CivilEndDate: "2026-03-03"},
		{MarkerID: "marker_opaque_02", Kind: "private-causal-claim", CivilStartDate: "2026-03-02"},
	}

	resp, err := service.HandleMessage(context.Background(), device, MessageRequest{
		SchemaVersion: SchemaVersion,
		Message:       "List my light therapy markers.",
		Context:       planning,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Result != ResultAnswerOnly || len(resp.Proposals) != 0 ||
		!strings.Contains(resp.Answer, "travel on 2026-03-01 through 2026-03-03") ||
		strings.Contains(resp.Answer, "Medication") || strings.Contains(resp.Answer, "private-causal-claim") {
		t.Fatalf("marker-only answer = %#v", resp)
	}
	if len(fake.requests) != 0 {
		t.Fatalf("provider was called for marker facts: %#v", fake.requests)
	}
}

func TestAssistantRejectsInvalidPlanningContextBeforeProvider(t *testing.T) {
	st, device := testStoreAndDevice(t)
	fake := &fakeProvider{responses: []provider.Response{{Text: `{"schema_version":"v1","recommended_action":"answer_only","answer":"provider should not run"}`}}}
	service := New(fake, fake.Status(), st)
	planning := planningContext()
	planning.FixedEvents[0].EndAt = planning.FixedEvents[0].StartAt

	_, err := service.HandleMessage(context.Background(), device, MessageRequest{
		SchemaVersion: SchemaVersion,
		Message:       "Find a task window.",
		Context:       planning,
	})
	if err == nil {
		t.Fatal("invalid fixed event was accepted")
	}
	if len(fake.requests) != 0 {
		t.Fatalf("provider was called with invalid planning context: %#v", fake.requests)
	}
	count, countErr := st.CountProposals(context.Background())
	if countErr != nil || count != 0 {
		t.Fatalf("invalid planning context created proposals: count=%d err=%v", count, countErr)
	}
}

func TestAssistantMalformedOutputRetriesOnceThenAnswerOnly(t *testing.T) {
	st, device := testStoreAndDevice(t)
	fake := &fakeProvider{responses: []provider.Response{
		{Text: `not json`},
		{Text: `{"schema_version":"v1","recommended_action":"answer_only","answer":"I cannot make a safe proposal."}`},
	}}
	service := New(fake, fake.Status(), st)
	service.now = fixedNow

	resp, err := service.HandleMessage(context.Background(), device, MessageRequest{
		SchemaVersion: SchemaVersion,
		Message:       "Move my task.",
		Context:       planningContext(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.requests) != 2 {
		t.Fatalf("provider calls = %d, want retry once", len(fake.requests))
	}
	if resp.Result != ResultAnswerOnly || len(resp.Proposals) != 0 {
		t.Fatalf("response = %+v, want answer only after retry", resp)
	}
}

func TestApprovalTokenWorksOnceAndExpires(t *testing.T) {
	st, device := testStoreAndDevice(t)
	fake := &fakeProvider{responses: []provider.Response{
		{Text: `{"schema_version":"v1","recommended_action":"propose_place_task","target":{"task_id":"task_flexible_01"}}`},
		{Text: `{"schema_version":"v1","recommended_action":"propose_place_task","target":{"task_id":"task_flexible_01"}}`},
	}}
	service := New(fake, fake.Status(), st)
	service.now = fixedNow

	resp, err := service.HandleMessage(context.Background(), device, MessageRequest{SchemaVersion: SchemaVersion, Message: "Queue a task.", Context: planningContext()})
	if err != nil {
		t.Fatal(err)
	}
	token := resp.Proposals[0].DecisionToken
	approved, err := st.DecideProposal(context.Background(), resp.Proposals[0].ProposalID, device.ID, store.ProposalApproved, token, fixedNow().Add(time.Minute), json.RawMessage(`{"test":true}`))
	if err != nil {
		t.Fatalf("approve with valid token: %v", err)
	}
	if approved.Status != store.ProposalApproved {
		t.Fatalf("status = %s, want approved", approved.Status)
	}
	if _, err := st.DecideProposal(context.Background(), resp.Proposals[0].ProposalID, device.ID, store.ProposalRejected, token, fixedNow().Add(2*time.Minute), json.RawMessage(`{"test":true}`)); !errors.Is(err, store.ErrUsedApprovalToken) {
		t.Fatalf("replay err = %v, want used token", err)
	}

	expiring, err := service.HandleMessage(context.Background(), device, MessageRequest{SchemaVersion: SchemaVersion, Message: "Queue another task.", Context: planningContext()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DecideProposal(context.Background(), expiring.Proposals[0].ProposalID, device.ID, store.ProposalApproved, expiring.Proposals[0].DecisionToken, fixedNow().Add(20*time.Minute), json.RawMessage(`{"test":true}`)); !errors.Is(err, store.ErrExpiredApprovalToken) {
		t.Fatalf("expired err = %v, want expired token", err)
	}
}

func testStoreAndDevice(t *testing.T) (*store.Store, store.Device) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "zeitboardd.db"), bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.RegisterDevice(context.Background(), "device_test_01", "test", auth.HashToken("token"), fixedNow()); err != nil {
		t.Fatal(err)
	}
	return st, store.Device{ID: "device_test_01", Label: "test", CreatedAt: fixedNow()}
}

func planningContext() PlanningContext {
	zone := "America/New_York"
	now := fixedNow()
	earliest := time.Date(2026, 3, 5, 15, 0, 0, 0, time.UTC)
	latest := time.Date(2026, 3, 6, 1, 0, 0, 0, time.UTC)
	return PlanningContext{
		ZoneID:     zone,
		Now:        now,
		EstimateID: "estimate_synthetic_01",
		Tasks: []TaskContext{{
			TaskID:            "task_flexible_01",
			DurationMinutes:   30,
			EarliestStartAt:   &earliest,
			LatestFinishAt:    &latest,
			MinimumConfidence: "low",
		}},
		Availability: []AvailabilityContext{{
			Kind:       "predicted_wake",
			StartAt:    time.Date(2026, 3, 5, 14, 30, 0, 0, time.UTC),
			EndAt:      time.Date(2026, 3, 6, 2, 30, 0, 0, time.UTC),
			ZoneID:     zone,
			Confidence: "medium",
		}},
		FixedEvents: []FixedEventContext{{
			EventID: "event_fixed_01",
			StartAt: time.Date(2026, 3, 5, 16, 0, 0, 0, time.UTC),
			EndAt:   time.Date(2026, 3, 5, 17, 0, 0, 0, time.UTC),
			ZoneID:  zone,
		}},
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC)
}

func validateScheduleProposalsPayload(t *testing.T, value scheduleProposalsPayload) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	for _, name := range []string{"common.schema.json", "schedule-proposals.schema.json"} {
		data, err := os.ReadFile(filepath.Join(root, "contracts", "v1", name))
		if err != nil {
			t.Fatal(err)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if err := compiler.AddResource("mem:///contracts/v1/"+name, doc); err != nil {
			t.Fatal(err)
		}
	}
	schema, err := compiler.Compile("mem:///contracts/v1/schedule-proposals.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(doc); err != nil {
		t.Fatalf("schedule proposal payload did not validate: %v\n%s", err, data)
	}
}

// Regression: the post-provider guard used to be gated on the PROMPT looking
// medical, which made it unreachable (medical prompts already returned before
// the provider call). A benign-looking prompt whose answer smuggles a dosing
// directive must now be refused on the way out.
func TestAssistantScreensUnsolicitedDosingAdviceInModelAnswer(t *testing.T) {
	st, device := testStoreAndDevice(t)
	fake := &fakeProvider{responses: []provider.Response{{
		Text: `{"schema_version":"v1","recommended_action":"answer_only","answer":"Your afternoon looks open. Also, start taking 5 mg an hour before bed."}`,
	}}}
	service := New(fake, fake.Status(), st)
	service.now = fixedNow

	resp, err := service.HandleMessage(context.Background(), device, MessageRequest{
		SchemaVersion: SchemaVersion,
		Message:       "when is my next good window?",
		Context:       planningContext(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.requests) != 1 {
		t.Fatalf("expected the provider to be called for a non-medical prompt, got %d calls", len(fake.requests))
	}
	if resp.Result != ResultRefused || resp.Answer != agentpolicy.MedicalRefusal {
		t.Fatalf("unsolicited dosing advice was not screened: %+v", resp)
	}
	if len(resp.Proposals) != 0 {
		t.Fatalf("refused answer must create no proposal: %+v", resp.Proposals)
	}
}

// The same screening must not fire on ordinary scheduling language.
func TestAssistantAllowsOrdinarySchedulingAnswer(t *testing.T) {
	st, device := testStoreAndDevice(t)
	fake := &fakeProvider{responses: []provider.Response{{
		Text: `{"schema_version":"v1","recommended_action":"answer_only","answer":"You should be awake from about 2 PM. Take the 3 PM slot; it is the best time before your predicted sleep."}`,
	}}}
	service := New(fake, fake.Status(), st)
	service.now = fixedNow

	resp, err := service.HandleMessage(context.Background(), device, MessageRequest{
		SchemaVersion: SchemaVersion,
		Message:       "when is my next good window?",
		Context:       planningContext(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Result == ResultRefused {
		t.Fatalf("ordinary scheduling answer was falsely refused: %+v", resp)
	}
}
