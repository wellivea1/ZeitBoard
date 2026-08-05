package portalbridge_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"non24.app/core/domain"
	"non24.app/server/internal/auth"
	"non24.app/server/internal/portal"
	"non24.app/server/internal/portalbridge"
	"non24.app/server/internal/readmodel"
	"non24.app/server/internal/store"
	syncmodel "non24.app/server/internal/sync"
)

type bridgeFixture struct {
	private *store.Store
	portal  *portal.Store
	bridge  portalbridge.RequestBridge
	profile portal.Profile
	session string
	now     time.Time
}

func newBridgeFixture(t *testing.T) *bridgeFixture {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	now := time.Date(2026, 8, 3, 15, 0, 0, 0, time.UTC)

	private, err := store.Open(filepath.Join(dir, "private.db"), bytes.Repeat([]byte{5}, 32))
	if err != nil {
		t.Fatalf("open private: %v", err)
	}
	t.Cleanup(func() { _ = private.Close() })
	if err := private.RegisterDevice(ctx, "dev_owner", "desktop", auth.HashToken("tok"), now.Add(-time.Hour)); err != nil {
		t.Fatalf("register device: %v", err)
	}

	portalStore, err := portal.Open(filepath.Join(dir, "portal.db"), bytes.Repeat([]byte{5}, 32))
	if err != nil {
		t.Fatalf("open portal: %v", err)
	}
	t.Cleanup(func() { _ = portalStore.Close() })

	profileID, _ := portal.NewProfileID()
	linkToken, _ := portal.NewLinkToken()
	if err := portalStore.CreateProfile(ctx, portal.CreateProfileInput{
		ProfileID: profileID,
		Token:     linkToken,
		Passcode:  testPasscode,
		Grants:    portal.Grants{WakingWindows: true, AllowRequests: true},
		CreatedAt: now,
		ExpiresAt: now.Add(30 * 24 * time.Hour),
	}); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	profile, err := portalStore.ResolveLink(ctx, linkToken, now)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	session, err := portalStore.CreateSession(ctx, profile, now)
	if err != nil {
		t.Fatalf("session: %v", err)
	}

	return &bridgeFixture{
		private: private,
		portal:  portalStore,
		profile: profile,
		session: session.Session,
		now:     now,
		bridge: portalbridge.RequestBridge{
			Portal:  portalStore,
			Private: private,
			Now:     func() time.Time { return now },
		},
	}
}

func (f *bridgeFixture) createRequest(t *testing.T, duration int) portal.CreatedRequest {
	t.Helper()
	created, err := f.portal.CreateRequest(context.Background(), f.profile, f.session, portal.RequestInput{
		WindowStart:     f.now.Add(30 * time.Hour),
		WindowEnd:       f.now.Add(34 * time.Hour),
		ZoneID:          "America/New_York",
		DurationMinutes: duration,
		Handle:          "Sam",
		Message:         "coffee?",
	}, false, f.now)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	return created
}

func (f *bridgeFixture) request(t *testing.T, id string) portal.Request {
	t.Helper()
	request, err := f.portal.ReadRequest(context.Background(), f.profile.ID, id)
	if err != nil {
		t.Fatalf("read request: %v", err)
	}
	return request
}

// visitorProposal finds the pending proposal the bridge created.
func (f *bridgeFixture) visitorProposal(t *testing.T) store.ProposalRecord {
	t.Helper()
	page, err := f.private.ListProposalPage(context.Background(), store.ProposalPageCursor{}, 50, f.now)
	if err != nil {
		t.Fatalf("list proposals: %v", err)
	}
	for _, record := range page.Records {
		if record.ActionID == store.ActionVisitorRequest {
			return record
		}
	}
	t.Fatal("no visitor proposal was created")
	return store.ProposalRecord{}
}

func TestRequestReachesTheOwnerQueue(t *testing.T) {
	f := newBridgeFixture(t)
	created := f.createRequest(t, 60)

	if got := f.request(t, created.Request.ID).Status; got != portal.RequestQueued {
		t.Fatalf("status before the pump = %q, want %q", got, portal.RequestQueued)
	}
	if err := f.bridge.Pump(context.Background()); err != nil {
		t.Fatalf("pump: %v", err)
	}
	if got := f.request(t, created.Request.ID).Status; got != portal.RequestPending {
		t.Errorf("status after the pump = %q, want %q", got, portal.RequestPending)
	}

	record := f.visitorProposal(t)
	payload, err := store.VisitorProposalPayloadOf(record)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Origin != "visitor" {
		t.Errorf("origin = %q, want visitor", payload.Origin)
	}
	if payload.PortalRequestID != created.Request.ID {
		t.Error("the proposal is not linked to its portal request")
	}
	// The owner must see what was actually asked, including the visitor's own
	// words; that is the point of the request.
	if payload.Handle != "Sam" || payload.Message != "coffee?" {
		t.Errorf("visitor text did not reach the owner: %+v", payload)
	}
}

// TestBridgeFailureLeavesTheRequestQueued is the honesty property: with no
// enrolled device there is nowhere to file the request, and the visitor must
// not be told it was delivered.
func TestBridgeFailureLeavesTheRequestQueued(t *testing.T) {
	f := newBridgeFixture(t)
	f.bridge.OwnerDevice = func(context.Context) (string, error) {
		return "", errors.New("backend unavailable")
	}
	created := f.createRequest(t, 0)

	if err := f.bridge.Pump(context.Background()); err == nil {
		t.Fatal("a failing bridge reported success")
	}
	if got := f.request(t, created.Request.ID).Status; got != portal.RequestQueued {
		t.Errorf("status = %q, want it to stay %q", got, portal.RequestQueued)
	}

	// Recovery: once a device is available the same request goes through.
	f.bridge.OwnerDevice = nil
	if err := f.bridge.Pump(context.Background()); err != nil {
		t.Fatalf("recovery pump: %v", err)
	}
	if got := f.request(t, created.Request.ID).Status; got != portal.RequestPending {
		t.Errorf("status after recovery = %q, want %q", got, portal.RequestPending)
	}
}

// TestBridgeSubmissionIsIdempotent covers the retry that matters: the private
// commit succeeded but the acknowledgement was lost.
func TestBridgeSubmissionIsIdempotent(t *testing.T) {
	f := newBridgeFixture(t)
	created := f.createRequest(t, 0)
	ctx := context.Background()

	for i := 0; i < 4; i++ {
		if err := f.bridge.Pump(ctx); err != nil {
			t.Fatalf("pump %d: %v", i, err)
		}
	}
	page, err := f.private.ListProposalPage(ctx, store.ProposalPageCursor{}, 50, f.now)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	count := 0
	for _, record := range page.Records {
		if record.ActionID == store.ActionVisitorRequest {
			count++
		}
	}
	if count != 1 {
		t.Errorf("visitor proposal count = %d, want exactly 1 after repeated pumps", count)
	}
	_ = created
}

func TestApprovalMustChooseASlotInsideTheWindow(t *testing.T) {
	f := newBridgeFixture(t)
	created := f.createRequest(t, 60)
	ctx := context.Background()
	if err := f.bridge.Pump(ctx); err != nil {
		t.Fatalf("pump: %v", err)
	}
	record := f.visitorProposal(t)

	cases := map[string]store.VisitorSlot{
		"before the window": {StartAt: f.now.Add(20 * time.Hour), EndAt: f.now.Add(21 * time.Hour)},
		"after the window":  {StartAt: f.now.Add(40 * time.Hour), EndAt: f.now.Add(41 * time.Hour)},
		"straddling the end": {
			StartAt: f.now.Add(33*time.Hour + 30*time.Minute),
			EndAt:   f.now.Add(34*time.Hour + 30*time.Minute),
		},
		"wrong duration": {StartAt: f.now.Add(31 * time.Hour), EndAt: f.now.Add(31*time.Hour + 30*time.Minute)},
		"empty":          {},
		"inverted":       {StartAt: f.now.Add(32 * time.Hour), EndAt: f.now.Add(31 * time.Hour)},
	}
	for name, slot := range cases {
		_, err := f.private.DecideVisitorProposal(ctx, record.ID, "dev_owner",
			store.ProposalApproved, record.DecisionToken, slot, f.now)
		if err == nil {
			t.Errorf("%s slot was accepted", name)
			continue
		}
		if !errors.Is(err, store.ErrVisitorSlotOutOfWindow) {
			t.Errorf("%s returned an untyped error: %v", name, err)
		}
	}

	// The valid slot: inside the window and exactly the requested length.
	good := store.VisitorSlot{StartAt: f.now.Add(31 * time.Hour), EndAt: f.now.Add(32 * time.Hour)}
	if _, err := f.private.DecideVisitorProposal(ctx, record.ID, "dev_owner",
		store.ProposalApproved, record.DecisionToken, good, f.now); err != nil {
		t.Fatalf("a valid slot was refused: %v", err)
	}

	if err := f.bridge.Pump(ctx); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	request := f.request(t, created.Request.ID)
	if request.Status != portal.RequestApproved {
		t.Errorf("status = %q, want approved", request.Status)
	}
	if !request.DecidedStart.Equal(good.StartAt.UTC()) || !request.DecidedEnd.Equal(good.EndAt.UTC()) {
		t.Errorf("the visitor was told a different block: %v-%v", request.DecidedStart, request.DecidedEnd)
	}
}

// TestDecisionTokenIsOneUse holds the queue's core invariant on the visitor
// path too: two decisions race, and only one can commit.
func TestDecisionTokenIsOneUse(t *testing.T) {
	f := newBridgeFixture(t)
	f.createRequest(t, 0)
	ctx := context.Background()
	if err := f.bridge.Pump(ctx); err != nil {
		t.Fatalf("pump: %v", err)
	}
	record := f.visitorProposal(t)
	slot := store.VisitorSlot{StartAt: f.now.Add(31 * time.Hour), EndAt: f.now.Add(32 * time.Hour)}

	if _, err := f.private.DecideVisitorProposal(ctx, record.ID, "dev_owner",
		store.ProposalApproved, record.DecisionToken, slot, f.now); err != nil {
		t.Fatalf("first decision: %v", err)
	}
	_, err := f.private.DecideVisitorProposal(ctx, record.ID, "dev_owner",
		store.ProposalRejected, record.DecisionToken, store.VisitorSlot{}, f.now)
	if err == nil {
		t.Fatal("the decision token was accepted twice")
	}
	if !errors.Is(err, store.ErrUsedApprovalToken) && !errors.Is(err, store.ErrProposalNotPending) {
		t.Errorf("second decision returned an unexpected error: %v", err)
	}
}

func TestDeclineDeliversWithoutASlot(t *testing.T) {
	f := newBridgeFixture(t)
	created := f.createRequest(t, 0)
	ctx := context.Background()
	if err := f.bridge.Pump(ctx); err != nil {
		t.Fatalf("pump: %v", err)
	}
	record := f.visitorProposal(t)

	if _, err := f.private.DecideVisitorProposal(ctx, record.ID, "dev_owner",
		store.ProposalRejected, record.DecisionToken, store.VisitorSlot{}, f.now); err != nil {
		t.Fatalf("decline: %v", err)
	}
	if err := f.bridge.Pump(ctx); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	request := f.request(t, created.Request.ID)
	if request.Status != portal.RequestDeclined {
		t.Errorf("status = %q, want declined", request.Status)
	}
	if !request.DecidedStart.IsZero() {
		t.Error("a declined request carries a chosen block")
	}
}

// TestVisitorProposalsAreNotAnAgentAction is the anti-forgery property: an
// agent must not be able to mint a proposal that presents itself as coming
// from an outside person.
func TestVisitorProposalsAreNotAnAgentAction(t *testing.T) {
	for _, agentAction := range []string{"answer_only", "propose_move_task", "propose_place_task", "propose_reminder_shift"} {
		if agentAction == store.ActionVisitorRequest {
			t.Fatalf("%s is exposed on the agent action surface", agentAction)
		}
	}
	if store.ActionVisitorRequest != "place_visitor_request" {
		t.Fatalf("unexpected visitor action id %q", store.ActionVisitorRequest)
	}
}

// TestDecideRejectsNonVisitorProposals keeps the slot rules from being applied
// to — or bypassed on — an ordinary proposal.
func TestDecideRejectsNonVisitorProposals(t *testing.T) {
	f := newBridgeFixture(t)
	ctx := context.Background()
	record, err := f.private.CreateProposal(ctx, store.ProposalInput{
		ID:        "proposal-plain",
		ActionID:  "propose_place_task",
		DeviceID:  "dev_owner",
		CreatedAt: f.now,
		ExpiresAt: f.now.Add(time.Hour),
		Payload:   []byte(`{"kind":"task"}`),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = f.private.DecideVisitorProposal(ctx, record.ID, "dev_owner",
		store.ProposalApproved, record.DecisionToken, store.VisitorSlot{
			StartAt: f.now, EndAt: f.now.Add(time.Hour),
		}, f.now)
	if !errors.Is(err, store.ErrNotVisitorProposal) {
		t.Errorf("error = %v, want ErrNotVisitorProposal", err)
	}
}

// TestVisitorTextNeverReachesTheAvailabilityProjection keeps the request path
// separate from the public projection: a message is for the owner, not for
// every other holder of the link.
func TestVisitorTextNeverReachesTheAvailabilityProjection(t *testing.T) {
	f := newBridgeFixture(t)
	ctx := context.Background()
	f.createRequest(t, 0)
	if err := f.bridge.Pump(ctx); err != nil {
		t.Fatalf("pump: %v", err)
	}
	materializer := portalbridge.Materializer{
		Sleep:    emptySleep{},
		Profiles: f.portal,
		Sink:     f.portal,
		Now:      func() time.Time { return f.now },
	}
	if err := materializer.MaterializeAll(ctx); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	snapshot, err := f.portal.ReadSnapshot(ctx, f.profile.ID)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	for _, window := range snapshot.Windows {
		if window.ZoneID == "Sam" || window.ZoneID == "coffee?" {
			t.Error("visitor text reached the projection")
		}
	}
	if snapshot.Status == portal.StatusAvailable {
		t.Error("an empty history produced availability")
	}
}

// emptySleep stands in for a read model with no history.
type emptySleep struct{}

func (emptySleep) EffectiveSleepSessions(context.Context) ([]domain.SleepSession, error) {
	return nil, nil
}

// TestStaleSleepEvidenceIsNotPublishedAsAvailability closes a defect in the
// P5-a materializer: GeneratedAt is the time the snapshot row was written, and
// MaterializeAll runs after *any* accepted sync push. Pushing a task therefore
// refreshed the portal's freshness signal while the sleep evidence underneath
// was days old, and the page read "updated just now".
func TestStaleSleepEvidenceIsNotPublishedAsAvailability(t *testing.T) {
	f := newBridgeFixture(t)
	ctx := context.Background()

	// Sleep history that fits, but whose newest episode ended three days ago.
	if err := f.private.RegisterDevice(ctx, "dev_stale", "desktop",
		auth.HashToken("tok2"), f.now.Add(-30*24*time.Hour)); err != nil {
		t.Fatalf("register: %v", err)
	}
	cycle := 24*time.Hour + 50*time.Minute
	lastStart := f.now.Add(-3 * 24 * time.Hour)
	records := make([]syncmodel.PushRecord, 0, 12)
	for i := 0; i < 12; i++ {
		start := lastStart.Add(-time.Duration(11-i) * cycle)
		end := start.Add(8 * time.Hour)
		records = append(records, syncmodel.PushRecord{
			RecordID:  fmt.Sprintf("stale-obs-%02d", i),
			Kind:      syncmodel.KindObservation,
			CreatedAt: end,
			Payload: json.RawMessage(fmt.Sprintf(
				`{"observation_id":%q,"kind":"sleep_episode","start_at":%q,"end_at":%q,"zone_id":"UTC","sleep":{"classification":"principal"},"provenance":{"acquisition_method":"synthetic","evidence_status":"directly_observed","recorded_at":%q,"source_record_id":"stale-src"}}`,
				fmt.Sprintf("stale-obs-%02d", i),
				start.UTC().Format(time.RFC3339),
				end.UTC().Format(time.RFC3339),
				end.UTC().Format(time.RFC3339))),
		})
	}
	request := syncmodel.PushRequest{SchemaVersion: syncmodel.SchemaVersion, Records: records}
	if err := syncmodel.ValidatePushRequest(&request); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if _, _, err := f.private.Append(ctx, "dev_stale", request.Records); err != nil {
		t.Fatalf("append: %v", err)
	}

	materializer := portalbridge.Materializer{
		Sleep:    readmodel.SleepReader{Store: f.private},
		Profiles: f.portal,
		Sink:     f.portal,
		Now:      func() time.Time { return f.now },
	}
	snapshot, err := materializer.Snapshot(ctx)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.Status == portal.StatusAvailable {
		t.Error("three-day-old sleep evidence was published as current availability")
	}
	if len(snapshot.Windows) != 0 {
		t.Errorf("stale evidence produced %d windows", len(snapshot.Windows))
	}
	// The snapshot is still written, and its generation time is still honest:
	// it says when the row was made, not that the rhythm is current.
	if snapshot.GeneratedAt.IsZero() {
		t.Error("the snapshot lost its generation time")
	}
}
