package main

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	storage "non24.app/core/storage/sqlite"
)

// Opt in only against an isolated loopback daemon containing synthetic data.
// This exercises the actual API/store in addition to the stateful TLS fixtures.
func TestDesktopSyncAgainstDisposableDaemon(t *testing.T) {
	address := os.Getenv("ZEITBOARD_TEST_SYNC_URL")
	if address == "" {
		t.Skip("requires a disposable loopback daemon")
	}
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Hostname() != "127.0.0.1" {
		t.Fatal("live test requires HTTPS loopback")
	}
	secret := os.Getenv("ZEITBOARD_TEST_SYNC_SECRET")
	if !strings.HasPrefix(secret, "synthetic-") {
		t.Fatal("live test requires a synthetic enrollment secret")
	}
	a := newTestApp(t)
	enroll := func(app *App) {
		t.Helper()
		if _, err := app.ConfigureBackendSync(BackendSyncInput{Enabled: true, BackendURL: address, EnrollmentSecret: secret, DeviceLabel: "Synthetic recovery qualification", InsecureSkipVerify: true}); err != nil {
			t.Fatal(err)
		}
	}
	enroll(a)
	seedOneSleepEntry(t, a)
	syncRecoveryPeer(t, a)
	t.Cleanup(func() { _ = a.store.DeleteAllSleepData(context.Background()); _, _ = a.SyncNow() })
	// Restore an empty record database with this same valid credential. Own
	// server envelopes must restore the source instead of being skipped.
	cfg, token, err := a.store.LoadSyncConnection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b := newTestApp(t)
	if err := b.store.SaveSyncEnrollment(context.Background(), cfg, token); err != nil {
		t.Fatal(err)
	}
	if syncRecoveryPeer(t, b).PulledCount < 1 {
		t.Fatal("same-device restore did not recover its source")
	}
	sources, err := b.store.ListSleepObservations(context.Background())
	if err != nil || len(sources) != 1 {
		t.Fatal("unexpected restored source count")
	}
	end := sources[0].EndAt.Add(-time.Minute)
	correction := storage.SleepCorrectionRecord{CorrectionID: newLocalID("cor_synthetic"), TargetObservationID: sources[0].ObservationID, CreatedAt: time.Now().UTC(), Reason: storage.CorrectionReasonUserEdit, Changes: storage.SleepCorrectionChanges{EndAt: &end}}
	if err := b.store.AppendSleepCorrection(context.Background(), correction); err != nil {
		t.Fatal(err)
	}
	syncRecoveryPeer(t, b)
	syncRecoveryPeer(t, a)
	corrections, err := a.store.ListSleepCorrections(context.Background())
	if err != nil || len(corrections) != 1 {
		t.Fatal("real daemon did not relay correction")
	}
	if err := a.store.DeleteSleepObservation(context.Background(), sources[0].ObservationID); err != nil {
		t.Fatal(err)
	}
	syncRecoveryPeer(t, a)
	syncRecoveryPeer(t, b)
	remaining, err := b.store.ListSleepObservations(context.Background())
	if err != nil || len(remaining) != 0 {
		t.Fatal("real daemon erasure did not clear restored copy")
	}
	enroll(b)
	syncRecoveryPeer(t, b)
	remaining, err = b.store.ListSleepObservations(context.Background())
	if err != nil || len(remaining) != 0 {
		t.Fatal("reenrollment revived erased source")
	}
}
