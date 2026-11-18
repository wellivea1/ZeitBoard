package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"non24.app/core/presence"
)

// Reported statuses (ADR-0070): what the owner says about the present. Each is
// kept as it was reported, uploaded to the owner's server, and received from
// their other devices. The newest one stands. None of them is an estimator
// input.

const (
	SyncKindStatus        = "status"
	MaxStatusSyncPageSize = 100

	// statusReportsKept is how long a report is kept after it was made. A
	// status is about one moment; a history of them would be a diary nobody
	// asked ZeitBoard to keep, on this computer and on the server.
	statusReportsKept = 7 * 24 * time.Hour
)

// StatusSyncRecord is one report ready for the push endpoint.
type StatusSyncRecord struct {
	RecordID  string
	CreatedAt time.Time
	Payload   []byte
}

// SyncPullStatus is a downloaded report.
type SyncPullStatus struct {
	Report presence.Report
}

func (SyncPullStatus) isSyncPullRecord() {}

type preparedStatusReport struct {
	report  presence.Report
	encoded []byte
}

func prepareStatusReport(report presence.Report) (preparedStatusReport, error) {
	report.ReportedAt = report.ReportedAt.UTC()
	if report.ExpiresAt != nil {
		expires := report.ExpiresAt.UTC()
		report.ExpiresAt = &expires
	}
	if err := report.Validate(); err != nil {
		return preparedStatusReport{}, fmt.Errorf("status: %w", err)
	}
	encoded, err := json.Marshal(report)
	return preparedStatusReport{report: report, encoded: encoded}, err
}

func insertStatusReportTx(ctx context.Context, tx *sql.Tx, prepared preparedStatusReport) (bool, error) {
	report := prepared.report
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO local_status_reports(
			status_id, status, reported_at, payload_json)
		VALUES(?, ?, ?, ?)`,
		report.StatusID, report.Status, formatSQLiteTime(report.ReportedAt), prepared.encoded)
	if err != nil {
		return false, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if inserted == 0 {
		var existing []byte
		err := tx.QueryRowContext(ctx, `SELECT payload_json FROM local_status_reports WHERE status_id = ?`, report.StatusID).Scan(&existing)
		if err != nil || !bytes.Equal(existing, prepared.encoded) {
			return false, errors.New("a status report with this id is already kept, and differs")
		}
	}
	return inserted > 0, nil
}

// AddStatusReport keeps a status the owner reported on this device, and lets
// go of reports older than a week, here and on the server.
func (s *Store) AddStatusReport(ctx context.Context, report presence.Report, now time.Time) error {
	prepared, err := prepareStatusReport(report)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := insertStatusReportTx(ctx, tx, prepared); err != nil {
		return err
	}
	var old []erasedRecord
	if err := collectErasedRecordsTx(ctx, tx, &old,
		`SELECT status_id, ? FROM local_status_reports WHERE reported_at < ?`,
		SyncKindStatus, formatSQLiteTime(now.UTC().Add(-statusReportsKept))); err != nil {
		return err
	}
	if err := settleErasureTx(ctx, tx, old, erasedHere); err != nil {
		return err
	}
	for _, record := range old {
		if _, err := tx.ExecContext(ctx, `DELETE FROM local_status_reports WHERE status_id = ?`, record.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// StatusReports returns every report kept, oldest first.
func (s *Store) StatusReports(ctx context.Context) ([]presence.Report, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload_json FROM local_status_reports ORDER BY reported_at, status_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var reports []presence.Report
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var report presence.Report
		if err := json.Unmarshal(payload, &report); err != nil {
			return nil, fmt.Errorf("stored status report: %w", err)
		}
		reports = append(reports, report)
	}
	return reports, rows.Err()
}

// StatusReportUploaded reports whether the owner's server holds a report.
func (s *Store) StatusReportUploaded(ctx context.Context, statusID string) (bool, error) {
	var uploaded bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM local_evidence_sync_records WHERE record_id = ? AND kind = ?)`,
		statusID, SyncKindStatus).Scan(&uploaded)
	return uploaded, err
}

// PendingStatusSyncRecords returns reports the server has not acknowledged, in
// the order they were made.
func (s *Store) PendingStatusSyncRecords(ctx context.Context, limit int) ([]StatusSyncRecord, error) {
	if limit < 1 || limit > MaxStatusSyncPageSize {
		return nil, fmt.Errorf("status sync page limit must be between 1 and %d", MaxStatusSyncPageSize)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT report.status_id, report.reported_at, report.payload_json
		FROM local_status_reports AS report
		WHERE NOT EXISTS(SELECT 1 FROM local_evidence_sync_records AS synced WHERE synced.record_id = report.status_id)
		ORDER BY report.reported_at, report.status_id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pending []StatusSyncRecord
	for rows.Next() {
		var record StatusSyncRecord
		var reportedAt string
		if err := rows.Scan(&record.RecordID, &reportedAt, &record.Payload); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, reportedAt)
		if err != nil {
			return nil, fmt.Errorf("pending status %s: %w", record.RecordID, err)
		}
		record.CreatedAt = parsed.UTC()
		pending = append(pending, record)
	}
	return pending, rows.Err()
}

// MarkStatusSyncRecordsPushed records that the server holds these reports.
func (s *Store) MarkStatusSyncRecordsPushed(ctx context.Context, records []StatusSyncRecord, pushedAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, record := range records {
		if err := acknowledgeEvidenceTx(ctx, tx, SyncKindStatus, record.RecordID, pushedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// insertSyncedStatusTx keeps a downloaded report. A second copy must match the
// first byte for byte, and an erased report is not kept again.
func insertSyncedStatusTx(ctx context.Context, tx *sql.Tx, prepared preparedStatusReport, syncedAt time.Time) (bool, error) {
	if suppressed, err := syncSuppressedTx(ctx, tx, "", prepared.report.StatusID); err != nil || suppressed {
		return false, err
	}
	inserted, err := insertStatusReportTx(ctx, tx, prepared)
	if err != nil {
		return false, err
	}
	return inserted, acknowledgeEvidenceTx(ctx, tx, SyncKindStatus, prepared.report.StatusID, syncedAt)
}

func eraseStatusReportTx(ctx context.Context, tx *sql.Tx, statusID string, origin erasureOrigin) (bool, error) {
	if err := settleErasureTx(ctx, tx, []erasedRecord{{statusID, SyncKindStatus}}, origin); err != nil {
		return false, err
	}
	return deleteRowTx(ctx, tx, `DELETE FROM local_status_reports WHERE status_id = ?`, statusID)
}

// eraseAllStatusReportsTx erases every report kept here, on the server too.
func eraseAllStatusReportsTx(ctx context.Context, tx *sql.Tx) error {
	var reports []erasedRecord
	if err := collectErasedRecordsTx(ctx, tx, &reports,
		`SELECT status_id, ? FROM local_status_reports`, SyncKindStatus); err != nil {
		return err
	}
	if err := settleErasureTx(ctx, tx, reports, erasedHere); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM local_status_reports`)
	return err
}

// DeleteAllStatusReports erases every report, here and on the server.
func (s *Store) DeleteAllStatusReports(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := eraseAllStatusReportsTx(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}
