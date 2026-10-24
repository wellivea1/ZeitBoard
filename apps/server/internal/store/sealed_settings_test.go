package store

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A sealed setting is kept encrypted whole, replaced in place, and a change
// the owner made is audited with the device that made it.
func TestASealedSettingIsKeptEncryptedAndAudited(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zeitboardd.db")
	st, err := Open(path, []byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	at := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)

	if _, ok, err := st.SealedSetting(ctx, "synthetic"); ok || err != nil {
		t.Fatalf("a setting never kept read as %v, %v", ok, err)
	}
	first := []byte(`{"refreshToken":"synthetic-canary-one"}`)
	if err := st.PutSealedSetting(ctx, "synthetic", first, at, "synthetic_changed", "device-synthetic"); err != nil {
		t.Fatal(err)
	}
	second := []byte(`{"refreshToken":"synthetic-canary-two"}`)
	if err := st.PutSealedSetting(ctx, "synthetic", second, at.Add(time.Minute), "", ""); err != nil {
		t.Fatal(err)
	}
	value, ok, err := st.SealedSetting(ctx, "synthetic")
	if err != nil || !ok || !bytes.Equal(value, second) {
		t.Fatalf("read %q, %v, %v", value, ok, err)
	}

	var events int
	var device string
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*), MAX(device_id) FROM audit_events WHERE event_type = 'synthetic_changed'`).
		Scan(&events, &device); err != nil {
		t.Fatal(err)
	}
	if events != 1 || device != "device-synthetic" {
		t.Errorf("%d audit events, from %q; the server's own change is not the owner's", events, device)
	}
	var total int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events`).Scan(&total); err != nil || total != 1 {
		t.Errorf("%d audit events in all, %v", total, err)
	}

	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{path, path + "-wal"} {
		data, err := os.ReadFile(file)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		for _, canary := range []string{"synthetic-canary-one", "synthetic-canary-two"} {
			if bytes.Contains(data, []byte(canary)) {
				t.Errorf("%s holds %s in the clear", filepath.Base(file), canary)
			}
		}
	}
}

func TestASealedSettingIsBoundToItsName(t *testing.T) {
	st, err := Open(":memory:", []byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if err := st.PutSealedSetting(ctx, "synthetic", []byte("value"), time.Now(), "", ""); err != nil {
		t.Fatal(err)
	}
	// A row moved under another name does not decrypt as that setting.
	if _, err := st.db.ExecContext(ctx, `UPDATE sealed_settings SET name = 'other' WHERE name = 'synthetic'`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.SealedSetting(ctx, "other"); err == nil {
		t.Error("a setting decrypted under another name")
	}
	if err := st.PutSealedSetting(ctx, "synthetic", make([]byte, maxSealedSettingBytes+1), time.Now(), "", ""); err == nil {
		t.Error("an oversized setting was kept")
	}
	if err := st.PutSealedSetting(ctx, " ", []byte("value"), time.Now(), "", ""); err == nil {
		t.Error("a setting without a name was kept")
	}
}
