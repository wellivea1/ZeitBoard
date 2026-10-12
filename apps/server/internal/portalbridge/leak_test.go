package portalbridge_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"non24.app/core/medication"
	"non24.app/server/internal/auth"
	"non24.app/server/internal/portal"
	"non24.app/server/internal/portalbridge"
	"non24.app/server/internal/readmodel"
	"non24.app/server/internal/store"
	syncmodel "non24.app/server/internal/sync"
)

const (
	testOrigin   = "https://share.example.test"
	testPasscode = "open-sesame-please"
)

// materializeAll computes and publishes in one step at a fixed instant.
//
// Production does not have this shortcut: it publishes through the recompute
// orchestrator (ADR-0033), which stamps the snapshot by content rather than by
// clock so an unchanged projection cannot present itself as a fresh one. These
// tests care about the value that crosses the boundary, not about the stamp.
func materializeAll(ctx context.Context, t *testing.T, m portalbridge.Materializer, now time.Time) {
	t.Helper()
	prep, err := m.Prepare(ctx, now)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if err := m.Publish(ctx, prep); err != nil {
		t.Fatalf("publish: %v", err)
	}
}

// canaries are distinctive values planted in every private field that could
// plausibly be carried along by a projection bug. Any appearance of one on the
// public surface, or anywhere in the portal database, is a leak.
// Record and observation identifiers must satisfy the sync identifier pattern
// (lowercase, hyphen or underscore), so the canaries are written in that shape
// rather than in shouting caps.
var canaries = map[string]string{
	"device label":     "canary-device-a41f",
	"observation id":   "canary-observation-b72c",
	"source record id": "canary-source-c93d",
	"task title":       "canary-task-title-d04e",
	"share label":      "canary-share-label-e15f",
	"correction id":    "canary-correction-f260",
	// A skipped dose may cross to a link that shows them, by time alone
	// (ADR-0058); nothing else about it, or about the medication, may.
	"medication id":    "canary-medication-a371",
	"medication label": "canary-medication-label-b482",
	"dose id":          "canary-dose-c593",
	"dose note":        "canary-dose-note-d6a4",
}

type leakFixture struct {
	private *store.Store
	portal  *portal.Store
	handler http.Handler
	dbPath  string
	token   string
	now     time.Time
}

func newLeakFixture(t *testing.T) *leakFixture {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	now := time.Date(2026, 8, 3, 15, 0, 0, 0, time.UTC)

	private, err := store.Open(filepath.Join(dir, "zeitboardd.db"), bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatalf("open private store: %v", err)
	}
	t.Cleanup(func() { _ = private.Close() })

	if err := private.RegisterDevice(ctx, "dev_canary", canaries["device label"], auth.HashToken("token"), now.Add(-90*24*time.Hour)); err != nil {
		t.Fatalf("register device: %v", err)
	}

	// A rhythm the estimator can actually fit: twelve principal episodes
	// drifting about 50 minutes per cycle, built backwards from now so the
	// newest one ended three hours ago. Anchoring them in the past instead
	// would make the fixture describe someone who has not slept in days,
	// which the shared freshness policy correctly refuses to publish.
	records := make([]syncmodel.PushRecord, 0, 14)
	cycle := 24*time.Hour + 50*time.Minute
	lastStart := now.Add(-11 * time.Hour)
	base := lastStart.Add(-11 * cycle)
	for i := 0; i < 12; i++ {
		start := base.Add(time.Duration(i) * cycle)
		id := fmt.Sprintf("obs-%02d", i)
		if i == 0 {
			id = canaries["observation id"]
		}
		records = append(records, syncmodel.PushRecord{
			RecordID:  id,
			Kind:      syncmodel.KindObservation,
			CreatedAt: now.Add(-time.Hour),
			Payload:   sleepObservationPayload(id, start, start.Add(8*time.Hour)),
		})
	}
	records = append(records, syncmodel.PushRecord{
		RecordID:  canaries["correction id"],
		Kind:      syncmodel.KindCorrection,
		CreatedAt: now.Add(-time.Hour),
		Payload: json.RawMessage(fmt.Sprintf(
			`{"correction_id":%q,"target_observation_id":%q,"created_at":%q,"reason":"user_edit","changes":{"start_at":%q}}`,
			canaries["correction id"], canaries["observation id"],
			base.Add(20*time.Minute).UTC().Format(time.RFC3339),
			base.Add(10*time.Minute).UTC().Format(time.RFC3339))),
	})
	// A task carries free user text, which is the most obviously private
	// thing a projection bug could pick up.
	records = append(records, syncmodel.PushRecord{
		RecordID:  "canary-task-d04e_r1",
		Kind:      syncmodel.KindTask,
		CreatedAt: now.Add(-time.Hour),
		Payload: json.RawMessage(fmt.Sprintf(
			`{"task_id":"canary-task-d04e","revision":1,"title":%q,"duration_minutes":30,"status":"open","created_at":%q,"updated_at":%q}`,
			canaries["task title"],
			now.Add(-2*time.Hour).UTC().Format(time.RFC3339),
			now.Add(-2*time.Hour).UTC().Format(time.RFC3339))),
	})

	records = append(records, medicationRecords(now)...)

	request := syncmodel.PushRequest{SchemaVersion: syncmodel.SchemaVersion, Records: records}
	if err := syncmodel.ValidatePushRequest(&request); err != nil {
		t.Fatalf("validate push: %v", err)
	}
	if _, _, err := private.Append(ctx, "dev_canary", request.Records); err != nil {
		t.Fatalf("append records: %v", err)
	}

	portalStore, err := portal.Open(filepath.Join(dir, "zeitboard-portal.db"), bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatalf("open portal store: %v", err)
	}
	t.Cleanup(func() { _ = portalStore.Close() })

	profileID, _ := portal.NewProfileID()
	token, _ := portal.NewLinkToken()
	if err := portalStore.CreateProfile(ctx, portal.CreateProfileInput{
		ProfileID: profileID,
		Token:     token,
		Passcode:  testPasscode,
		Grants:    portal.Grants{WakingWindows: true, SkippedDoses: true},
		CreatedAt: now,
		ExpiresAt: now.Add(30 * 24 * time.Hour),
	}); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := private.PutPortalLabel(ctx, profileID, canaries["share label"], now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("put label: %v", err)
	}

	materializer := portalbridge.Materializer{
		Sleep:    readmodel.SleepReader{Store: private},
		Doses:    readmodel.MedicationReader{Store: private},
		Profiles: portalStore,
		Sink:     portalStore,
		Now:      func() time.Time { return now },
	}
	materializeAll(ctx, t, materializer, now)
	snapshot, err := portalStore.ReadSnapshot(ctx, profileID)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if snapshot.Status != portal.StatusAvailable || len(snapshot.Windows) == 0 {
		t.Fatalf("fixture did not produce availability: status=%q windows=%d", snapshot.Status, len(snapshot.Windows))
	}

	handler, err := portal.NewHandler(portal.HandlerConfig{
		Store:           portalStore,
		Now:             func() time.Time { return now },
		PublicOrigin:    testOrigin,
		ResolutionFloor: time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}

	return &leakFixture{
		private: private,
		portal:  portalStore,
		handler: handler.Routes(),
		dbPath:  filepath.Join(dir, "zeitboard-portal.db"),
		token:   token,
		now:     now,
	}
}

// skippedDoseAt is the one dose the fixture logs as skipped and leaves so.
func skippedDoseAt(now time.Time) time.Time {
	return now.Add(-5 * time.Hour).Truncate(time.Minute)
}

// medicationRecords are a medication with a name and four doses: one logged
// as skipped, with a note; one taken; one skipped and then corrected to
// taken; and one skipped four days ago, past what a link shows.
func medicationRecords(now time.Time) []syncmodel.PushRecord {
	medicationID := canaries["medication id"]
	stamp := func(at time.Time) string { return at.UTC().Format(time.RFC3339) }
	dose := func(id string, at time.Time, status, note string) syncmodel.PushRecord {
		notePart := ""
		if note != "" {
			notePart = fmt.Sprintf(`,"note":%q`, note)
		}
		return syncmodel.PushRecord{
			RecordID: id, Kind: syncmodel.KindMedicationEvent, CreatedAt: now.Add(-time.Hour),
			Payload: json.RawMessage(fmt.Sprintf(
				`{"event_id":%q,"medication_id":%q,"dose_at":%q,"zone_id":"America/New_York","status":%q,"scheduled":true%s,"provenance":{"acquisition_method":"manual","evidence_status":"user_reported","recorded_at":%q}}`,
				id, medicationID, stamp(at), status, notePart, stamp(at))),
		}
	}
	return []syncmodel.PushRecord{
		{
			RecordID: syncmodel.MedicationRevisionID(medicationID, 1), Kind: syncmodel.KindMedication, CreatedAt: now.Add(-48 * time.Hour),
			Payload: json.RawMessage(fmt.Sprintf(
				`{"medication_id":%q,"label":%q,"active":true,"created_at":%q,"revision":1,"updated_at":%q}`,
				medicationID, canaries["medication label"], stamp(now.Add(-48*time.Hour)), stamp(now.Add(-48*time.Hour)))),
		},
		dose(canaries["dose id"], skippedDoseAt(now), "skipped", canaries["dose note"]),
		dose("dose-taken-01", now.Add(-29*time.Hour), "taken", ""),
		dose("dose-corrected-01", now.Add(-53*time.Hour), "skipped", ""),
		{
			RecordID: "fix-corrected-01", Kind: syncmodel.KindMedicationCorrection, CreatedAt: now.Add(-time.Hour),
			Payload: json.RawMessage(fmt.Sprintf(
				`{"correction_id":"fix-corrected-01","target_event_id":"dose-corrected-01","created_at":%q,"reason":"user_edit","changes":{"status":"taken"}}`,
				stamp(now.Add(-52*time.Hour)))),
		},
		dose("dose-old-01", now.Add(-96*time.Hour), "skipped", ""),
	}
}

// sleepObservationPayload records each episode as observed when it ended, so
// the fixture's evidence age is realistic for the freshness policy.
func sleepObservationPayload(id string, start, end time.Time) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(
		`{"observation_id":%q,"kind":"sleep_episode","start_at":%q,"end_at":%q,"zone_id":"America/New_York","sleep":{"classification":"principal"},"provenance":{"acquisition_method":"synthetic","evidence_status":"directly_observed","recorded_at":%q,"source_record_id":%q}}`,
		id,
		start.UTC().Format(time.RFC3339),
		end.UTC().Format(time.RFC3339),
		end.UTC().Format(time.RFC3339),
		canaries["source record id"],
	))
}

// TestNoCanaryReachesThePublicSurface is the acceptance test for the P5-a
// projection firewall. It walks every public response, including failures,
// and asserts that nothing planted in the private store appears.
func TestNoCanaryReachesThePublicSurface(t *testing.T) {
	fixture := newLeakFixture(t)

	cookie := fixture.authenticate(t)
	responses := map[string]*httptest.ResponseRecorder{
		"page":          fixture.get(t, "/p/"+fixture.token, cookie),
		"availability":  fixture.get(t, "/p/"+fixture.token+"/availability", cookie),
		"stylesheet":    fixture.get(t, "/p/assets/portal.css", ""),
		"unauth page":   fixture.get(t, "/p/"+fixture.token, ""),
		"unknown link":  fixture.get(t, "/p/not-a-real-token", ""),
		"unauth JSON":   fixture.get(t, "/p/"+fixture.token+"/availability", ""),
		"foreign login": fixture.post(t, "/p/"+fixture.token+"/session", "https://evil.example"),
	}

	for name, recorder := range responses {
		haystack := recorder.Body.String()
		for _, values := range recorder.Header() {
			haystack += "\n" + strings.Join(values, "\n")
		}
		for what, canary := range canaries {
			if strings.Contains(haystack, canary) {
				t.Errorf("%s response leaked the %s canary", name, what)
			}
		}
	}
}

// TestNoCanaryReachesThePortalDatabase is the stronger structural claim: even
// at rest, the public database must not contain private values. It reads the
// file bytes rather than querying, so an unexpected column would still fail.
func TestNoCanaryReachesThePortalDatabase(t *testing.T) {
	fixture := newLeakFixture(t)
	cookie := fixture.authenticate(t)
	fixture.get(t, "/p/"+fixture.token, cookie)
	fixture.get(t, "/p/"+fixture.token+"/availability", cookie)

	// Checkpoint the WAL so pages that are still in the write-ahead log are
	// included in what we scan.
	for _, suffix := range []string{"", "-wal"} {
		path := fixture.dbPath + suffix
		data, err := os.ReadFile(path)
		if err != nil {
			if suffix == "-wal" && os.IsNotExist(err) {
				continue
			}
			t.Fatalf("read %s: %v", path, err)
		}
		for what, canary := range canaries {
			if bytes.Contains(data, []byte(canary)) {
				t.Errorf("portal database file %s contains the %s canary", filepath.Base(path), what)
			}
		}
	}
}

// TestPortalAuditNeverStoresVisitorOrLinkText checks the audit table holds
// only closed-enum events and pseudonymous sources.
func TestPortalAuditNeverStoresVisitorOrLinkText(t *testing.T) {
	fixture := newLeakFixture(t)
	cookie := fixture.authenticate(t)
	fixture.get(t, "/p/"+fixture.token, cookie)

	summaries, err := fixture.portal.SummarizeAccess(context.Background(), fixture.profileID(t), time.Time{})
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	if len(summaries) == 0 {
		t.Fatal("no audit rows were recorded")
	}
	allowed := map[portal.AccessEvent]bool{
		portal.EventPageView: true, portal.EventAvailabilityRead: true,
		portal.EventPasscodeAccepted: true, portal.EventPasscodeRejected: true,
		portal.EventThrottled: true, portal.EventLinkRejected: true,
	}
	for _, summary := range summaries {
		if !allowed[summary.Event] {
			t.Errorf("audit contains unexpected event %q", summary.Event)
		}
		if strings.Contains(string(summary.Event), fixture.token) {
			t.Error("audit event text contains the link token")
		}
	}
}

// TestMaterializerDropsEstimatorInternals asserts the narrowing directly on
// the value that crosses the boundary, independent of any HTTP handler.
func TestMaterializerDropsEstimatorInternals(t *testing.T) {
	fixture := newLeakFixture(t)
	snapshot, err := portalbridge.Materializer{
		Sleep: readmodel.SleepReader{Store: fixture.private},
		Now:   func() time.Time { return fixture.now },
	}.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.Status != portal.StatusAvailable {
		t.Fatalf("status = %q, want %q", snapshot.Status, portal.StatusAvailable)
	}
	if len(snapshot.Windows) == 0 {
		t.Fatal("no windows were produced")
	}

	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for what, canary := range canaries {
		if bytes.Contains(encoded, []byte(canary)) {
			t.Errorf("snapshot carries the %s canary", what)
		}
	}
	// Confidence is deliberately absent: ADR-0022 measured the buckets
	// inverted, so a public label would misinform.
	for _, forbidden := range []string{"confidence", "Confidence", "explanation", "estimateId", "high", "medium"} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Errorf("snapshot carries %q, which is not in the allowlist", forbidden)
		}
	}
	for _, window := range snapshot.Windows {
		if !window.EndAt.After(fixture.now) {
			t.Error("snapshot contains an already-elapsed window")
		}
	}
}

// TestTheWakingDayUnderWayIsPublished is the regression guard for a page that
// said "not awake right now" all day. The estimator forecasts waking windows
// only after each predicted sleep, so the day in progress was never a window.
// It now is: from the observed wake to the earliest likely next sleep, and a
// visitor during it reads that the owner is likely awake.
func TestTheWakingDayUnderWayIsPublished(t *testing.T) {
	fixture := newLeakFixture(t)
	snapshot, err := portalbridge.Materializer{
		Sleep: readmodel.SleepReader{Store: fixture.private},
		Now:   func() time.Time { return fixture.now },
	}.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snapshot.Windows) < 2 {
		t.Fatalf("windows = %+v", snapshot.Windows)
	}
	// The newest recorded sleep ended three hours before now.
	current := snapshot.Windows[0]
	if want := fixture.now.Add(-3 * time.Hour); !current.StartAt.Equal(want) {
		t.Errorf("the day under way starts at %s, want the observed wake %s", current.StartAt, want)
	}
	if !current.EndAt.After(fixture.now) || !current.EndAt.Before(snapshot.Windows[1].StartAt) {
		t.Errorf("the day under way ends at %s; it must end after now and before the next forecast window at %s",
			current.EndAt, snapshot.Windows[1].StartAt)
	}
	if view := portal.BuildView(snapshot, fixture.now); view.Headline != "Likely awake right now" {
		t.Errorf("a visitor during the owner's waking day reads %q", view.Headline)
	}
}

// TestMaterializerRefusesRatherThanGuesses covers the empty-history case: the
// portal must report a refusal, not invent availability.
func TestMaterializerRefusesRatherThanGuesses(t *testing.T) {
	dir := t.TempDir()
	private, err := store.Open(filepath.Join(dir, "empty.db"), bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = private.Close() }()

	snapshot, err := portalbridge.Materializer{
		Sleep: readmodel.SleepReader{Store: private},
		Now:   func() time.Time { return time.Date(2026, 8, 3, 15, 0, 0, 0, time.UTC) },
	}.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.Status == portal.StatusAvailable {
		t.Error("an empty history produced an availability claim")
	}
	if len(snapshot.Windows) != 0 {
		t.Errorf("an empty history produced %d windows", len(snapshot.Windows))
	}
}

// TestGrantWithoutWindowsPublishesNothing proves the grant gates what is
// materialized, not merely what is rendered.
func TestGrantWithoutWindowsPublishesNothing(t *testing.T) {
	fixture := newLeakFixture(t)
	ctx := context.Background()
	profileID, _ := portal.NewProfileID()
	token, _ := portal.NewLinkToken()
	if err := fixture.portal.CreateProfile(ctx, portal.CreateProfileInput{
		ProfileID: profileID,
		Token:     token,
		Passcode:  testPasscode,
		Grants:    portal.Grants{WakingWindows: false},
		CreatedAt: fixture.now,
		ExpiresAt: fixture.now.Add(24 * time.Hour),
	}); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	materializer := portalbridge.Materializer{
		Sleep:    readmodel.SleepReader{Store: fixture.private},
		Profiles: fixture.portal,
		Sink:     fixture.portal,
		Now:      func() time.Time { return fixture.now },
	}
	materializeAll(ctx, t, materializer, fixture.now)
	snapshot, err := fixture.portal.ReadSnapshot(ctx, profileID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(snapshot.Windows) != 0 {
		t.Errorf("a profile without the windows grant received %d windows", len(snapshot.Windows))
	}
	if snapshot.Status == portal.StatusAvailable {
		t.Error("a profile without the windows grant was marked available")
	}
}

func (f *leakFixture) profileID(t *testing.T) string {
	t.Helper()
	profile, err := f.portal.ResolveLink(context.Background(), f.token, f.now)
	if err != nil {
		t.Fatalf("resolve link: %v", err)
	}
	return profile.ID
}

func (f *leakFixture) get(t *testing.T, path, cookie string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.RemoteAddr = "203.0.113.44:6000"
	if cookie != "" {
		request.AddCookie(&http.Cookie{Name: "__Host-zb_portal", Value: cookie})
	}
	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, request)
	return recorder
}

func (f *leakFixture) post(t *testing.T, path, origin string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"passcode": {testPasscode}}
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", origin)
	request.RemoteAddr = "203.0.113.44:6000"
	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, request)
	return recorder
}

func (f *leakFixture) authenticate(t *testing.T) string {
	t.Helper()
	recorder := f.post(t, "/p/"+f.token+"/session", testOrigin)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("login status = %d, want 303", recorder.Code)
	}
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == "__Host-zb_portal" {
			return cookie.Value
		}
	}
	t.Fatal("no session cookie issued")
	return ""
}

// A link with the grant receives the one dose that now stands as skipped and
// is less than 72 hours old, by its time and zone alone (ADR-0058).
func TestALinkSharesSkippedDosesByTimeAlone(t *testing.T) {
	fixture := newLeakFixture(t)
	snapshot, err := fixture.portal.ReadSnapshot(context.Background(), fixture.profileID(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []portal.SkippedDose{{At: skippedDoseAt(fixture.now).UTC(), ZoneID: "America/New_York"}}
	if !snapshot.DosesShared || len(snapshot.SkippedDoses) != 1 || snapshot.SkippedDoses[0] != want[0] {
		t.Fatalf("doses shared %v: %+v, want %+v", snapshot.DosesShared, snapshot.SkippedDoses, want)
	}
}

// A link without the grant gets no doses, even beside one that has it, and
// with no link that has it the doses are not read at all.
func TestOnlyALinkWithTheGrantGetsDoses(t *testing.T) {
	fixture := newLeakFixture(t)
	ctx := context.Background()
	profileID, _ := portal.NewProfileID()
	token, _ := portal.NewLinkToken()
	if err := fixture.portal.CreateProfile(ctx, portal.CreateProfileInput{
		ProfileID: profileID, Token: token, Passcode: testPasscode,
		Grants:    portal.Grants{WakingWindows: true},
		CreatedAt: fixture.now, ExpiresAt: fixture.now.Add(24 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	materializer := portalbridge.Materializer{
		Sleep:    readmodel.SleepReader{Store: fixture.private},
		Doses:    readmodel.MedicationReader{Store: fixture.private},
		Profiles: fixture.portal,
		Sink:     fixture.portal,
		Now:      func() time.Time { return fixture.now },
	}
	materializeAll(ctx, t, materializer, fixture.now)
	snapshot, err := fixture.portal.ReadSnapshot(ctx, profileID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.DosesShared || len(snapshot.SkippedDoses) != 0 {
		t.Errorf("a link without the grant received doses: %+v", snapshot.SkippedDoses)
	}

	unread := portalbridge.Materializer{
		Sleep:    readmodel.SleepReader{Store: fixture.private},
		Doses:    failingDoses{t: t},
		Profiles: onlyProfiles{profiles: []portal.Profile{{ID: profileID, Grants: portal.Grants{WakingWindows: true}}}},
		Sink:     fixture.portal,
	}
	if _, err := unread.Prepare(ctx, fixture.now); err != nil {
		t.Fatal(err)
	}
}

type failingDoses struct{ t *testing.T }

func (f failingDoses) EffectiveDoses(context.Context) ([]medication.EffectiveEvent, error) {
	f.t.Error("doses were read with no link that shows them")
	return nil, nil
}

type onlyProfiles struct{ profiles []portal.Profile }

func (o onlyProfiles) ListActiveProfiles(context.Context, time.Time) ([]portal.Profile, error) {
	return o.profiles, nil
}
