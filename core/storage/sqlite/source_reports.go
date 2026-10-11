package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"non24.app/core/sources"
)

// Source reports (ADR-0054): what a phone found when it checked Health
// Connect. They arrive from the server, are kept as sent, and read back as
// each source's status. None of them changes an estimate.

const SyncKindSourceReport = "source_report"

// SyncPullSourceReport is a downloaded source report and the device that sent
// it: a status is per device, so the sender is kept.
type SyncPullSourceReport struct {
	DeviceID string
	Report   sources.Report
}

func (SyncPullSourceReport) isSyncPullRecord() {}

type preparedSourceReport struct {
	deviceID string
	report   sources.Report
	encoded  []byte
}

func prepareSyncedSourceReport(value SyncPullSourceReport) (preparedSourceReport, error) {
	if !contractIdentifier.MatchString(value.DeviceID) {
		return preparedSourceReport{}, errors.New("source report device id is not a valid identifier")
	}
	if err := value.Report.Validate(); err != nil {
		return preparedSourceReport{}, fmt.Errorf("source report: %w", err)
	}
	encoded, err := json.Marshal(value.Report)
	return preparedSourceReport{deviceID: value.DeviceID, report: value.Report, encoded: encoded}, err
}

// insertSyncedSourceReportTx keeps a downloaded report. A second copy must
// match the first byte for byte, and an erased report is not kept again.
func insertSyncedSourceReportTx(ctx context.Context, tx *sql.Tx, prepared preparedSourceReport) (bool, error) {
	report := prepared.report
	if suppressed, err := syncSuppressedTx(ctx, tx, "", report.ReportID); err != nil || suppressed {
		return false, err
	}
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO local_source_reports(
			report_id, device_id, source, outcome, reported_at, payload_json)
		VALUES(?, ?, ?, ?, ?, ?)`,
		report.ReportID, prepared.deviceID, report.Source, report.Outcome,
		formatSQLiteTime(report.ReportedAt), prepared.encoded)
	if err != nil {
		return false, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if inserted == 0 {
		var existing []byte
		err := tx.QueryRowContext(ctx, `SELECT payload_json FROM local_source_reports WHERE report_id = ?`, report.ReportID).Scan(&existing)
		if err != nil || !bytes.Equal(existing, prepared.encoded) {
			return false, errors.New("downloaded source report conflicts with the one kept")
		}
	}
	return inserted > 0, nil
}

func eraseSourceReportTx(ctx context.Context, tx *sql.Tx, reportID string, origin erasureOrigin) (bool, error) {
	if err := settleErasureTx(ctx, tx, []erasedRecord{{reportID, SyncKindSourceReport}}, origin); err != nil {
		return false, err
	}
	return deleteRowTx(ctx, tx, `DELETE FROM local_source_reports WHERE report_id = ?`, reportID)
}

// eraseAllSourceReportsTx erases every report kept here, on the server too.
func eraseAllSourceReportsTx(ctx context.Context, tx *sql.Tx) error {
	var reports []erasedRecord
	if err := collectErasedRecordsTx(ctx, tx, &reports,
		`SELECT report_id, ? FROM local_source_reports`, SyncKindSourceReport); err != nil {
		return err
	}
	if err := settleErasureTx(ctx, tx, reports, erasedHere); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM local_source_reports`)
	return err
}

// SourceReports returns every report kept, with the device that sent it.
func (s *Store) SourceReports(ctx context.Context) ([]sources.Received, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT device_id, payload_json FROM local_source_reports
		ORDER BY reported_at, report_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var received []sources.Received
	for rows.Next() {
		var item sources.Received
		var payload []byte
		if err := rows.Scan(&item.DeviceID, &payload); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(payload, &item.Report); err != nil {
			return nil, fmt.Errorf("stored source report: %w", err)
		}
		received = append(received, item)
	}
	return received, rows.Err()
}

// NewestSleepEndFrom is when the newest sleep record a source sent ends, as
// the source recorded it; zero when it has sent none.
func (s *Store) NewestSleepEndFrom(ctx context.Context, acquisitionMethod string) (time.Time, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT end_at FROM local_sleep_observations
		WHERE acquisition_method = ? ORDER BY start_at DESC LIMIT 10`, acquisitionMethod)
	if err != nil {
		return time.Time{}, err
	}
	defer rows.Close()
	var newest time.Time
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return time.Time{}, err
		}
		end, err := time.Parse(time.RFC3339Nano, text)
		if err != nil {
			return time.Time{}, fmt.Errorf("stored sleep end: %w", err)
		}
		if end.After(newest) {
			newest = end.UTC()
		}
	}
	return newest, rows.Err()
}
