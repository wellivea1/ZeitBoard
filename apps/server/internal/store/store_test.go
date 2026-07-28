package store

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	syncmodel "non24.app/server/internal/sync"
)

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
