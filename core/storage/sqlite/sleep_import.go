package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	MaxSleepImportBytes = 8 * 1024 * 1024
	MaxSleepImportRows  = 20_000

	SleepImportStatusReady     = "ready"
	SleepImportStatusDuplicate = "duplicate"
	SleepImportStatusInvalid   = "invalid"
	SleepImportStatusImported  = "imported"
)

var sleepImportCSVColumns = []string{
	"observation_id",
	"kind",
	"start_at",
	"end_at",
	"zone_id",
	"sleep_classification",
	"acquisition_method",
	"evidence_status",
	"recorded_at",
	"source_record_id",
}

type SleepImportInput struct {
	FileName string
	Contents string
}

type SleepImportRow struct {
	RowNumber    int
	Observation  SleepObservationRecord
	Status       string
	Errors       []string
	StatusDetail string
}

type SleepImportReport struct {
	FileName      string
	Format        string
	DryRun        bool
	TotalRows     int
	ReadyRows     int
	DuplicateRows int
	InvalidRows   int
	ImportedRows  int
	CanImport     bool
	Errors        []string
	Rows          []SleepImportRow
	Message       string
}

type sleepImportDocument struct {
	fileName string
	format   string
	errors   []string
	rows     []SleepImportRow
}

type sleepImportJSONEnvelope struct {
	SchemaVersion *string            `json:"schema_version"`
	GeneratedAt   *time.Time         `json:"generated_at"`
	Observations  *[]json.RawMessage `json:"observations"`
}

type sleepImportJSONObservation struct {
	ObservationID string                     `json:"observation_id"`
	Kind          string                     `json:"kind"`
	StartAt       time.Time                  `json:"start_at"`
	EndAt         time.Time                  `json:"end_at"`
	ZoneID        string                     `json:"zone_id"`
	Sleep         *SleepObservationDetails   `json:"sleep"`
	Provenance    SleepObservationProvenance `json:"provenance"`
}

type sleepImportQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (s *Store) PreviewSleepImport(ctx context.Context, input SleepImportInput) (SleepImportReport, error) {
	document := parseSleepImport(input)
	return s.classifySleepImport(ctx, s.db, document, true)
}

func (s *Store) ImportSleepObservations(ctx context.Context, input SleepImportInput) (SleepImportReport, error) {
	document := parseSleepImport(input)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SleepImportReport{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	report, err := s.classifySleepImport(ctx, tx, document, false)
	if err != nil {
		return SleepImportReport{}, err
	}
	if !report.CanImport {
		return report, nil
	}
	for index := range report.Rows {
		row := &report.Rows[index]
		if row.Status != SleepImportStatusReady {
			continue
		}
		if err := appendImportedSleepObservation(ctx, tx, row.Observation); err != nil {
			return SleepImportReport{}, fmt.Errorf("import row %d: %w", row.RowNumber, err)
		}
		row.Status = SleepImportStatusImported
		row.StatusDetail = "Imported as " + row.Observation.ObservationID
		report.ImportedRows++
	}
	if err := tx.Commit(); err != nil {
		return SleepImportReport{}, err
	}
	committed = true
	report.ReadyRows = 0
	report.CanImport = false
	report.Message = fmt.Sprintf(
		"Imported %d sleep %s; %d exact %s already present.",
		report.ImportedRows,
		pluralImport(report.ImportedRows, "observation", "observations"),
		report.DuplicateRows,
		pluralImport(report.DuplicateRows, "duplicate was", "duplicates were"),
	)
	return report, nil
}

func parseSleepImport(input SleepImportInput) sleepImportDocument {
	document := sleepImportDocument{fileName: strings.TrimSpace(input.FileName)}
	if document.fileName == "" {
		document.fileName = "sleep-import"
	}
	if len([]byte(input.Contents)) > MaxSleepImportBytes {
		document.errors = append(document.errors, fmt.Sprintf("file exceeds the %d MiB import limit", MaxSleepImportBytes/(1024*1024)))
		return document
	}
	if strings.TrimSpace(input.Contents) == "" {
		document.errors = append(document.errors, "file is empty")
		return document
	}

	switch strings.ToLower(filepath.Ext(document.fileName)) {
	case ".json":
		document.format = "json"
		parseSleepImportJSON(input.Contents, &document)
	case ".csv":
		document.format = "csv"
		parseSleepImportCSV(input.Contents, &document)
	default:
		document.errors = append(document.errors, "file name must end in .json or .csv")
	}
	return document
}

func parseSleepImportJSON(contents string, document *sleepImportDocument) {
	var envelope sleepImportJSONEnvelope
	if err := decodeSingleJSONString(contents, &envelope); err != nil {
		document.errors = append(document.errors, "invalid JSON: "+err.Error())
		return
	}
	if envelope.SchemaVersion == nil {
		document.errors = append(document.errors, "missing top-level field \"schema_version\"")
	}
	if envelope.GeneratedAt == nil {
		document.errors = append(document.errors, "missing top-level field \"generated_at\"")
	}
	if envelope.Observations == nil {
		document.errors = append(document.errors, "missing top-level field \"observations\"")
	}
	if len(document.errors) > 0 {
		return
	}
	if *envelope.SchemaVersion != "v1" {
		document.errors = append(document.errors, "schema_version must be v1")
	}
	if envelope.GeneratedAt.IsZero() {
		document.errors = append(document.errors, "generated_at must be an RFC 3339 timestamp")
	}
	observations := *envelope.Observations
	if len(observations) > MaxSleepImportRows {
		document.errors = append(document.errors, fmt.Sprintf("observation set exceeds the %d-row import limit", MaxSleepImportRows))
		return
	}

	for index, raw := range observations {
		row := SleepImportRow{RowNumber: index + 1, Status: SleepImportStatusReady, Errors: []string{}}
		var value sleepImportJSONObservation
		if err := decodeSingleJSON(raw, &value); err != nil {
			row.Errors = append(row.Errors, "invalid observation object: "+err.Error())
		} else {
			row.Observation = SleepObservationRecord{
				ObservationID: value.ObservationID,
				Kind:          value.Kind,
				StartAt:       value.StartAt,
				EndAt:         value.EndAt,
				ZoneID:        value.ZoneID,
				Provenance:    value.Provenance,
			}
			if value.Sleep == nil {
				row.Errors = append(row.Errors, "sleep is required; activity_interval rows are not accepted by the local sleep importer")
			} else {
				row.Observation.Sleep = *value.Sleep
			}
			row.Errors = append(row.Errors, validateImportedSleepObservation(row.Observation)...)
		}
		if len(row.Errors) > 0 {
			row.Status = SleepImportStatusInvalid
		}
		document.rows = append(document.rows, row)
	}
}

func parseSleepImportCSV(contents string, document *sleepImportDocument) {
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(contents, "\ufeff")))
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if errors.Is(err, io.EOF) {
		document.errors = append(document.errors, "CSV has no header row")
		return
	}
	if err != nil {
		document.errors = append(document.errors, "invalid CSV header: "+err.Error())
		return
	}
	columns, headerErrors := sleepImportColumnMap(header)
	if len(headerErrors) > 0 {
		document.errors = append(document.errors, headerErrors...)
		return
	}

	rowNumber := 1
	for {
		record, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		rowNumber++
		row := SleepImportRow{RowNumber: rowNumber, Status: SleepImportStatusReady, Errors: []string{}}
		if readErr != nil {
			row.Status = SleepImportStatusInvalid
			row.Errors = append(row.Errors, "invalid CSV row: "+readErr.Error())
			document.rows = append(document.rows, row)
			break
		}
		if len(record) != len(header) {
			row.Errors = append(row.Errors, fmt.Sprintf("expected %d columns; found %d", len(header), len(record)))
		} else {
			row.Observation, row.Errors = sleepObservationFromCSV(record, columns)
		}
		if len(row.Errors) > 0 {
			row.Status = SleepImportStatusInvalid
		}
		if len(document.rows) >= MaxSleepImportRows {
			document.errors = append(document.errors, fmt.Sprintf("CSV exceeds the %d-row import limit", MaxSleepImportRows))
			document.rows = nil
			return
		}
		document.rows = append(document.rows, row)
	}
}

func sleepImportColumnMap(header []string) (map[string]int, []string) {
	columns := make(map[string]int, len(header))
	var problems []string
	allowed := make(map[string]struct{}, len(sleepImportCSVColumns))
	for _, name := range sleepImportCSVColumns {
		allowed[name] = struct{}{}
	}
	for index, raw := range header {
		name := strings.TrimSpace(raw)
		if name == "" {
			problems = append(problems, fmt.Sprintf("CSV column %d has an empty name", index+1))
			continue
		}
		if name != raw {
			problems = append(problems, fmt.Sprintf("CSV column %d must not have surrounding whitespace", index+1))
			continue
		}
		if _, exists := columns[name]; exists {
			problems = append(problems, fmt.Sprintf("CSV header repeats %q", name))
			continue
		}
		if _, exists := allowed[name]; !exists {
			problems = append(problems, fmt.Sprintf("CSV header contains unsupported column %q", name))
			continue
		}
		columns[name] = index
	}
	for _, required := range sleepImportCSVColumns {
		if _, exists := columns[required]; !exists {
			problems = append(problems, fmt.Sprintf("CSV header is missing %q", required))
		}
	}
	return columns, problems
}

func sleepObservationFromCSV(record []string, columns map[string]int) (SleepObservationRecord, []string) {
	value := func(name string) string { return record[columns[name]] }
	start, startErr := time.Parse(time.RFC3339, value("start_at"))
	end, endErr := time.Parse(time.RFC3339, value("end_at"))
	recorded, recordedErr := time.Parse(time.RFC3339, value("recorded_at"))
	var problems []string
	if startErr != nil {
		problems = append(problems, "start_at must be an RFC 3339 timestamp with an offset")
	}
	if endErr != nil {
		problems = append(problems, "end_at must be an RFC 3339 timestamp with an offset")
	}
	if recordedErr != nil {
		problems = append(problems, "recorded_at must be an RFC 3339 timestamp with an offset")
	}
	recordValue := SleepObservationRecord{
		ObservationID: value("observation_id"),
		Kind:          value("kind"),
		StartAt:       start,
		EndAt:         end,
		ZoneID:        value("zone_id"),
		Sleep: SleepObservationDetails{
			Classification: value("sleep_classification"),
		},
		Provenance: SleepObservationProvenance{
			AcquisitionMethod: value("acquisition_method"),
			EvidenceStatus:    value("evidence_status"),
			RecordedAt:        recorded,
			SourceRecordID:    value("source_record_id"),
		},
	}
	problems = append(problems, validateImportedSleepObservation(recordValue)...)
	return recordValue, uniqueStrings(problems)
}

func validateImportedSleepObservation(record SleepObservationRecord) []string {
	var problems []string
	if err := validateSleepObservation(record); err != nil {
		problems = append(problems, err.Error())
	}
	if record.Provenance.AcquisitionMethod != ProvenanceAcquisitionFileImport {
		problems = append(problems, "provenance.acquisition_method must be file_import")
	}
	sourceRecordID := record.Provenance.SourceRecordID
	if strings.TrimSpace(sourceRecordID) == "" {
		problems = append(problems, "provenance.source_record_id is required for deduplication")
	} else if sourceRecordID != strings.TrimSpace(sourceRecordID) {
		problems = append(problems, "provenance.source_record_id must not have surrounding whitespace")
	} else if len([]byte(sourceRecordID)) > 128 {
		problems = append(problems, "provenance.source_record_id exceeds 128 bytes")
	}
	if record.Sleep.Classification == SleepClassificationUnknown {
		problems = append(problems, "sleep.classification must be principal or nap for estimator input; unknown is not silently treated as principal")
	}
	return uniqueStrings(problems)
}

func (s *Store) classifySleepImport(ctx context.Context, queryer sleepImportQueryer, document sleepImportDocument, dryRun bool) (SleepImportReport, error) {
	report := SleepImportReport{
		FileName: document.fileName,
		Format:   document.format,
		DryRun:   dryRun,
		Errors:   append([]string(nil), document.errors...),
		Rows:     cloneSleepImportRows(document.rows),
	}
	if len(report.Errors) > 0 {
		for index := range report.Rows {
			row := &report.Rows[index]
			if row.Status != SleepImportStatusInvalid {
				row.Status = SleepImportStatusInvalid
				row.StatusDetail = "Blocked by a file-level error"
			}
		}
		report.TotalRows = len(report.Rows)
		report.InvalidRows = countSleepImportRows(report.Rows, SleepImportStatusInvalid)
		report.Message = "The import file is invalid; no data will be written."
		return report, nil
	}

	existingBySource, existingByObservation, err := loadExistingSleepImports(ctx, queryer, report.Rows)
	if err != nil {
		return SleepImportReport{}, err
	}
	batchBySource := map[string]int{}
	batchByObservation := map[string]int{}
	for index := range report.Rows {
		row := &report.Rows[index]
		if row.Status == SleepImportStatusInvalid {
			continue
		}
		observation := row.Observation
		sourceID := observation.Provenance.SourceRecordID
		if previousIndex, exists := batchBySource[sourceID]; exists {
			previous := &report.Rows[previousIndex]
			if sameImportedSleepPayload(previous.Observation, observation) {
				row.Status = SleepImportStatusDuplicate
				row.StatusDetail = fmt.Sprintf("Exact duplicate of row %d", previous.RowNumber)
			} else {
				markSleepImportConflict(previous, fmt.Sprintf("source_record_id also appears with different data on row %d", row.RowNumber))
				markSleepImportConflict(row, fmt.Sprintf("source_record_id conflicts with row %d", previous.RowNumber))
			}
			continue
		}
		if previousIndex, exists := batchByObservation[observation.ObservationID]; exists {
			previous := &report.Rows[previousIndex]
			markSleepImportConflict(previous, fmt.Sprintf("observation_id also appears on row %d", row.RowNumber))
			markSleepImportConflict(row, fmt.Sprintf("observation_id conflicts with row %d", previous.RowNumber))
			continue
		}
		if existing, exists := existingBySource[sourceID]; exists {
			if sameImportedSleepPayload(existing, observation) {
				row.Status = SleepImportStatusDuplicate
				row.StatusDetail = "Already imported as " + existing.ObservationID
			} else {
				markSleepImportConflict(row, "source_record_id already belongs to different imported data")
			}
			continue
		}
		if existing, exists := existingByObservation[observation.ObservationID]; exists {
			markSleepImportConflict(row, fmt.Sprintf("observation_id already belongs to source record %q", existing.Provenance.SourceRecordID))
			continue
		}
		batchBySource[sourceID] = index
		batchByObservation[observation.ObservationID] = index
		row.StatusDetail = "Ready to append"
	}

	report.TotalRows = len(report.Rows)
	report.ReadyRows = countSleepImportRows(report.Rows, SleepImportStatusReady)
	report.DuplicateRows = countSleepImportRows(report.Rows, SleepImportStatusDuplicate)
	report.InvalidRows = countSleepImportRows(report.Rows, SleepImportStatusInvalid)
	report.CanImport = report.ReadyRows > 0 && report.InvalidRows == 0
	switch {
	case report.InvalidRows > 0:
		report.Message = fmt.Sprintf("Fix %d invalid %s; no data will be written.", report.InvalidRows, pluralImport(report.InvalidRows, "row", "rows"))
	case report.ReadyRows == 0 && report.DuplicateRows > 0:
		report.Message = fmt.Sprintf("All %d %s already imported; no changes are needed.", report.DuplicateRows, pluralImport(report.DuplicateRows, "row is", "rows are"))
	case report.ReadyRows == 0:
		report.Message = "The file contains no sleep observations."
	default:
		report.Message = fmt.Sprintf(
			"%d %s ready to append; %d exact %s already present.",
			report.ReadyRows,
			pluralImport(report.ReadyRows, "row is", "rows are"),
			report.DuplicateRows,
			pluralImport(report.DuplicateRows, "duplicate is", "duplicates are"),
		)
	}
	return report, nil
}

const sleepImportConflictQueryBatchSize = 400

func loadExistingSleepImports(
	ctx context.Context,
	queryer sleepImportQueryer,
	candidates []SleepImportRow,
) (map[string]SleepObservationRecord, map[string]SleepObservationRecord, error) {
	observationIDs := map[string]struct{}{}
	sourceIDs := map[string]struct{}{}
	for _, candidate := range candidates {
		if candidate.Status == SleepImportStatusInvalid {
			continue
		}
		observationIDs[candidate.Observation.ObservationID] = struct{}{}
		sourceIDs[candidate.Observation.Provenance.SourceRecordID] = struct{}{}
	}

	bySource := map[string]SleepObservationRecord{}
	byObservation := map[string]SleepObservationRecord{}
	seenObservation := map[string]struct{}{}
	visit := func(record SleepObservationRecord) error {
		if _, seen := seenObservation[record.ObservationID]; seen {
			return nil
		}
		seenObservation[record.ObservationID] = struct{}{}
		byObservation[record.ObservationID] = record
		if record.Provenance.AcquisitionMethod != ProvenanceAcquisitionFileImport || record.Provenance.SourceRecordID == "" {
			return nil
		}
		if existing, exists := bySource[record.Provenance.SourceRecordID]; exists && existing.ObservationID != record.ObservationID {
			return errors.New("local store contains repeated imported source_record_id values; suppress or erase the duplicate before importing")
		}
		bySource[record.Provenance.SourceRecordID] = record
		return nil
	}

	if err := queryExistingSleepImportRows(ctx, queryer, "observation_id", sortedSleepImportKeys(observationIDs), visit); err != nil {
		return nil, nil, err
	}
	if err := queryExistingSleepImportRows(ctx, queryer, "source_record_id", sortedSleepImportKeys(sourceIDs), visit); err != nil {
		return nil, nil, err
	}
	return bySource, byObservation, nil
}

func sortedSleepImportKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func queryExistingSleepImportRows(
	ctx context.Context,
	queryer sleepImportQueryer,
	field string,
	values []string,
	visit func(SleepObservationRecord) error,
) error {
	for offset := 0; offset < len(values); offset += sleepImportConflictQueryBatchSize {
		end := offset + sleepImportConflictQueryBatchSize
		if end > len(values) {
			end = len(values)
		}
		batch := values[offset:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		args := make([]any, 0, len(batch)+1)
		var where string
		switch field {
		case "observation_id":
			where = "observation_id IN (" + placeholders + ")"
		case "source_record_id":
			where = "acquisition_method = ? AND source_record_id IN (" + placeholders + ")"
			args = append(args, ProvenanceAcquisitionFileImport)
		default:
			return fmt.Errorf("unsupported sleep import conflict field %q", field)
		}
		for _, value := range batch {
			args = append(args, value)
		}
		rows, err := queryer.QueryContext(ctx, `SELECT
			observation_id, kind, start_at, end_at, zone_id, classification,
			acquisition_method, evidence_status, recorded_at, source_record_id
			FROM local_sleep_observations
			WHERE `+where+`
			ORDER BY observation_id`, args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			record, scanErr := scanSleepImportConflictRow(rows)
			if scanErr != nil {
				_ = rows.Close()
				return scanErr
			}
			if visitErr := visit(record); visitErr != nil {
				_ = rows.Close()
				return visitErr
			}
		}
		rowErr := rows.Err()
		closeErr := rows.Close()
		if rowErr != nil {
			return rowErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func scanSleepImportConflictRow(rows *sql.Rows) (SleepObservationRecord, error) {
	var record SleepObservationRecord
	var startAt, endAt, recordedAt string
	if err := rows.Scan(
		&record.ObservationID, &record.Kind, &startAt, &endAt, &record.ZoneID,
		&record.Sleep.Classification, &record.Provenance.AcquisitionMethod,
		&record.Provenance.EvidenceStatus, &recordedAt, &record.Provenance.SourceRecordID,
	); err != nil {
		return SleepObservationRecord{}, err
	}
	var err error
	if record.StartAt, err = time.Parse(time.RFC3339Nano, startAt); err != nil {
		return SleepObservationRecord{}, fmt.Errorf("stored sleep start_at: %w", err)
	}
	if record.EndAt, err = time.Parse(time.RFC3339Nano, endAt); err != nil {
		return SleepObservationRecord{}, fmt.Errorf("stored sleep end_at: %w", err)
	}
	if record.Provenance.RecordedAt, err = time.Parse(time.RFC3339Nano, recordedAt); err != nil {
		return SleepObservationRecord{}, fmt.Errorf("stored sleep recorded_at: %w", err)
	}
	return record, nil
}

func appendImportedSleepObservation(ctx context.Context, tx *sql.Tx, record SleepObservationRecord) error {
	if len(validateImportedSleepObservation(record)) > 0 {
		return errors.New("validated import row became invalid")
	}
	record.StartAt = record.StartAt.UTC()
	record.EndAt = record.EndAt.UTC()
	record.Provenance.RecordedAt = record.Provenance.RecordedAt.UTC()
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO local_sleep_observations(
		observation_id, kind, start_at, end_at, zone_id, classification,
		acquisition_method, evidence_status, recorded_at, source_record_id, payload_json
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ObservationID, record.Kind, formatSQLiteTime(record.StartAt), formatSQLiteTime(record.EndAt),
		record.ZoneID, record.Sleep.Classification, record.Provenance.AcquisitionMethod,
		record.Provenance.EvidenceStatus, formatSQLiteTime(record.Provenance.RecordedAt),
		record.Provenance.SourceRecordID, encoded,
	)
	return err
}

func decodeSingleJSONString(data string, target any) error {
	return decodeSingleJSONReader(strings.NewReader(data), target)
}

func decodeSingleJSON(data []byte, target any) error {
	return decodeSingleJSONReader(bytes.NewReader(data), target)
}

func decodeSingleJSONReader(reader io.Reader, target any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func sameImportedSleepPayload(left, right SleepObservationRecord) bool {
	return left.Kind == right.Kind &&
		left.StartAt.UTC().Equal(right.StartAt.UTC()) &&
		left.EndAt.UTC().Equal(right.EndAt.UTC()) &&
		left.ZoneID == right.ZoneID &&
		left.Sleep.Classification == right.Sleep.Classification &&
		left.Provenance.AcquisitionMethod == right.Provenance.AcquisitionMethod &&
		left.Provenance.EvidenceStatus == right.Provenance.EvidenceStatus &&
		left.Provenance.RecordedAt.UTC().Equal(right.Provenance.RecordedAt.UTC())
}

func markSleepImportConflict(row *SleepImportRow, problem string) {
	row.Status = SleepImportStatusInvalid
	row.StatusDetail = "Conflicting identifier"
	row.Errors = uniqueStrings(append(row.Errors, problem))
}

func cloneSleepImportRows(rows []SleepImportRow) []SleepImportRow {
	result := make([]SleepImportRow, len(rows))
	for index, row := range rows {
		result[index] = row
		result[index].Errors = append([]string(nil), row.Errors...)
	}
	return result
}

func countSleepImportRows(rows []SleepImportRow, status string) int {
	count := 0
	for _, row := range rows {
		if row.Status == status {
			count++
		}
	}
	return count
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func pluralImport(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}
