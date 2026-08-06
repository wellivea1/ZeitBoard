package analysis_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"non24.app/core/recompute"
	"non24.app/server/internal/analysis"
	"non24.app/server/internal/auth"
	"non24.app/server/internal/portal"
	"non24.app/server/internal/portalbridge"
	"non24.app/server/internal/readmodel"
	"non24.app/server/internal/store"
	syncmodel "non24.app/server/internal/sync"
)

// The rhythm the fixture records: twelve nights drifting fifty minutes a cycle,
// the newest ending shortly before `now`. That is a history the estimator
// accepts and the freshness policy calls current.
const (
	fixtureCycle    = 24*time.Hour + 50*time.Minute
	fixtureEpisodes = 12
	fixtureDuration = 8 * time.Hour
)

type fixture struct {
	private   *store.Store
	portal    *portal.Store
	clock     time.Time
	profileID string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	now := time.Date(2026, 8, 6, 18, 0, 0, 0, time.UTC)

	private, err := store.Open(filepath.Join(dir, "private.db"), bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatalf("open private store: %v", err)
	}
	t.Cleanup(func() { _ = private.Close() })
	if err := private.RegisterDevice(ctx, "dev_owner", "desktop",
		auth.HashToken("tok"), now.Add(-40*24*time.Hour)); err != nil {
		t.Fatalf("register device: %v", err)
	}

	portalStore, err := portal.Open(filepath.Join(dir, "portal.db"), bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatalf("open portal store: %v", err)
	}
	t.Cleanup(func() { _ = portalStore.Close() })

	f := &fixture{private: private, portal: portalStore, clock: now}
	f.profileID = f.addProfile(t, "first")
	f.recordHistory(t, now.Add(-2*time.Hour))
	return f
}

// addProfile creates a share link and returns its opaque profile id.
func (f *fixture) addProfile(t *testing.T, label string) string {
	t.Helper()
	ctx := context.Background()
	profileID, err := portal.NewProfileID()
	if err != nil {
		t.Fatalf("%s: profile id: %v", label, err)
	}
	token, err := portal.NewLinkToken()
	if err != nil {
		t.Fatalf("%s: link token: %v", label, err)
	}
	if err := f.portal.CreateProfile(ctx, portal.CreateProfileInput{
		ProfileID: profileID,
		Token:     token,
		Passcode:  "open-sesame-please",
		Grants:    portal.Grants{WakingWindows: true},
		CreatedAt: f.clock.Add(-time.Hour),
		ExpiresAt: f.clock.Add(60 * 24 * time.Hour),
	}); err != nil {
		t.Fatalf("%s: create profile: %v", label, err)
	}
	return profileID
}

// recordHistory pushes a drifting sleep history whose newest episode ends at
// `lastEnd`, recorded as it happened.
func (f *fixture) recordHistory(t *testing.T, lastEnd time.Time) {
	t.Helper()
	records := make([]syncmodel.PushRecord, 0, fixtureEpisodes)
	for i := 0; i < fixtureEpisodes; i++ {
		end := lastEnd.Add(-time.Duration(fixtureEpisodes-1-i) * fixtureCycle)
		start := end.Add(-fixtureDuration)
		id := fmt.Sprintf("obs-%02d", i)
		records = append(records, syncmodel.PushRecord{
			RecordID:  id,
			Kind:      syncmodel.KindObservation,
			CreatedAt: end,
			Payload: json.RawMessage(fmt.Sprintf(
				`{"observation_id":%q,"kind":"sleep_episode","start_at":%q,"end_at":%q,`+
					`"zone_id":"UTC","sleep":{"classification":"principal"},`+
					`"provenance":{"acquisition_method":"synthetic","evidence_status":"directly_observed",`+
					`"recorded_at":%q,"source_record_id":%q}}`,
				id,
				start.UTC().Format(time.RFC3339),
				end.UTC().Format(time.RFC3339),
				end.UTC().Format(time.RFC3339),
				id)),
		})
	}
	request := syncmodel.PushRequest{SchemaVersion: syncmodel.SchemaVersion, Records: records}
	if err := syncmodel.ValidatePushRequest(&request); err != nil {
		t.Fatalf("fixture history is not a valid batch: %v", err)
	}
	if _, _, err := f.private.Append(context.Background(), "dev_owner", request.Records); err != nil {
		t.Fatalf("append history: %v", err)
	}
}

func (f *fixture) materializer() portalbridge.Materializer {
	return portalbridge.Materializer{
		Sleep:    readmodel.SleepReader{Store: f.private},
		Profiles: f.portal,
		Sink:     f.portal,
	}
}

func (f *fixture) orchestrator() recompute.Orchestrator {
	return recompute.Orchestrator{
		Analysis: analysis.Portal{Materializer: f.materializer()},
		Journal:  store.RecomputeJournal{Store: f.private},
	}
}

func (f *fixture) snapshot(t *testing.T, profileID string) portal.Snapshot {
	t.Helper()
	snapshot, err := f.portal.ReadSnapshot(context.Background(), profileID)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	return snapshot
}

// TestAQuietDayWithholdsAvailabilityWithNobodyPushing is the slice's whole
// reason for existing.
//
// ADR-0031 moved the freshness decision to materialization, which made it
// correct and made it conditional: the decision now only happens when something
// else causes a materialization. A user who records nothing causes nothing, so
// the page kept publishing yesterday's windows. Here nothing is pushed, nothing
// is opened, and nobody asks — only the clock moves, and the claim is withdrawn.
func TestAQuietDayWithholdsAvailabilityWithNobodyPushing(t *testing.T) {
	f := newFixture(t)
	orchestrator := f.orchestrator()
	ctx := context.Background()

	first, err := orchestrator.Run(ctx, recompute.ReasonEvidence, f.clock)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if got := f.snapshot(t, f.profileID).Status; got != portal.StatusAvailable {
		t.Fatalf("status = %q, want the history to produce availability", got)
	}
	if first.ValidUntil.IsZero() {
		t.Fatal("nothing was scheduled to expire; the claim would stand forever")
	}
	if !first.ValidUntil.After(f.clock) {
		t.Fatalf("expiry %s is not in the future", first.ValidUntil)
	}

	// Walk to each expiry in turn, exactly as the worker would, pushing nothing.
	at := first.ValidUntil
	outcome := first
	for i := 0; i < 10; i++ {
		outcome, err = orchestrator.Run(ctx, recompute.ReasonFreshnessExpiry, at)
		if err != nil {
			t.Fatalf("expiry run %d: %v", i, err)
		}
		if f.snapshot(t, f.profileID).Status != portal.StatusAvailable {
			break
		}
		if outcome.ValidUntil.IsZero() || !outcome.ValidUntil.After(at) {
			t.Fatalf("run %d left availability published with no further expiry", i)
		}
		at = outcome.ValidUntil
	}

	final := f.snapshot(t, f.profileID)
	if final.Status == portal.StatusAvailable {
		t.Errorf("availability was still published at %s with no evidence since %s",
			at, f.clock.Add(-2*time.Hour))
	}
	if len(final.Windows) != 0 {
		t.Errorf("%d windows survived the withholding", len(final.Windows))
	}
	if at.Sub(f.clock) > 24*time.Hour {
		t.Errorf("the claim took %s to be withdrawn, longer than the policy's own ceiling", at.Sub(f.clock))
	}
}

// TestAnUnchangedProjectionDoesNotLookFresher. The rendered page and the JSON
// DTO both key their staleness on how old the snapshot is, so a restamp on a
// run that changed nothing would silently reset the visitor's only warning.
func TestAnUnchangedProjectionDoesNotLookFresher(t *testing.T) {
	f := newFixture(t)
	orchestrator := f.orchestrator()
	ctx := context.Background()

	if _, err := orchestrator.Run(ctx, recompute.ReasonEvidence, f.clock); err != nil {
		t.Fatalf("first run: %v", err)
	}
	firstStamp := f.snapshot(t, f.profileID).GeneratedAt

	for _, delay := range []time.Duration{time.Minute, time.Hour, 3 * time.Hour} {
		if _, err := orchestrator.Run(ctx, recompute.ReasonHeartbeat, f.clock.Add(delay)); err != nil {
			t.Fatalf("run at +%s: %v", delay, err)
		}
		if got := f.snapshot(t, f.profileID).GeneratedAt; !got.Equal(firstStamp) {
			t.Errorf("at +%s the stamp moved to %s; nothing had changed", delay, got)
		}
	}
}

// TestNewEvidenceMovesTheStamp is the other half: the mechanism must not be so
// conservative that a real change goes unannounced.
func TestNewEvidenceMovesTheStamp(t *testing.T) {
	f := newFixture(t)
	orchestrator := f.orchestrator()
	ctx := context.Background()

	if _, err := orchestrator.Run(ctx, recompute.ReasonEvidence, f.clock); err != nil {
		t.Fatalf("first run: %v", err)
	}
	before := f.snapshot(t, f.profileID).GeneratedAt

	// One more night, recorded an hour later.
	later := f.clock.Add(time.Hour)
	end := f.clock.Add(-2 * time.Hour).Add(fixtureCycle)
	start := end.Add(-fixtureDuration)
	record := syncmodel.PushRecord{
		RecordID:  "obs-new",
		Kind:      syncmodel.KindObservation,
		CreatedAt: later,
		Payload: json.RawMessage(fmt.Sprintf(
			`{"observation_id":"obs-new","kind":"sleep_episode","start_at":%q,"end_at":%q,`+
				`"zone_id":"UTC","sleep":{"classification":"principal"},`+
				`"provenance":{"acquisition_method":"synthetic","evidence_status":"directly_observed",`+
				`"recorded_at":%q,"source_record_id":"obs-new"}}`,
			start.UTC().Format(time.RFC3339),
			end.UTC().Format(time.RFC3339),
			later.UTC().Format(time.RFC3339))),
	}
	if _, _, err := f.private.Append(ctx, "dev_owner", []syncmodel.PushRecord{record}); err != nil {
		t.Fatalf("append new night: %v", err)
	}

	outcome, err := orchestrator.Run(ctx, recompute.ReasonEvidence, later)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if !outcome.InputsChanged {
		t.Error("a new sleep record did not change the input fingerprint")
	}
	if !outcome.ContentChanged {
		t.Error("a new night did not change what would be published")
	}
	if got := f.snapshot(t, f.profileID).GeneratedAt; !got.After(before) {
		t.Errorf("stamp = %s, want it to advance past %s", got, before)
	}
}

// TestANewLinkDoesNotRestampTheOthers. Creating a link for one person must not
// make everybody else's page claim to have been updated, which is why the
// consumer set is part of the input fingerprint and not of the content one.
func TestANewLinkDoesNotRestampTheOthers(t *testing.T) {
	f := newFixture(t)
	orchestrator := f.orchestrator()
	ctx := context.Background()

	if _, err := orchestrator.Run(ctx, recompute.ReasonEvidence, f.clock); err != nil {
		t.Fatalf("first run: %v", err)
	}
	before := f.snapshot(t, f.profileID).GeneratedAt

	second := f.addProfile(t, "second")
	outcome, err := orchestrator.Run(ctx, recompute.ReasonSharing, f.clock.Add(20*time.Minute))
	if err != nil {
		t.Fatalf("sharing run: %v", err)
	}
	if !outcome.InputsChanged {
		t.Error("a new consumer did not change the input fingerprint; its page would never be written")
	}
	if outcome.ContentChanged {
		t.Error("adding a link was treated as a change to the availability itself")
	}

	if got := f.snapshot(t, f.profileID).GeneratedAt; !got.Equal(before) {
		t.Errorf("the existing link was restamped to %s by somebody else's link", got)
	}
	fresh := f.snapshot(t, second)
	if fresh.Status != portal.StatusAvailable {
		t.Errorf("the new link's status = %q, want the same availability as the first", fresh.Status)
	}
	if !fresh.GeneratedAt.Equal(before) {
		t.Errorf("the new link's stamp = %s, want the honest %s the estimate actually dates from",
			fresh.GeneratedAt, before)
	}
}

// TestPublishedWindowsDoNotDependOnTheInstant. The snapshot has to be stable for
// the content fingerprint to mean anything; windows used to be filtered and
// clipped against `now` as they were written, which made every run look like a
// change. Both rules now run at render.
func TestPublishedWindowsDoNotDependOnTheInstant(t *testing.T) {
	f := newFixture(t)
	materializer := f.materializer()
	ctx := context.Background()

	early, err := materializer.Prepare(ctx, f.clock)
	if err != nil {
		t.Fatalf("prepare early: %v", err)
	}
	late, err := materializer.Prepare(ctx, f.clock.Add(90*time.Minute))
	if err != nil {
		t.Fatalf("prepare late: %v", err)
	}
	if len(early.Snapshot.Windows) == 0 {
		t.Fatal("the fixture history produced no windows")
	}
	if len(early.Snapshot.Windows) != len(late.Snapshot.Windows) {
		t.Fatalf("window count changed with the clock: %d then %d",
			len(early.Snapshot.Windows), len(late.Snapshot.Windows))
	}
	for i := range early.Snapshot.Windows {
		if !early.Snapshot.Windows[i].StartAt.Equal(late.Snapshot.Windows[i].StartAt) {
			t.Errorf("window %d start moved from %s to %s with the clock alone",
				i, early.Snapshot.Windows[i].StartAt, late.Snapshot.Windows[i].StartAt)
		}
	}
}

// countingSink counts publications so a burst can be shown to produce one. The
// counter is atomic because the test reads it while the worker goroutine is
// still writing to it.
type countingSink struct {
	inner  portalbridge.SnapshotSink
	writes atomic.Int64
}

func (c *countingSink) PublishSnapshot(ctx context.Context, profileID string, snapshot portal.Snapshot) error {
	c.writes.Add(1)
	return c.inner.PublishSnapshot(ctx, profileID, snapshot)
}

// TestABurstOfPushesPublishesOnce runs the worker for real — goroutine, timer
// and all — because the coalescing that matters is the one that ships.
func TestABurstOfPushesPublishesOnce(t *testing.T) {
	f := newFixture(t)
	sink := &countingSink{inner: f.portal}
	materializer := f.materializer()
	materializer.Sink = sink

	worker := &analysis.Worker{
		Orchestrator: recompute.Orchestrator{
			Analysis: analysis.Portal{Materializer: materializer},
			Journal:  store.RecomputeJournal{Store: f.private},
		},
		Schedule: recompute.Schedule{
			Debounce:    40 * time.Millisecond,
			MinInterval: time.Millisecond,
			RetryBase:   time.Second,
			MaxBackoff:  time.Second,
			Heartbeat:   time.Hour,
		},
		Logf: func(string, ...any) {},
	}

	stop := make(chan struct{})
	done := worker.Start(stop)

	// Let the startup reconciliation land before measuring the burst.
	deadline := time.Now().Add(5 * time.Second)
	for sink.writes.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	afterStartup := sink.writes.Load()
	if afterStartup == 0 {
		close(stop)
		<-done
		t.Fatal("the worker never performed its startup reconciliation")
	}

	for i := 0; i < 40; i++ {
		worker.Request(recompute.ReasonEvidence)
	}
	time.Sleep(400 * time.Millisecond)

	close(stop)
	<-done

	burstWrites := sink.writes.Load() - afterStartup
	if burstWrites == 0 {
		t.Fatal("a burst of 40 pushes produced no recomputation at all")
	}
	// One publication per profile per run, and there is one profile.
	if burstWrites > 2 {
		t.Errorf("40 pushes produced %d publications; the burst was not coalesced", burstWrites)
	}
}

// expiringAnalysis reports a result that goes out of date shortly after it is
// computed, and counts how often it is asked again.
type expiringAnalysis struct {
	after time.Duration
	runs  chan struct{}
}

func (e *expiringAnalysis) Prepare(_ context.Context, now time.Time) (recompute.Prepared, error) {
	select {
	case e.runs <- struct{}{}:
	default:
	}
	return recompute.Prepared{
		Inputs:     "steady",
		Content:    "steady",
		ValidUntil: now.Add(e.after),
		Apply:      func(context.Context, recompute.Stamp) error { return nil },
	}, nil
}

// TestTheLoopWakesForAnExpiryWithNoRequest is the wiring the quiet-day test
// proves in principle: Prepared.ValidUntil has to reach the schedule, or the
// worker sleeps through the moment the answer stopped being true.
func TestTheLoopWakesForAnExpiryWithNoRequest(t *testing.T) {
	f := newFixture(t)
	analysisStub := &expiringAnalysis{after: 60 * time.Millisecond, runs: make(chan struct{}, 8)}
	worker := &analysis.Worker{
		Orchestrator: recompute.Orchestrator{
			Analysis: analysisStub,
			Journal:  store.RecomputeJournal{Store: f.private},
		},
		Schedule: recompute.Schedule{
			Debounce:    time.Millisecond,
			MinInterval: time.Millisecond,
			RetryBase:   time.Second,
			MaxBackoff:  time.Second,
			Heartbeat:   time.Hour,
		},
		Logf: func(string, ...any) {},
	}

	stop := make(chan struct{})
	done := worker.Start(stop)
	defer func() {
		close(stop)
		<-done
	}()

	// The startup pass, then two more that nothing asked for.
	for i := 0; i < 3; i++ {
		select {
		case <-analysisStub.runs:
		case <-time.After(3 * time.Second):
			t.Fatalf("run %d never happened; the loop does not wake for an expiry", i+1)
		}
	}
}

// TestRunNowPublishesBeforeItReturns covers the one path that waits: a share
// link must not be handed out pointing at a page that has nothing on it yet.
func TestRunNowPublishesBeforeItReturns(t *testing.T) {
	f := newFixture(t)
	worker := &analysis.Worker{Orchestrator: f.orchestrator(), Now: func() time.Time { return f.clock }}

	if err := worker.RunNow(context.Background(), recompute.ReasonSharing); err != nil {
		t.Fatalf("run now: %v", err)
	}
	if got := f.snapshot(t, f.profileID).Status; got != portal.StatusAvailable {
		t.Errorf("status = %q immediately after RunNow, want the projection to exist", got)
	}
}
