package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	syncmodel "non24.app/server/internal/sync"
)

func TestErasingObservationRemovesItsRetainedCorrectionPayloads(t *testing.T) {
	ctx := context.Background()
	st, err := Open(t.TempDir()+"/synthetic.db", bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	if err := st.RegisterDevice(ctx, "device_synthetic", "synthetic", bytes.Repeat([]byte{1}, 32), now); err != nil {
		t.Fatal(err)
	}
	_, _, err = st.Append(ctx, "device_synthetic", []syncmodel.PushRecord{
		{RecordID: "obs_synthetic", Kind: syncmodel.KindObservation, CreatedAt: now, Payload: json.RawMessage(`{"kind":"sleep_episode"}`)},
		{RecordID: "cor_synthetic", Kind: syncmodel.KindCorrection, CreatedAt: now, Payload: json.RawMessage(`{"target_observation_id":"obs_synthetic","reason":"user_edit"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	erased, tombstones, _, err := st.EraseSyncRecords(ctx, "device_synthetic", []string{"obs_synthetic"}, now)
	if err != nil || erased != 2 || tombstones != 2 {
		t.Fatalf("dependent erasure: erased=%d tombstones=%d err=%v", erased, tombstones, err)
	}
	var remaining int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sync_records WHERE kind != 'tombstone'`).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatal("private correction payload remained")
	}
}

func TestErasedObservationRejectsLaterProviderRevisionIDs(t *testing.T) {
	ctx := context.Background()
	st, err := Open(t.TempDir()+"/server.db", bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	if err := st.RegisterDevice(ctx, "device_synthetic", "synthetic", bytes.Repeat([]byte{1}, 32), now); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := st.EraseSyncRecords(ctx, "device_synthetic", []string{"hc_erased_synthetic"}, now); err != nil {
		t.Fatal(err)
	}
	_, accepted, err := st.Append(ctx, "device_synthetic", []syncmodel.PushRecord{{
		RecordID: "cor_new_provider_revision", Kind: syncmodel.KindCorrection, CreatedAt: now,
		Payload: json.RawMessage(`{"correction_id":"cor_new_provider_revision","target_observation_id":"hc_erased_synthetic","created_at":"2026-09-02T11:00:00Z","reason":"source_conflict","acquisition_method":"health_connect","changes":{"start_at":"2026-09-02T02:02:00Z"}}`),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if accepted != 0 {
		t.Fatal("new provider revision retained erased behavioral timestamps")
	}
	var corrections int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sync_records WHERE kind = 'correction'`).Scan(&corrections); err != nil {
		t.Fatal(err)
	}
	if corrections != 0 {
		t.Fatal("erased target acquired another correction payload")
	}
}

func TestPullKindThroughFiltersAndHonorsHighWater(t *testing.T) {
	ctx := context.Background()
	st, err := Open(t.TempDir()+"/server.db", bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	if err := st.RegisterDevice(ctx, "device_store_test", "desktop", bytes.Repeat([]byte{1}, 32), now); err != nil {
		t.Fatal(err)
	}
	initial := []syncmodel.PushRecord{
		{RecordID: "obs_store_01", Kind: syncmodel.KindObservation, CreatedAt: now, Payload: json.RawMessage(`{"kind":"sleep_episode"}`)},
		{RecordID: "task_store_01_r1", Kind: syncmodel.KindTask, CreatedAt: now, Payload: json.RawMessage(`{"revision":1}`)},
		{RecordID: "cor_store_01", Kind: syncmodel.KindCorrection, CreatedAt: now, Payload: json.RawMessage(`{"reason":"user_edit"}`)},
	}
	if _, _, err := st.Append(ctx, "device_store_test", initial); err != nil {
		t.Fatal(err)
	}
	highWater, err := st.RecordHighWater(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if highWater != 3 {
		t.Fatalf("high-water cursor = %d, want 3", highWater)
	}
	if _, _, err := st.Append(ctx, "device_store_test", []syncmodel.PushRecord{{
		RecordID: "obs_store_02", Kind: syncmodel.KindObservation, CreatedAt: now.Add(time.Minute), Payload: json.RawMessage(`{"kind":"sleep_episode"}`),
	}}); err != nil {
		t.Fatal(err)
	}

	records, cursor, err := st.PullKindThrough(ctx, syncmodel.KindObservation, 0, highWater, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].RecordID != "obs_store_01" || cursor != 1 {
		t.Fatalf("observation page = %#v cursor=%d", records, cursor)
	}
	records, cursor, err = st.PullKindThrough(ctx, syncmodel.KindObservation, cursor, highWater, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 || cursor != 1 {
		t.Fatalf("observation tail = %#v cursor=%d, want no post-snapshot records", records, cursor)
	}
	records, _, err = st.PullKindThrough(ctx, syncmodel.KindCorrection, 0, highWater, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].RecordID != "cor_store_01" {
		t.Fatalf("correction page = %#v", records)
	}
}

func TestPullKindThroughRejectsUnsupportedKindAndMigrationCreatesIndex(t *testing.T) {
	ctx := context.Background()
	st, err := Open(t.TempDir()+"/server.db", bytes.Repeat([]byte{6}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, _, err := st.PullKindThrough(ctx, syncmodel.KindTombstone, 0, 1, 1); err == nil {
		t.Fatal("unsupported stored kind was accepted")
	}
	var sqlText string
	if err := st.db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'index' AND name = 'idx_sync_records_kind_seq'`).Scan(&sqlText); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sqlText, "sync_records(kind, seq)") {
		t.Fatalf("unexpected filtered-read index: %s", sqlText)
	}
}

func TestListProposalPageIsBoundedStableAndCarriesJoinedOneUseTokens(t *testing.T) {
	ctx := context.Background()
	st, err := Open(t.TempDir()+"/server.db", bytes.Repeat([]byte{5}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	if err := st.RegisterDevice(ctx, "device_agent", "agent", bytes.Repeat([]byte{1}, 32), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := st.RegisterDevice(ctx, "device_desktop", "desktop", bytes.Repeat([]byte{2}, 32), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	expired := createProposalForStoreTest(t, st, "proposal_01", now.Add(-4*time.Hour), now.Add(-time.Hour))
	approved := createProposalForStoreTest(t, st, "proposal_02", now.Add(-3*time.Hour), now.Add(2*time.Hour))
	if _, err := st.DecideProposal(ctx, approved.ID, "device_agent", ProposalApproved, approved.DecisionToken, now.Add(-2*time.Hour), json.RawMessage(`{"source":"test"}`)); err != nil {
		t.Fatal(err)
	}
	createProposalForStoreTest(t, st, "proposal_03", now.Add(-2*time.Hour), now.Add(2*time.Hour))
	createProposalForStoreTest(t, st, "proposal_04", now.Add(-time.Hour), now.Add(2*time.Hour))

	first, err := st.ListProposalPage(ctx, ProposalPageCursor{}, 2, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Records) != 2 || !first.HasMore || first.NextCursor.AfterRowID <= 0 || !first.NextCursor.Active {
		t.Fatalf("first page = %+v, want two records and a continuation", first)
	}
	if first.Records[0].ID != "proposal_04" || first.Records[1].ID != "proposal_03" {
		t.Fatalf("first page order = %q, %q", first.Records[0].ID, first.Records[1].ID)
	}
	for _, record := range first.Records {
		if record.DecisionToken == "" {
			t.Fatalf("pending unexpired proposal %q has no joined decision token", record.ID)
		}
	}

	// A newer insert between requests must not shift or duplicate the next page.
	createProposalForStoreTest(t, st, "proposal_05", now, now.Add(2*time.Hour))
	second, err := st.ListProposalPage(ctx, first.NextCursor, 2, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Records) != 2 || second.HasMore || second.NextCursor != (ProposalPageCursor{}) {
		t.Fatalf("second page = %+v, want the final two records", second)
	}
	if second.Records[0].ID != approved.ID || second.Records[1].ID != expired.ID {
		t.Fatalf("stable second page order = %q, %q", second.Records[0].ID, second.Records[1].ID)
	}
	if second.Records[0].Status != ProposalApproved || second.Records[0].DecisionToken != "" {
		t.Fatalf("approved proposal leaked a token: %+v", second.Records[0])
	}
	if second.Records[1].Status != ProposalPending || second.Records[1].DecisionToken != "" {
		t.Fatalf("expired pending proposal carried a token: %+v", second.Records[1])
	}

	// A token listed for one enrolled device remains decidable by another, and
	// the consumed nonce is not re-minted on a later listing.
	decided, err := st.DecideProposal(ctx, first.Records[0].ID, "device_desktop", ProposalRejected, first.Records[0].DecisionToken, now, json.RawMessage(`{"source":"cross_device_test"}`))
	if err != nil {
		t.Fatal(err)
	}
	if decided.Status != ProposalRejected {
		t.Fatalf("cross-device decision = %+v", decided)
	}
	latest, err := st.ListProposalPage(ctx, ProposalPageCursor{}, MaxProposalPageLimit, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range latest.Records {
		if record.ID == decided.ID && (record.Status != ProposalRejected || record.DecisionToken != "") {
			t.Fatalf("decided proposal listing = %+v", record)
		}
	}
}

func TestListProposalPagePrioritizesOlderActiveProposal(t *testing.T) {
	ctx := context.Background()
	st, err := Open(t.TempDir()+"/server.db", bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	if err := st.RegisterDevice(ctx, "device_agent", "agent", bytes.Repeat([]byte{1}, 32), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	active := createProposalForStoreTest(t, st, "proposal_active", now.Add(-time.Hour), now.Add(time.Hour))
	decided := createProposalForStoreTest(t, st, "proposal_decided", now.Add(-time.Minute), now.Add(time.Hour))
	if _, err := st.DecideProposal(ctx, decided.ID, "device_agent", ProposalRejected, decided.DecisionToken, now, json.RawMessage(`{"source":"test"}`)); err != nil {
		t.Fatal(err)
	}

	first, err := st.ListProposalPage(ctx, ProposalPageCursor{}, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Records) != 1 || first.Records[0].ID != active.ID || !first.HasMore || !first.NextCursor.Active {
		t.Fatalf("active-first page = %+v", first)
	}
	second, err := st.ListProposalPage(ctx, first.NextCursor, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Records) != 1 || second.Records[0].ID != decided.ID || second.HasMore {
		t.Fatalf("history continuation = %+v", second)
	}
}

func TestListProposalPageValidatesBoundsAndMigrationCreatesUnusedNonceIndex(t *testing.T) {
	ctx := context.Background()
	st, err := Open(t.TempDir()+"/server.db", bytes.Repeat([]byte{4}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	for name, call := range map[string]func() error{
		"negative cursor": func() error {
			_, err := st.ListProposalPage(ctx, ProposalPageCursor{AfterRowID: -1}, 1, now)
			return err
		},
		"incomplete continuation": func() error {
			_, err := st.ListProposalPage(ctx, ProposalPageCursor{AfterRowID: 1}, 1, now)
			return err
		},
		"future snapshot": func() error {
			_, err := st.ListProposalPage(ctx, ProposalPageCursor{AfterRowID: 1, ThroughRowID: 1, AsOf: now.Add(time.Second)}, 1, now)
			return err
		},
		"non-empty initial cursor": func() error {
			_, err := st.ListProposalPage(ctx, ProposalPageCursor{ThroughRowID: 1}, 1, now)
			return err
		},
		"zero limit": func() error {
			_, err := st.ListProposalPage(ctx, ProposalPageCursor{}, 0, now)
			return err
		},
		"excessive limit": func() error {
			_, err := st.ListProposalPage(ctx, ProposalPageCursor{}, MaxProposalPageLimit+1, now)
			return err
		},
		"zero time": func() error {
			_, err := st.ListProposalPage(ctx, ProposalPageCursor{}, 1, time.Time{})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil {
				t.Fatal("invalid proposal page request was accepted")
			}
		})
	}

	var sqlText string
	if err := st.db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'index' AND name = 'idx_approval_nonces_proposal_unused'`).Scan(&sqlText); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sqlText, "approval_nonces(proposal_id)") || !strings.Contains(sqlText, "WHERE used_at = ''") {
		t.Fatalf("unexpected unused-nonce index: %s", sqlText)
	}

	planRows, err := st.db.QueryContext(ctx, `EXPLAIN QUERY PLAN
		SELECT p.rowid, approval.nonce
		FROM proposals AS p
		LEFT JOIN approval_nonces AS approval
		  ON approval.proposal_id = p.id
		 AND approval.used_at = ''
		 AND approval.expires_at > ?
		 AND p.status = ?
		 AND p.expires_at > ?
		ORDER BY p.rowid DESC
		LIMIT ?`, now.Format(time.RFC3339Nano), string(ProposalPending), now.Format(time.RFC3339Nano), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer planRows.Close()
	usesUnusedNonceIndex := false
	for planRows.Next() {
		var id, parent, unused int
		var detail string
		if err := planRows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(detail, "idx_approval_nonces_proposal_unused") {
			usesUnusedNonceIndex = true
		}
	}
	if err := planRows.Err(); err != nil {
		t.Fatal(err)
	}
	if !usesUnusedNonceIndex {
		t.Fatal("proposal page query plan did not use the unused-nonce index")
	}
}

func createProposalForStoreTest(t *testing.T, st *Store, id string, createdAt, expiresAt time.Time) ProposalRecord {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"proposal_id": id})
	if err != nil {
		t.Fatal(err)
	}
	record, err := st.CreateProposal(context.Background(), ProposalInput{
		ID:        id,
		ActionID:  "propose_place_task",
		DeviceID:  "device_agent",
		CreatedAt: createdAt,
		ExpiresAt: expiresAt,
		Payload:   payload,
		Audit:     json.RawMessage(`{"source":"test"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func TestProposalScopesCountAllPendingBeforePagination(t *testing.T) {
	ctx := t.Context()
	st, err := Open(t.TempDir()+"/scope.db", bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	if err := st.RegisterDevice(ctx, "device_agent", "agent", bytes.Repeat([]byte{1}, 32), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	create := func(id, action string, expires time.Time) ProposalRecord {
		record, err := st.CreateProposal(ctx, ProposalInput{ID: id, ActionID: action, DeviceID: "device_agent", CreatedAt: now.Add(-time.Hour), ExpiresAt: expires, Payload: json.RawMessage(`{"synthetic":true}`), Audit: json.RawMessage(`{"source":"test"}`)})
		if err != nil {
			t.Fatal(err)
		}
		return record
	}
	create("visitor_old", ActionVisitorRequest, now.Add(time.Hour))
	create("visitor_older", ActionVisitorRequest, now.Add(2*time.Hour))
	for i := 0; i < 105; i++ {
		create(fmt.Sprintf("backend_%03d", i), "propose_place_task", now.Add(3*time.Hour))
	}
	create("visitor_expired", ActionVisitorRequest, now.Add(-time.Minute))
	consumed := create("visitor_consumed", ActionVisitorRequest, now.Add(time.Hour))
	if _, err := st.db.ExecContext(ctx, `UPDATE approval_nonces SET used_at=? WHERE proposal_id=?`, now.Format(time.RFC3339Nano), consumed.ID); err != nil {
		t.Fatal(err)
	}
	page, err := st.ListProposalPage(ctx, ProposalPageCursor{Scope: ProposalScopeVisitor}, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if page.PendingCount != 2 || page.NextExpiryAt != now.Add(time.Hour).Format(time.RFC3339Nano) || !page.HasMore || page.NextCursor.Scope != ProposalScopeVisitor {
		t.Fatalf("visitor summary = %+v", page)
	}
	seen := map[string]bool{}
	for {
		for _, record := range page.Records {
			if record.ActionID != ActionVisitorRequest || seen[record.ID] {
				t.Fatal("scope or pagination leaked")
			}
			seen[record.ID] = true
		}
		if !page.HasMore {
			break
		}
		page, err = st.ListProposalPage(ctx, page.NextCursor, 1, now)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != 4 {
		t.Fatalf("visitor pages contained %d records", len(seen))
	}
	backend, err := st.ListProposalPage(ctx, ProposalPageCursor{Scope: ProposalScopeBackend}, 1, now)
	if err != nil || backend.PendingCount != 105 || len(backend.Records) != 1 || backend.Records[0].ActionID == ActionVisitorRequest {
		t.Fatalf("backend count/scope failed: %v", err)
	}
	expired, err := st.ListProposalPage(ctx, ProposalPageCursor{Scope: ProposalScopeVisitor}, 10, now.Add(4*time.Hour))
	if err != nil || expired.PendingCount != 0 || expired.NextExpiryAt != "" {
		t.Fatalf("expiry summary incorrect: %v", err)
	}
}

func TestProposalExpiryMatchesTokenAtSubsecondBoundary(t *testing.T) {
	st, err := Open(t.TempDir()+"/expiry.db", bytes.Repeat([]byte{8}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	if err := st.RegisterDevice(t.Context(), "device_agent", "agent", bytes.Repeat([]byte{1}, 32), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	record := createProposalForStoreTest(t, st, "expiry", now.Add(-time.Hour), now.Add(123*time.Millisecond))
	for _, at := range []time.Time{now.Add(-time.Nanosecond), now, now.Add(time.Nanosecond), now.Add(500 * time.Millisecond)} {
		page, err := st.ListProposalPage(t.Context(), ProposalPageCursor{}, 10, at)
		if err != nil {
			t.Fatal(err)
		}
		want := 0
		if at.Before(now) {
			want = 1
		}
		if page.PendingCount != want || (page.Records[0].DecisionToken != "") != (want == 1) {
			t.Fatalf("pending count/token at %s = %d", at.Format(time.RFC3339Nano), page.PendingCount)
		}
	}
	if _, err := st.DecideProposal(t.Context(), record.ID, "device_agent", ProposalApproved, record.DecisionToken, now, json.RawMessage(`{"source":"test"}`)); !errors.Is(err, ErrExpiredApprovalToken) {
		t.Fatalf("boundary decision was not expired: %v", err)
	}
}
