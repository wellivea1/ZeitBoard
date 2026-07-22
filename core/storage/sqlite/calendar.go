package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	calendarcore "non24.app/core/calendar"
	"non24.app/core/domain"
)

const ZeitBoardCalendarSourceID = "calendar_source_zeitboard"

const (
	ProposalApproved = "approved"
	ProposalRejected = "rejected"
	ProposalUndone   = "undone"
)

var (
	ErrCalendarSourceNotFound = errors.New("calendar source does not exist")
	ErrCalendarSourceReadOnly = errors.New("imported calendar events are read-only")
	ErrProposalAlreadyDecided = errors.New("proposal already has an active decision")
	ErrProposalNotDecided     = errors.New("proposal has no active decision")
	ErrStaleProposal          = errors.New("proposal inputs changed; refresh proposals")
)

type CalendarSourceRecord struct {
	calendarcore.Source
	Endpoint string `json:"endpoint,omitempty"`
}

type ProposalDecisionInput struct {
	DecisionID        string
	ProposalID        string
	TaskID            string
	TaskRevision      int
	EstimateID        string
	ProposalTitle     string
	ProposalStartAt   time.Time
	ProposalEndAt     time.Time
	ZoneID            string
	Confidence        string
	ExplanationCodes  []string
	Decision          string
	DecidedAt         time.Time
	SnapshotStartAt   time.Time
	SnapshotEndAt     time.Time
	EventSnapshotHash string
}

type ProposalDecisionRecord struct {
	DecisionID        string    `json:"decision_id"`
	ProposalID        string    `json:"proposal_id"`
	TaskID            string    `json:"task_id"`
	TaskRevision      int       `json:"task_revision"`
	EstimateID        string    `json:"estimate_id"`
	ProposalTitle     string    `json:"proposal_title"`
	ProposalStartAt   time.Time `json:"proposal_start_at"`
	ProposalEndAt     time.Time `json:"proposal_end_at"`
	ZoneID            string    `json:"zone_id"`
	Confidence        string    `json:"confidence"`
	ExplanationCodes  []string  `json:"explanation_codes"`
	Decision          string    `json:"decision"`
	DecidedAt         time.Time `json:"decided_at"`
	SupersedesID      string    `json:"supersedes_decision_id,omitempty"`
	EventID           string    `json:"event_id,omitempty"`
	SnapshotStartAt   time.Time `json:"snapshot_start_at"`
	SnapshotEndAt     time.Time `json:"snapshot_end_at"`
	EventSnapshotHash string    `json:"event_snapshot_hash"`
}

func (s *Store) ReplaceImportedCalendar(ctx context.Context, source calendarcore.Source, events []calendarcore.Event, endpoint string) error {
	if err := validateImportedCalendar(source, events, endpoint); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var existingKind string
	var existingReadOnly int
	err = tx.QueryRowContext(ctx,
		`SELECT kind, read_only FROM local_calendar_sources WHERE source_id = ?`, source.SourceID,
	).Scan(&existingKind, &existingReadOnly)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && (existingKind == string(calendarcore.SourceZeitBoard) || existingReadOnly != 1) {
		return ErrCalendarSourceReadOnly
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM local_calendar_events WHERE source_id = ?`, source.SourceID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO local_calendar_sources(
		source_id, label, kind, read_only, coverage_start_at, coverage_end_at, last_imported_at, endpoint
	) VALUES(?, ?, ?, 1, ?, ?, ?, ?)
	ON CONFLICT(source_id) DO UPDATE SET
		label = excluded.label,
		kind = excluded.kind,
		read_only = 1,
		coverage_start_at = excluded.coverage_start_at,
		coverage_end_at = excluded.coverage_end_at,
		last_imported_at = excluded.last_imported_at,
		endpoint = excluded.endpoint`,
		source.SourceID, source.Label, source.Kind,
		formatSQLiteTime(source.CoverageStartAt), formatSQLiteTime(source.CoverageEndAt),
		formatSQLiteTime(source.LastImportedAt), endpoint,
	); err != nil {
		return err
	}
	for _, event := range events {
		if err := insertCalendarEvent(ctx, tx, event); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) RemoveImportedCalendar(ctx context.Context, sourceID string) error {
	if !contractIdentifier.MatchString(sourceID) {
		return errors.New("source_id must match the v1 identifier format")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var kind string
	var readOnly int
	if err := tx.QueryRowContext(ctx,
		`SELECT kind, read_only FROM local_calendar_sources WHERE source_id = ?`, sourceID,
	).Scan(&kind, &readOnly); errors.Is(err, sql.ErrNoRows) {
		return ErrCalendarSourceNotFound
	} else if err != nil {
		return err
	}
	if kind == string(calendarcore.SourceZeitBoard) || readOnly != 1 {
		return ErrCalendarSourceReadOnly
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM local_calendar_sources WHERE source_id = ?`, sourceID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.compactDeletedData(ctx)
}

func (s *Store) ListCalendarSources(ctx context.Context) ([]CalendarSourceRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT
		source_id, label, kind, read_only, coverage_start_at, coverage_end_at, last_imported_at, endpoint
		FROM local_calendar_sources ORDER BY kind, label, source_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []CalendarSourceRecord
	for rows.Next() {
		var record CalendarSourceRecord
		var kind, coverageStart, coverageEnd, importedAt string
		var readOnly int
		if err := rows.Scan(
			&record.SourceID, &record.Label, &kind, &readOnly,
			&coverageStart, &coverageEnd, &importedAt, &record.Endpoint,
		); err != nil {
			return nil, err
		}
		record.Kind = calendarcore.SourceKind(kind)
		record.ReadOnly = readOnly == 1
		if record.CoverageStartAt, err = parseCalendarTime(coverageStart); err != nil {
			return nil, err
		}
		if record.CoverageEndAt, err = parseCalendarTime(coverageEnd); err != nil {
			return nil, err
		}
		if record.LastImportedAt, err = parseCalendarTime(importedAt); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) CalendarEvents(ctx context.Context, start, end time.Time) ([]calendarcore.Event, error) {
	if start.IsZero() || end.IsZero() || !start.Before(end) {
		return nil, errors.New("calendar query requires a non-empty interval")
	}
	rows, err := s.db.QueryContext(ctx, calendarEventsQuery,
		formatSQLiteTime(start), formatSQLiteTime(end), formatSQLiteTime(start), formatSQLiteTime(end))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []calendarcore.Event
	for rows.Next() {
		event, err := scanCalendarEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) OwnedCalendarEvents(ctx context.Context) ([]calendarcore.Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT
		event_id, source_id, source_record_id, title, start_at, end_at, zone_id,
		all_day, busy, ownership, created_at, location, notes, task_id, task_revision, proposal_id
		FROM local_calendar_events
		WHERE ownership = 'app_owned'
		ORDER BY start_at, end_at, event_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []calendarcore.Event
	for rows.Next() {
		event, err := scanCalendarEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) BusyDomainEvents(ctx context.Context, start, end time.Time, zoneID string) ([]domain.CalendarEvent, string, error) {
	if start.IsZero() || end.IsZero() || !start.Before(end) {
		return nil, "", errors.New("calendar query requires a non-empty interval")
	}
	return busyDomainEvents(ctx, s.db, start, end, zoneID)
}

// DecideProposal verifies the exact task revision and text-free busy-event
// fingerprint in the same transaction that records the decision. Approval also
// inserts the supplied app-owned block; rejection never writes an event.
func (s *Store) DecideProposal(ctx context.Context, input ProposalDecisionInput, ownedEvent *calendarcore.Event) (ProposalDecisionRecord, error) {
	if err := validateProposalDecisionInput(input); err != nil {
		return ProposalDecisionRecord{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ProposalDecisionRecord{}, err
	}
	defer tx.Rollback()

	latest, found, err := latestProposalDecision(ctx, tx, input.ProposalID)
	if err != nil {
		return ProposalDecisionRecord{}, err
	}
	if found && latest.Decision != ProposalUndone {
		return ProposalDecisionRecord{}, ErrProposalAlreadyDecided
	}

	task, err := taskByIDTx(ctx, tx, input.TaskID)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			return ProposalDecisionRecord{}, ErrStaleProposal
		}
		return ProposalDecisionRecord{}, err
	}
	if task.Status != TaskStatusOpen || effectiveRevision(task) != input.TaskRevision {
		return ProposalDecisionRecord{}, ErrStaleProposal
	}
	if input.ProposalTitle != task.Title {
		return ProposalDecisionRecord{}, ErrStaleProposal
	}
	_, fingerprint, err := busyDomainEvents(ctx, tx, input.SnapshotStartAt, input.SnapshotEndAt, "UTC")
	if err != nil {
		return ProposalDecisionRecord{}, err
	}
	if fingerprint != input.EventSnapshotHash {
		return ProposalDecisionRecord{}, ErrStaleProposal
	}

	record := ProposalDecisionRecord{
		DecisionID:        input.DecisionID,
		ProposalID:        input.ProposalID,
		TaskID:            input.TaskID,
		TaskRevision:      input.TaskRevision,
		EstimateID:        input.EstimateID,
		ProposalTitle:     input.ProposalTitle,
		ProposalStartAt:   input.ProposalStartAt.UTC(),
		ProposalEndAt:     input.ProposalEndAt.UTC(),
		ZoneID:            input.ZoneID,
		Confidence:        input.Confidence,
		ExplanationCodes:  append([]string(nil), input.ExplanationCodes...),
		Decision:          input.Decision,
		DecidedAt:         input.DecidedAt.UTC(),
		SnapshotStartAt:   input.SnapshotStartAt.UTC(),
		SnapshotEndAt:     input.SnapshotEndAt.UTC(),
		EventSnapshotHash: input.EventSnapshotHash,
	}
	if input.Decision == ProposalApproved {
		if ownedEvent == nil {
			return ProposalDecisionRecord{}, errors.New("approval requires an app-owned calendar event")
		}
		if err := validatePlacementEvent(*ownedEvent, input, task); err != nil {
			return ProposalDecisionRecord{}, err
		}
		if err := ensureZeitBoardSource(ctx, tx, input.DecidedAt); err != nil {
			return ProposalDecisionRecord{}, err
		}
		if err := insertCalendarEvent(ctx, tx, *ownedEvent); err != nil {
			return ProposalDecisionRecord{}, err
		}
		record.EventID = ownedEvent.EventID
	} else if ownedEvent != nil {
		return ProposalDecisionRecord{}, errors.New("rejection cannot include a calendar event")
	}
	if err := insertProposalDecision(ctx, tx, record); err != nil {
		return ProposalDecisionRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return ProposalDecisionRecord{}, err
	}
	return record, nil
}

func (s *Store) UndoProposalDecision(ctx context.Context, decisionID, proposalID string, decidedAt time.Time) (ProposalDecisionRecord, error) {
	if !contractIdentifier.MatchString(decisionID) || !contractIdentifier.MatchString(proposalID) || decidedAt.IsZero() {
		return ProposalDecisionRecord{}, errors.New("undo requires valid decision and proposal ids plus decided_at")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ProposalDecisionRecord{}, err
	}
	defer tx.Rollback()
	latest, found, err := latestProposalDecision(ctx, tx, proposalID)
	if err != nil {
		return ProposalDecisionRecord{}, err
	}
	if !found || latest.Decision == ProposalUndone {
		return ProposalDecisionRecord{}, ErrProposalNotDecided
	}
	if latest.Decision == ProposalApproved {
		result, err := tx.ExecContext(ctx, `DELETE FROM local_calendar_events
			WHERE event_id = ? AND proposal_id = ? AND ownership = 'app_owned'`, latest.EventID, proposalID)
		if err != nil {
			return ProposalDecisionRecord{}, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return ProposalDecisionRecord{}, err
		}
		if affected != 1 {
			return ProposalDecisionRecord{}, errors.New("approved placement block is missing")
		}
	}
	undo := latest
	undo.DecisionID = decisionID
	undo.Decision = ProposalUndone
	undo.DecidedAt = decidedAt.UTC()
	undo.SupersedesID = latest.DecisionID
	if err := insertProposalDecision(ctx, tx, undo); err != nil {
		return ProposalDecisionRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return ProposalDecisionRecord{}, err
	}
	return undo, nil
}

func (s *Store) ActiveProposalDecisions(ctx context.Context) ([]ProposalDecisionRecord, error) {
	rows, err := s.db.QueryContext(ctx, proposalDecisionSelect+` ORDER BY rowid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	active := make(map[string]ProposalDecisionRecord)
	for rows.Next() {
		record, err := scanProposalDecision(rows)
		if err != nil {
			return nil, err
		}
		if record.Decision == ProposalUndone {
			delete(active, record.ProposalID)
		} else {
			active[record.ProposalID] = record
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	records := make([]ProposalDecisionRecord, 0, len(active))
	for _, record := range active {
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].DecidedAt.Equal(records[j].DecidedAt) {
			return records[i].ProposalID < records[j].ProposalID
		}
		return records[i].DecidedAt.Before(records[j].DecidedAt)
	})
	return records, nil
}

const calendarEventsQuery = `SELECT
	event_id, source_id, source_record_id, title, start_at, end_at, zone_id,
	all_day, busy, ownership, created_at, location, notes, task_id, task_revision, proposal_id
	FROM local_calendar_events
	WHERE (end_at > ? AND start_at < ?)
		OR (start_at = end_at AND start_at >= ? AND start_at < ?)
	ORDER BY start_at, end_at, event_id`

const proposalDecisionSelect = `SELECT
	decision_id, proposal_id, task_id, task_revision, estimate_id,
	proposal_title, proposal_start_at, proposal_end_at, zone_id, confidence, explanation_codes_json,
	decision, decided_at,
	supersedes_decision_id, event_id, snapshot_start_at, snapshot_end_at, event_snapshot_hash
	FROM local_proposal_decisions`

type queryContext interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type rowScanner interface {
	Scan(...any) error
}

func validateImportedCalendar(source calendarcore.Source, events []calendarcore.Event, endpoint string) error {
	if err := source.Validate(); err != nil {
		return err
	}
	if source.Kind != calendarcore.SourceICS && source.Kind != calendarcore.SourceCalDAV {
		return errors.New("only imported calendar sources can be replaced")
	}
	if len(events) > calendarcore.MaxMaterializedEvents {
		return fmt.Errorf("calendar contains more than %d events", calendarcore.MaxMaterializedEvents)
	}
	if err := validateStoredEndpoint(source.Kind, endpoint); err != nil {
		return err
	}
	seenIDs := make(map[string]struct{}, len(events))
	seenRecords := make(map[string]struct{}, len(events))
	for _, event := range events {
		if err := event.Validate(); err != nil {
			return fmt.Errorf("event %q: %w", event.EventID, err)
		}
		if event.SourceID != source.SourceID || event.Ownership != calendarcore.OwnershipImported {
			return errors.New("imported events must belong to the replaced read-only source")
		}
		if _, found := seenIDs[event.EventID]; found {
			return fmt.Errorf("duplicate event id %q", event.EventID)
		}
		if _, found := seenRecords[event.SourceRecordID]; found {
			return fmt.Errorf("duplicate source record id %q", event.SourceRecordID)
		}
		seenIDs[event.EventID] = struct{}{}
		seenRecords[event.SourceRecordID] = struct{}{}
	}
	return nil
}

func validateStoredEndpoint(kind calendarcore.SourceKind, endpoint string) error {
	if kind == calendarcore.SourceICS {
		if endpoint != "" {
			return errors.New("file calendar source cannot store an endpoint")
		}
		return nil
	}
	if endpoint == "" {
		return errors.New("CalDAV source requires a sanitized endpoint")
	}
	parsed, err := url.ParseRequestURI(endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("CalDAV endpoint must be a sanitized absolute URL without credentials, query, or fragment")
	}
	if strings.EqualFold(parsed.Scheme, "https") {
		return nil
	}
	if strings.EqualFold(parsed.Scheme, "http") && isLoopbackHost(parsed.Hostname()) {
		return nil
	}
	return errors.New("CalDAV endpoint must use HTTPS except on loopback")
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func insertCalendarEvent(ctx context.Context, tx *sql.Tx, event calendarcore.Event) error {
	if err := event.Validate(); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO local_calendar_events(
		event_id, source_id, source_record_id, title, start_at, end_at, zone_id,
		all_day, busy, ownership, created_at, location, notes, task_id, task_revision, proposal_id
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.EventID, event.SourceID, event.SourceRecordID, event.Title,
		formatSQLiteTime(event.StartAt), formatSQLiteTime(event.EndAt), event.ZoneID,
		boolInt(event.AllDay), boolInt(event.Busy), event.Ownership, formatSQLiteTime(event.CreatedAt),
		event.Location, event.Notes, event.TaskID, event.TaskRevision, event.ProposalID,
	)
	return err
}

func scanCalendarEvent(scanner rowScanner) (calendarcore.Event, error) {
	var event calendarcore.Event
	var startAt, endAt, createdAt, ownership string
	var allDay, busy int
	if err := scanner.Scan(
		&event.EventID, &event.SourceID, &event.SourceRecordID, &event.Title,
		&startAt, &endAt, &event.ZoneID, &allDay, &busy, &ownership, &createdAt,
		&event.Location, &event.Notes, &event.TaskID, &event.TaskRevision, &event.ProposalID,
	); err != nil {
		return calendarcore.Event{}, err
	}
	var err error
	if event.StartAt, err = parseCalendarTime(startAt); err != nil {
		return calendarcore.Event{}, err
	}
	if event.EndAt, err = parseCalendarTime(endAt); err != nil {
		return calendarcore.Event{}, err
	}
	if event.CreatedAt, err = parseCalendarTime(createdAt); err != nil {
		return calendarcore.Event{}, err
	}
	event.AllDay = allDay == 1
	event.Busy = busy == 1
	event.Ownership = calendarcore.Ownership(ownership)
	return event, event.Validate()
}

func busyDomainEvents(ctx context.Context, query queryContext, start, end time.Time, zoneID string) ([]domain.CalendarEvent, string, error) {
	if _, err := time.LoadLocation(zoneID); err != nil {
		return nil, "", fmt.Errorf("calendar query zone: %w", err)
	}
	rows, err := query.QueryContext(ctx, `SELECT event_id, start_at, end_at
		FROM local_calendar_events
		WHERE busy = 1 AND end_at > start_at AND end_at > ? AND start_at < ?
		ORDER BY start_at, end_at, event_id`, formatSQLiteTime(start), formatSQLiteTime(end))
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	hash := sha256.New()
	var events []domain.CalendarEvent
	for rows.Next() {
		var eventID, rawStart, rawEnd string
		if err := rows.Scan(&eventID, &rawStart, &rawEnd); err != nil {
			return nil, "", err
		}
		startAt, err := parseCalendarTime(rawStart)
		if err != nil {
			return nil, "", err
		}
		endAt, err := parseCalendarTime(rawEnd)
		if err != nil {
			return nil, "", err
		}
		startInstant, err := domain.NewZonedInstant(startAt, zoneID)
		if err != nil {
			return nil, "", err
		}
		endInstant, err := domain.NewZonedInstant(endAt, zoneID)
		if err != nil {
			return nil, "", err
		}
		events = append(events, domain.CalendarEvent{
			ID:       domain.CalendarEventID(eventID),
			Interval: domain.TimeRange{Start: startInstant, End: endInstant},
			Fixed:    true,
		})
		hash.Write([]byte(eventID))
		hash.Write([]byte{0})
		hash.Write([]byte(rawStart))
		hash.Write([]byte{0})
		hash.Write([]byte(rawEnd))
		hash.Write([]byte{0})
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	return events, hex.EncodeToString(hash.Sum(nil)), nil
}

func validateProposalDecisionInput(input ProposalDecisionInput) error {
	for name, value := range map[string]string{
		"decision_id": input.DecisionID,
		"proposal_id": input.ProposalID,
		"task_id":     input.TaskID,
		"estimate_id": input.EstimateID,
	} {
		if !contractIdentifier.MatchString(value) {
			return fmt.Errorf("%s must match the v1 identifier format", name)
		}
	}
	if input.TaskRevision < 1 {
		return errors.New("task revision must be at least 1")
	}
	if strings.TrimSpace(input.ProposalTitle) == "" || len([]rune(input.ProposalTitle)) > 120 {
		return errors.New("proposal title must contain 1 to 120 characters")
	}
	if input.ProposalStartAt.IsZero() || input.ProposalEndAt.IsZero() || !input.ProposalStartAt.Before(input.ProposalEndAt) {
		return errors.New("proposal interval must be non-empty")
	}
	if _, err := time.LoadLocation(input.ZoneID); err != nil {
		return fmt.Errorf("proposal zone: %w", err)
	}
	if input.Confidence != "low" && input.Confidence != "medium" && input.Confidence != "high" {
		return errors.New("proposal confidence must be low, medium, or high")
	}
	if len(input.ExplanationCodes) == 0 || len(input.ExplanationCodes) > 16 {
		return errors.New("proposal requires 1 to 16 explanation codes")
	}
	for _, code := range input.ExplanationCodes {
		if !contractIdentifier.MatchString(code) {
			return fmt.Errorf("invalid proposal explanation code %q", code)
		}
	}
	if input.Decision != ProposalApproved && input.Decision != ProposalRejected {
		return errors.New("decision must be approved or rejected")
	}
	if input.DecidedAt.IsZero() || input.SnapshotStartAt.IsZero() || input.SnapshotEndAt.IsZero() || !input.SnapshotStartAt.Before(input.SnapshotEndAt) {
		return errors.New("decision and snapshot times are invalid")
	}
	decoded, err := hex.DecodeString(input.EventSnapshotHash)
	if err != nil || len(decoded) != sha256.Size || input.EventSnapshotHash != strings.ToLower(input.EventSnapshotHash) {
		return errors.New("event snapshot hash must be a lowercase SHA-256 digest")
	}
	return nil
}

func validatePlacementEvent(event calendarcore.Event, input ProposalDecisionInput, task TaskRecord) error {
	if err := event.Validate(); err != nil {
		return err
	}
	if event.Ownership != calendarcore.OwnershipAppOwned || event.SourceID != ZeitBoardCalendarSourceID || !event.Busy || event.AllDay {
		return errors.New("approved placement must be a busy timed event owned by the ZeitBoard source")
	}
	if event.TaskID != input.TaskID || event.TaskRevision != input.TaskRevision || event.ProposalID != input.ProposalID || event.SourceRecordID != input.ProposalID {
		return errors.New("approved placement links do not match the proposal decision")
	}
	if !event.StartAt.Equal(input.ProposalStartAt) || !event.EndAt.Equal(input.ProposalEndAt) || event.ZoneID != input.ZoneID {
		return errors.New("approved placement interval does not match the proposal")
	}
	if event.Title != task.Title || event.EndAt.Sub(event.StartAt) != time.Duration(task.DurationMinutes)*time.Minute {
		return errors.New("approved placement does not match the current task title and duration")
	}
	if event.StartAt.Before(input.SnapshotStartAt) || event.EndAt.After(input.SnapshotEndAt) {
		return errors.New("approved placement falls outside the verified calendar snapshot")
	}
	return nil
}

func ensureZeitBoardSource(ctx context.Context, tx *sql.Tx, createdAt time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO local_calendar_sources(
		source_id, label, kind, read_only, coverage_start_at, coverage_end_at, last_imported_at, endpoint
	) VALUES(?, 'ZeitBoard placements', 'zeitboard', 0, ?, ?, ?, '')`,
		ZeitBoardCalendarSourceID,
		"1970-01-01T00:00:00Z", "9999-12-31T23:59:59Z", formatSQLiteTime(createdAt),
	)
	return err
}

func insertProposalDecision(ctx context.Context, tx *sql.Tx, record ProposalDecisionRecord) error {
	explanationCodes, err := json.Marshal(record.ExplanationCodes)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO local_proposal_decisions(
		decision_id, proposal_id, task_id, task_revision, estimate_id,
		proposal_title, proposal_start_at, proposal_end_at, zone_id, confidence, explanation_codes_json,
		decision, decided_at,
		supersedes_decision_id, event_id, snapshot_start_at, snapshot_end_at, event_snapshot_hash
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.DecisionID, record.ProposalID, record.TaskID, record.TaskRevision,
		record.EstimateID, record.ProposalTitle, formatSQLiteTime(record.ProposalStartAt),
		formatSQLiteTime(record.ProposalEndAt), record.ZoneID, record.Confidence, explanationCodes,
		record.Decision, formatSQLiteTime(record.DecidedAt),
		record.SupersedesID, record.EventID, formatSQLiteTime(record.SnapshotStartAt),
		formatSQLiteTime(record.SnapshotEndAt), record.EventSnapshotHash,
	)
	return err
}

func latestProposalDecision(ctx context.Context, tx *sql.Tx, proposalID string) (ProposalDecisionRecord, bool, error) {
	row := tx.QueryRowContext(ctx, proposalDecisionSelect+` WHERE proposal_id = ? ORDER BY rowid DESC LIMIT 1`, proposalID)
	record, err := scanProposalDecision(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ProposalDecisionRecord{}, false, nil
	}
	return record, err == nil, err
}

func scanProposalDecision(scanner rowScanner) (ProposalDecisionRecord, error) {
	var record ProposalDecisionRecord
	var proposalStart, proposalEnd, decidedAt, snapshotStart, snapshotEnd string
	var explanationCodes []byte
	if err := scanner.Scan(
		&record.DecisionID, &record.ProposalID, &record.TaskID, &record.TaskRevision,
		&record.EstimateID, &record.ProposalTitle, &proposalStart, &proposalEnd,
		&record.ZoneID, &record.Confidence, &explanationCodes,
		&record.Decision, &decidedAt, &record.SupersedesID,
		&record.EventID, &snapshotStart, &snapshotEnd, &record.EventSnapshotHash,
	); err != nil {
		return ProposalDecisionRecord{}, err
	}
	var err error
	if record.ProposalStartAt, err = parseCalendarTime(proposalStart); err != nil {
		return ProposalDecisionRecord{}, err
	}
	if record.ProposalEndAt, err = parseCalendarTime(proposalEnd); err != nil {
		return ProposalDecisionRecord{}, err
	}
	if err := json.Unmarshal(explanationCodes, &record.ExplanationCodes); err != nil {
		return ProposalDecisionRecord{}, err
	}
	if record.DecidedAt, err = parseCalendarTime(decidedAt); err != nil {
		return ProposalDecisionRecord{}, err
	}
	if record.SnapshotStartAt, err = parseCalendarTime(snapshotStart); err != nil {
		return ProposalDecisionRecord{}, err
	}
	if record.SnapshotEndAt, err = parseCalendarTime(snapshotEnd); err != nil {
		return ProposalDecisionRecord{}, err
	}
	return record, nil
}

func taskByIDTx(ctx context.Context, tx *sql.Tx, taskID string) (TaskRecord, error) {
	var payload []byte
	if err := tx.QueryRowContext(ctx, `SELECT payload_json FROM local_tasks WHERE task_id = ?`, taskID).Scan(&payload); errors.Is(err, sql.ErrNoRows) {
		return TaskRecord{}, ErrTaskNotFound
	} else if err != nil {
		return TaskRecord{}, err
	}
	var task TaskRecord
	if err := json.Unmarshal(payload, &task); err != nil {
		return TaskRecord{}, err
	}
	return task, nil
}

func parseCalendarTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse stored calendar time: %w", err)
	}
	return parsed.UTC(), nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
