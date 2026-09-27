package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// Medication sync (ADR-0048). A definition is a mutable intention and travels
// like a task, as immutable revisions "<medication_id>_r<n>"; doses and dose
// corrections are immutable evidence whose record id is their own. A device
// whose unsent edit to a definition meets a revision it had not seen rebases
// the edit on top of it: the fields the edit changed keep its values and every
// other field takes the other device's, so neither edit is silently reverted.
// Deleting a definition or a dose erases it and everything recorded against it
// here, queues the same erasure for the server, and refuses anything that
// arrives for it later.

const (
	SyncKindMedication           = "medication"
	SyncKindMedicationEvent      = "medication_event"
	SyncKindMedicationCorrection = "medication_correction"

	MaxMedicationSyncPageSize = 100
)

// MedicationSyncRecord is one definition revision, dose or dose correction
// ready for the push endpoint.
type MedicationSyncRecord struct {
	RecordID  string
	Kind      string
	CreatedAt time.Time
	Payload   []byte
}

// SyncPullMedication is a downloaded definition revision.
type SyncPullMedication struct {
	Medication MedicationRecord
}

func (SyncPullMedication) isSyncPullRecord() {}

// SyncPullMedicationEvent is a downloaded dose.
type SyncPullMedicationEvent struct {
	Event MedicationEventRecord
}

func (SyncPullMedicationEvent) isSyncPullRecord() {}

// SyncPullMedicationCorrection is a downloaded dose correction.
type SyncPullMedicationCorrection struct {
	Correction MedicationEventCorrectionRecord
}

func (SyncPullMedicationCorrection) isSyncPullRecord() {}

// errSyncParentMissing defers a downloaded dose until its definition exists,
// and a correction until its dose does. Parents normally arrive first; after a
// server restore, devices may upload again in any order.
var errSyncParentMissing = errors.New("downloaded record arrived before the record it belongs to")

func (s *Store) initializeMedicationSync(ctx context.Context) error {
	for _, statement := range []string{
		// Acknowledged definition revisions. The highest one's payload is the
		// version an unsent local edit was made from.
		`CREATE TABLE IF NOT EXISTS local_medication_sync_records(
			record_id TEXT PRIMARY KEY,
			medication_id TEXT NOT NULL,
			revision INTEGER NOT NULL,
			payload_json BLOB NOT NULL,
			pushed_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_medication_sync_records ON local_medication_sync_records(medication_id, revision)`,
		// Acknowledged immutable records: doses, dose corrections, markers.
		`CREATE TABLE IF NOT EXISTS local_evidence_sync_records(record_id TEXT PRIMARY KEY, kind TEXT NOT NULL, pushed_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS local_sync_erased_medications(medication_id TEXT PRIMARY KEY)`,
		`CREATE TABLE IF NOT EXISTS local_sync_deferred_medication(
			record_id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			parent_id TEXT NOT NULL,
			payload_json BLOB NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_sync_deferred_medication_parent ON local_sync_deferred_medication(kind, parent_id)`,
		// Two devices may each correct the same dose while apart, so synced
		// corrections can follow the same predecessor. The effective dose
		// applies every correction in creation order; a local correction still
		// has to follow the latest one.
		`DROP INDEX IF EXISTS idx_local_medication_corrections_supersedes`,
		`DROP INDEX IF EXISTS idx_local_medication_corrections_root`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func medicationRevisionRecordID(medicationID string, revision int) string {
	return fmt.Sprintf("%s_r%d", medicationID, revision)
}

// splitRevisionRecordID reads "<id>_r<n>".
func splitRevisionRecordID(recordID string) (string, int, bool) {
	match := taskRevisionIDPattern.FindStringSubmatch(recordID)
	if match == nil {
		return "", 0, false
	}
	revision, err := strconv.Atoi(recordID[len(match[1])+2:])
	if err != nil || revision < 1 {
		return "", 0, false
	}
	return match[1], revision, true
}

// Definitions go first, then doses, then corrections, so a record never
// reaches the server before the one it belongs to.
const pendingMedicationSyncRecordsFrom = `FROM (
	SELECT 0 AS rank, medication.medication_id || '_r' || medication.revision AS record_id,
		'medication' AS kind, medication.updated_at AS created_at, medication.payload_json AS payload_json
	FROM local_medications AS medication
	WHERE NOT EXISTS(SELECT 1 FROM local_medication_sync_records AS synced
		WHERE synced.record_id = medication.medication_id || '_r' || medication.revision)
	UNION ALL
	SELECT 1, event.event_id, 'medication_event', event.recorded_at, event.payload_json
	FROM local_medication_events AS event
	WHERE NOT EXISTS(SELECT 1 FROM local_evidence_sync_records AS synced WHERE synced.record_id = event.event_id)
	UNION ALL
	SELECT 2, correction.correction_id, 'medication_correction', correction.created_at, correction.payload_json
	FROM local_medication_event_corrections AS correction
	WHERE NOT EXISTS(SELECT 1 FROM local_evidence_sync_records AS synced WHERE synced.record_id = correction.correction_id)
) AS pending`

// PendingMedicationSyncRecords returns the current revision of every definition
// edited since its last acknowledgment, and every unacknowledged dose and
// correction, in dependency order.
func (s *Store) PendingMedicationSyncRecords(ctx context.Context, limit int) ([]MedicationSyncRecord, error) {
	if limit < 1 || limit > MaxMedicationSyncPageSize {
		return nil, fmt.Errorf("medication sync page limit must be between 1 and %d", MaxMedicationSyncPageSize)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT record_id, kind, created_at, payload_json `+
		pendingMedicationSyncRecordsFrom+` ORDER BY rank, created_at, record_id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pending []MedicationSyncRecord
	for rows.Next() {
		var record MedicationSyncRecord
		var createdAt string
		if err := rows.Scan(&record.RecordID, &record.Kind, &createdAt, &record.Payload); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("pending %s %s: %w", record.Kind, record.RecordID, err)
		}
		record.CreatedAt = parsed.UTC()
		pending = append(pending, record)
	}
	return pending, rows.Err()
}

func (s *Store) PendingMedicationSyncRecordCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) `+pendingMedicationSyncRecordsFrom).Scan(&count)
	return count, err
}

// MarkMedicationSyncRecordsPushed records the server's acknowledgment. A dose
// or correction deleted while its upload was in flight already has its erasure
// queued. A definition deleted meanwhile has its uploaded revision queued now,
// since the server holds it.
func (s *Store) MarkMedicationSyncRecordsPushed(ctx context.Context, records []MedicationSyncRecord, pushedAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, record := range records {
		switch record.Kind {
		case SyncKindMedication:
			var medication MedicationRecord
			if err := json.Unmarshal(record.Payload, &medication); err != nil {
				return fmt.Errorf("pushed medication %s: %w", record.RecordID, err)
			}
			var erased bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM local_sync_erased_medications WHERE medication_id = ?)`,
				medication.MedicationID).Scan(&erased); err != nil {
				return err
			}
			if erased {
				if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO local_sleep_erasures(record_id, erased_at) VALUES(?, ?)`,
					record.RecordID, formatSQLiteTime(pushedAt)); err != nil {
					return err
				}
				continue
			}
			if err := acknowledgeMedicationRevisionTx(ctx, tx, medication, record.Payload, pushedAt); err != nil {
				return err
			}
		case SyncKindMedicationEvent, SyncKindMedicationCorrection:
			if err := acknowledgeEvidenceTx(ctx, tx, record.Kind, record.RecordID, pushedAt); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported medication sync kind %q", record.Kind)
		}
	}
	return tx.Commit()
}

func acknowledgeMedicationRevisionTx(ctx context.Context, tx *sql.Tx, record MedicationRecord, payload []byte, at time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO local_medication_sync_records(record_id, medication_id, revision, payload_json, pushed_at)
		VALUES(?, ?, ?, ?, ?)`,
		medicationRevisionRecordID(record.MedicationID, record.Revision), record.MedicationID, record.Revision, payload, formatSQLiteTime(at))
	return err
}

var evidenceTables = map[string]struct{ table, column string }{
	SyncKindMedicationEvent:      {"local_medication_events", "event_id"},
	SyncKindMedicationCorrection: {"local_medication_event_corrections", "correction_id"},
	SyncKindContextMarker:        {"local_rhythm_markers", "marker_id"},
}

// acknowledgeEvidenceTx records that the server holds an immutable record,
// unless the record has been deleted here since.
func acknowledgeEvidenceTx(ctx context.Context, tx *sql.Tx, kind, recordID string, at time.Time) error {
	source, ok := evidenceTables[kind]
	if !ok {
		return fmt.Errorf("unsupported evidence kind %q", kind)
	}
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO local_evidence_sync_records(record_id, kind, pushed_at)
		SELECT ?, ?, ? WHERE EXISTS(SELECT 1 FROM `+source.table+` WHERE `+source.column+` = ?)`,
		recordID, kind, formatSQLiteTime(at), recordID)
	return err
}

// preparedMedicationSync is a downloaded definition revision, dose or
// correction, validated and canonically encoded.
type preparedMedicationSync struct {
	kind       string
	recordID   string
	parentID   string // the definition of a dose, the dose of a correction
	medication MedicationRecord
	event      MedicationEventRecord
	correction MedicationEventCorrectionRecord
	encoded    []byte
}

func prepareSyncedMedication(record MedicationRecord) (preparedMedicationSync, error) {
	record = normalizeMedicationRecord(record)
	if err := record.Validate(); err != nil {
		return preparedMedicationSync{}, err
	}
	encoded, err := json.Marshal(record)
	return preparedMedicationSync{
		kind: SyncKindMedication, recordID: medicationRevisionRecordID(record.MedicationID, record.Revision),
		parentID: record.MedicationID, medication: record, encoded: encoded,
	}, err
}

func prepareSyncedMedicationEvent(record MedicationEventRecord) (preparedMedicationSync, error) {
	record = normalizeMedicationEvent(record)
	if err := record.Validate(); err != nil {
		return preparedMedicationSync{}, err
	}
	encoded, err := json.Marshal(record)
	return preparedMedicationSync{
		kind: SyncKindMedicationEvent, recordID: record.EventID,
		parentID: record.MedicationID, event: record, encoded: encoded,
	}, err
}

func prepareSyncedMedicationCorrection(record MedicationEventCorrectionRecord) (preparedMedicationSync, error) {
	record = normalizeMedicationCorrection(record)
	if err := record.Validate(); err != nil {
		return preparedMedicationSync{}, err
	}
	encoded, err := json.Marshal(record)
	return preparedMedicationSync{
		kind: SyncKindMedicationCorrection, recordID: record.CorrectionID,
		parentID: record.TargetEventID, correction: record, encoded: encoded,
	}, err
}

// applySyncedMedicationTx applies a downloaded definition revision. The
// highest revision wins; an unsent local edit that meets a revision it had
// not seen is rebased on top of it and stays pending, so it is uploaded next.
func applySyncedMedicationTx(ctx context.Context, tx *sql.Tx, prepared preparedMedicationSync, syncedAt time.Time) (bool, error) {
	remote := prepared.medication
	if suppressed, err := syncSuppressedTx(ctx, tx, remote.MedicationID, prepared.recordID); err != nil || suppressed {
		return false, err
	}
	var localPayload []byte
	err := tx.QueryRowContext(ctx, `SELECT payload_json FROM local_medications WHERE medication_id = ?`, remote.MedicationID).Scan(&localPayload)
	if errors.Is(err, sql.ErrNoRows) {
		if err := writeMedicationTx(ctx, tx, remote, prepared.encoded); err != nil {
			return false, err
		}
		return true, acknowledgeMedicationRevisionTx(ctx, tx, remote, prepared.encoded, syncedAt)
	}
	if err != nil {
		return false, err
	}
	var local MedicationRecord
	if err := json.Unmarshal(localPayload, &local); err != nil {
		return false, err
	}
	var localAcknowledged bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM local_medication_sync_records WHERE record_id = ?)`,
		medicationRevisionRecordID(local.MedicationID, local.Revision)).Scan(&localAcknowledged); err != nil {
		return false, err
	}
	baseRevision, basePayload, err := medicationSyncBaseTx(ctx, tx, remote.MedicationID)
	if err != nil {
		return false, err
	}
	var next MedicationRecord
	switch {
	case !localAcknowledged && remote.Revision == local.Revision && bytes.Equal(prepared.encoded, localPayload):
		// This device's own upload, whose acknowledgment was lost.
	case localAcknowledged || remote.Revision <= baseRevision:
		// Nothing unsent here, or a revision the local edit already builds on.
		if localAcknowledged && remote.Revision > local.Revision {
			next = remote
		}
	default:
		next, err = rebaseMedication(basePayload, local, localPayload, remote, prepared.encoded)
		if err != nil {
			return false, err
		}
	}
	if err := acknowledgeMedicationRevisionTx(ctx, tx, remote, prepared.encoded, syncedAt); err != nil {
		return false, err
	}
	if next.MedicationID == "" {
		return false, nil
	}
	encoded := prepared.encoded
	if next.Revision != remote.Revision {
		if encoded, err = json.Marshal(next); err != nil {
			return false, err
		}
	}
	return true, writeMedicationTx(ctx, tx, next, encoded)
}

// rebaseMedication replays an unsent local edit on top of a revision it had
// not seen. A field the edit changed from its base keeps the local value;
// every other field takes the remote value. If that combination is not a
// valid definition, the local edit wins whole. If it equals the remote
// revision, the remote revision is adopted as it is.
func rebaseMedication(base []byte, local MedicationRecord, localPayload []byte, remote MedicationRecord, remotePayload []byte) (MedicationRecord, error) {
	fields := func(data []byte) (map[string]json.RawMessage, error) {
		values := map[string]json.RawMessage{}
		if len(data) == 0 {
			return values, nil
		}
		return values, json.Unmarshal(data, &values)
	}
	baseFields, err := fields(base)
	if err != nil {
		return MedicationRecord{}, err
	}
	localFields, err := fields(localPayload)
	if err != nil {
		return MedicationRecord{}, err
	}
	remoteFields, err := fields(remotePayload)
	if err != nil {
		return MedicationRecord{}, err
	}
	merged := make(map[string]json.RawMessage, len(remoteFields))
	for key, value := range remoteFields {
		merged[key] = value
	}
	changed := false
	for _, source := range []map[string]json.RawMessage{localFields, baseFields} {
		for key := range source {
			if key == "revision" || key == "updated_at" {
				continue
			}
			localValue, inLocal := localFields[key]
			baseValue, inBase := baseFields[key]
			if inLocal == inBase && bytes.Equal(localValue, baseValue) {
				continue
			}
			remoteValue, inRemote := remoteFields[key]
			if inLocal != inRemote || !bytes.Equal(localValue, remoteValue) {
				changed = true
			}
			if inLocal {
				merged[key] = localValue
			} else {
				delete(merged, key)
			}
		}
	}
	if !changed {
		return remote, nil
	}
	encoded, err := json.Marshal(merged)
	if err != nil {
		return MedicationRecord{}, err
	}
	var rebased MedicationRecord
	if err := json.Unmarshal(encoded, &rebased); err != nil {
		return MedicationRecord{}, err
	}
	rebased = normalizeMedicationRecord(rebased)
	revision := max(local.Revision, remote.Revision) + 1
	updatedAt := local.UpdatedAt
	if remote.UpdatedAt.After(updatedAt) {
		updatedAt = remote.UpdatedAt
	}
	rebased.Revision, rebased.UpdatedAt = revision, updatedAt
	if rebased.Validate() != nil {
		rebased = local
		rebased.Revision, rebased.UpdatedAt = revision, updatedAt
	}
	return rebased, nil
}

func medicationSyncBaseTx(ctx context.Context, tx *sql.Tx, medicationID string) (int, []byte, error) {
	var revision int
	var payload []byte
	err := tx.QueryRowContext(ctx, `SELECT revision, payload_json FROM local_medication_sync_records
		WHERE medication_id = ? ORDER BY revision DESC LIMIT 1`, medicationID).Scan(&revision, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, nil
	}
	return revision, payload, err
}

func writeMedicationTx(ctx context.Context, tx *sql.Tx, record MedicationRecord, encoded []byte) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO local_medications(medication_id, active, revision, created_at, updated_at, payload_json)
		VALUES(?, ?, ?, ?, ?, ?)
		ON CONFLICT(medication_id) DO UPDATE SET active = excluded.active, revision = excluded.revision,
			updated_at = excluded.updated_at, payload_json = excluded.payload_json`,
		record.MedicationID, boolInt(record.Active), record.Revision,
		formatSQLiteTime(record.CreatedAt), formatSQLiteTime(record.UpdatedAt), encoded)
	return err
}

// insertSyncedMedicationEvidenceTx applies a downloaded dose or correction.
// Both are immutable: a second copy must match the first byte for byte.
func insertSyncedMedicationEvidenceTx(ctx context.Context, tx *sql.Tx, prepared preparedMedicationSync, syncedAt time.Time) (bool, error) {
	var suppressed bool
	var err error
	var parentQuery, insert string
	var args []any
	switch prepared.kind {
	case SyncKindMedicationEvent:
		event := prepared.event
		suppressed, err = syncSuppressedTx(ctx, tx, event.MedicationID, event.EventID)
		parentQuery = `SELECT EXISTS(SELECT 1 FROM local_medications WHERE medication_id = ?)`
		insert = `INSERT OR IGNORE INTO local_medication_events(
			event_id, medication_id, dose_at, status, scheduled, recorded_at, payload_json
		) VALUES(?, ?, ?, ?, ?, ?, ?)`
		args = []any{event.EventID, event.MedicationID, formatSQLiteTime(event.DoseAt), event.Status,
			boolInt(event.Scheduled), formatSQLiteTime(event.Provenance.RecordedAt), prepared.encoded}
	case SyncKindMedicationCorrection:
		correction := prepared.correction
		suppressed, err = syncSuppressedTx(ctx, tx, "", correction.CorrectionID, correction.TargetEventID)
		parentQuery = `SELECT EXISTS(SELECT 1 FROM local_medication_events WHERE event_id = ?)`
		changes, encodeErr := json.Marshal(correction.Changes)
		if encodeErr != nil {
			return false, encodeErr
		}
		insert = `INSERT OR IGNORE INTO local_medication_event_corrections(
			correction_id, target_event_id, supersedes_correction_id, created_at, reason, changes_json, payload_json
		) VALUES(?, ?, ?, ?, ?, ?, ?)`
		args = []any{correction.CorrectionID, correction.TargetEventID, correction.SupersedesCorrectionID,
			formatSQLiteTime(correction.CreatedAt), correction.Reason, changes, prepared.encoded}
	default:
		return false, fmt.Errorf("unsupported medication evidence kind %q", prepared.kind)
	}
	if err != nil || suppressed {
		return false, err
	}
	var parentExists bool
	if err := tx.QueryRowContext(ctx, parentQuery, prepared.parentID).Scan(&parentExists); err != nil {
		return false, err
	}
	if !parentExists {
		return false, errSyncParentMissing
	}
	result, err := tx.ExecContext(ctx, insert, args...)
	if err != nil {
		return false, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if inserted == 0 {
		source := evidenceTables[prepared.kind]
		var existing []byte
		if err := tx.QueryRowContext(ctx, `SELECT payload_json FROM `+source.table+` WHERE `+source.column+` = ?`,
			prepared.recordID).Scan(&existing); err != nil || !bytes.Equal(existing, prepared.encoded) {
			return false, fmt.Errorf("downloaded %s conflicts with an immutable local record", prepared.kind)
		}
	}
	return inserted > 0, acknowledgeEvidenceTx(ctx, tx, prepared.kind, prepared.recordID, syncedAt)
}

func deferMedicationRecordTx(ctx context.Context, tx *sql.Tx, prepared preparedMedicationSync) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO local_sync_deferred_medication(record_id, kind, parent_id, payload_json)
		VALUES(?, ?, ?, ?) ON CONFLICT(record_id) DO NOTHING`,
		prepared.recordID, prepared.kind, prepared.parentID, prepared.encoded)
	return err
}

// applyDeferredMedicationRecordsTx applies waiting doses whose definitions have
// arrived, then waiting corrections whose doses have.
func applyDeferredMedicationRecordsTx(ctx context.Context, tx *sql.Tx, syncedAt time.Time) (int, error) {
	applied := 0
	for _, step := range []struct{ kind, parents string }{
		{SyncKindMedicationEvent, `SELECT medication_id FROM local_medications`},
		{SyncKindMedicationCorrection, `SELECT event_id FROM local_medication_events`},
	} {
		var ready []preparedMedicationSync
		if err := readJSONRows(ctx, tx, `SELECT payload_json FROM local_sync_deferred_medication
			WHERE kind = ? AND parent_id IN (`+step.parents+`) ORDER BY record_id LIMIT 500`, func(data []byte) error {
			var prepared preparedMedicationSync
			var err error
			if step.kind == SyncKindMedicationEvent {
				var event MedicationEventRecord
				if err = json.Unmarshal(data, &event); err == nil {
					prepared, err = prepareSyncedMedicationEvent(event)
				}
			} else {
				var correction MedicationEventCorrectionRecord
				if err = json.Unmarshal(data, &correction); err == nil {
					prepared, err = prepareSyncedMedicationCorrection(correction)
				}
			}
			ready = append(ready, prepared)
			return err
		}, step.kind); err != nil {
			return applied, err
		}
		for _, prepared := range ready {
			inserted, err := insertSyncedMedicationEvidenceTx(ctx, tx, prepared, syncedAt)
			if err != nil {
				return applied, err
			}
			if inserted {
				applied++
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM local_sync_deferred_medication WHERE record_id = ?`, prepared.recordID); err != nil {
				return applied, err
			}
		}
	}
	return applied, nil
}

// eraseMedicationTx deletes a definition with every revision, dose and
// correction of it, and refuses them from now on. It reports whether the
// definition existed here.
func eraseMedicationTx(ctx context.Context, tx *sql.Tx, medicationID string, origin erasureOrigin) (bool, error) {
	var records []erasedRecord
	if err := collectErasedRecordsTx(ctx, tx, &records, `SELECT record_id, 'medication' FROM local_medication_sync_records WHERE medication_id = ?
		UNION SELECT medication_id || '_r' || revision, 'medication' FROM local_medications WHERE medication_id = ?`,
		medicationID, medicationID); err != nil {
		return false, err
	}
	if err := collectErasedRecordsTx(ctx, tx, &records, `WITH doses(id) AS (
			SELECT event_id FROM local_medication_events WHERE medication_id = ?
			UNION SELECT record_id FROM local_sync_deferred_medication WHERE kind = 'medication_event' AND parent_id = ?)
		SELECT id, 'medication_event' FROM doses
		UNION SELECT correction_id, 'medication_correction' FROM local_medication_event_corrections WHERE target_event_id IN (SELECT id FROM doses)
		UNION SELECT record_id, 'medication_correction' FROM local_sync_deferred_medication
			WHERE kind = 'medication_correction' AND parent_id IN (SELECT id FROM doses)`,
		medicationID, medicationID); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO local_sync_erased_medications(medication_id) VALUES(?)`, medicationID); err != nil {
		return false, err
	}
	if err := settleErasureTx(ctx, tx, records, origin); err != nil {
		return false, err
	}
	return deleteRowTx(ctx, tx, `DELETE FROM local_medications WHERE medication_id = ?`, medicationID)
}

// eraseMedicationEventTx deletes a dose with its corrections.
func eraseMedicationEventTx(ctx context.Context, tx *sql.Tx, eventID string, origin erasureOrigin) (bool, error) {
	records := []erasedRecord{{eventID, SyncKindMedicationEvent}}
	if err := collectErasedRecordsTx(ctx, tx, &records, `SELECT correction_id, 'medication_correction' FROM local_medication_event_corrections WHERE target_event_id = ?
		UNION SELECT record_id, 'medication_correction' FROM local_sync_deferred_medication WHERE kind = 'medication_correction' AND parent_id = ?`,
		eventID, eventID); err != nil {
		return false, err
	}
	if err := settleErasureTx(ctx, tx, records, origin); err != nil {
		return false, err
	}
	return deleteRowTx(ctx, tx, `DELETE FROM local_medication_events WHERE event_id = ?`, eventID)
}

func eraseMedicationCorrectionTx(ctx context.Context, tx *sql.Tx, correctionID string, origin erasureOrigin) (bool, error) {
	if err := settleErasureTx(ctx, tx, []erasedRecord{{correctionID, SyncKindMedicationCorrection}}, origin); err != nil {
		return false, err
	}
	return deleteRowTx(ctx, tx, `DELETE FROM local_medication_event_corrections WHERE correction_id = ?`, correctionID)
}
