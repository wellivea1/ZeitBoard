package history

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	storage "non24.app/core/storage/sqlite"
)

const TranscriptionTemplate = "source_record_id,start_local,end_local,zone_id,classification,review_status\r\n"

type TranscriptionTemplateOptions struct {
	FromDate     string
	ThroughDate  string
	ZoneID       string
	SourcePrefix string
}

var canonicalCSVColumns = []string{
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

func EncodeTranscriptionTemplate(options TranscriptionTemplateOptions) ([]byte, int, error) {
	fromText := strings.TrimSpace(options.FromDate)
	throughText := strings.TrimSpace(options.ThroughDate)
	if fromText == "" && throughText == "" {
		return []byte(TranscriptionTemplate), 0, nil
	}
	if fromText == "" || throughText == "" {
		return nil, 0, errors.New("template date coverage requires both from and through dates")
	}
	from, err := time.Parse(time.DateOnly, fromText)
	if err != nil {
		return nil, 0, errors.New("template from date must use YYYY-MM-DD")
	}
	through, err := time.Parse(time.DateOnly, throughText)
	if err != nil {
		return nil, 0, errors.New("template through date must use YYYY-MM-DD")
	}
	if through.Before(from) {
		return nil, 0, errors.New("template through date must not be before from date")
	}
	zoneID := strings.TrimSpace(options.ZoneID)
	if _, err := time.LoadLocation(zoneID); err != nil {
		return nil, 0, fmt.Errorf("load template time zone %q: %w", zoneID, err)
	}
	prefix := strings.TrimSpace(options.SourcePrefix)
	if prefix == "" {
		prefix = "chart"
	}
	if prefix != options.SourcePrefix && options.SourcePrefix != "" {
		return nil, 0, errors.New("template source prefix must not have surrounding whitespace")
	}
	for _, character := range prefix {
		if (character < 'a' || character > 'z') &&
			(character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') &&
			character != '_' && character != '-' {
			return nil, 0, errors.New("template source prefix may contain only ASCII letters, digits, underscores, and hyphens")
		}
	}

	rowCount := int(through.Sub(from).Hours()/24) + 1
	if rowCount > storage.MaxSleepImportRows {
		return nil, 0, fmt.Errorf("template exceeds the %d-row import limit", storage.MaxSleepImportRows)
	}
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	writer.UseCRLF = true
	if err := writer.Write(transcriptionColumns); err != nil {
		return nil, 0, err
	}
	for date := from; !date.After(through); date = date.AddDate(0, 0, 1) {
		sourceID := prefix + "-" + date.Format(time.DateOnly)
		if len([]byte(sourceID)) > 128 {
			return nil, 0, errors.New("generated source_record_id exceeds 128 bytes; shorten the source prefix")
		}
		if err := writer.Write([]string{
			sourceID,
			"",
			"",
			zoneID,
			"",
			TranscriptionReviewNeedsReview,
		}); err != nil {
			return nil, 0, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, 0, err
	}
	return output.Bytes(), rowCount, nil
}

func EncodeObservationSet(format string, generatedAt time.Time, observations []storage.SleepObservationRecord) ([]byte, error) {
	values := append([]storage.SleepObservationRecord(nil), observations...)
	sort.Slice(values, func(i, j int) bool {
		if values[i].StartAt.Equal(values[j].StartAt) {
			return values[i].ObservationID < values[j].ObservationID
		}
		return values[i].StartAt.Before(values[j].StartAt)
	})
	switch strings.ToLower(format) {
	case "json":
		return encodeObservationJSON(generatedAt, values)
	case "csv":
		return encodeObservationCSV(values)
	default:
		return nil, errors.New("output format must be json or csv")
	}
}

func encodeObservationJSON(generatedAt time.Time, observations []storage.SleepObservationRecord) ([]byte, error) {
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}
	if observations == nil {
		observations = []storage.SleepObservationRecord{}
	}
	encoded, err := json.MarshalIndent(storage.SleepObservationSet{
		SchemaVersion: "v1",
		GeneratedAt:   generatedAt.UTC(),
		Observations:  observations,
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func encodeObservationCSV(observations []storage.SleepObservationRecord) ([]byte, error) {
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	writer.UseCRLF = true
	if err := writer.Write(canonicalCSVColumns); err != nil {
		return nil, err
	}
	for _, observation := range observations {
		row := []string{
			observation.ObservationID,
			observation.Kind,
			observation.StartAt.Format(time.RFC3339),
			observation.EndAt.Format(time.RFC3339),
			observation.ZoneID,
			observation.Sleep.Classification,
			observation.Provenance.AcquisitionMethod,
			observation.Provenance.EvidenceStatus,
			observation.Provenance.RecordedAt.Format(time.RFC3339),
			observation.Provenance.SourceRecordID,
		}
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func columnMap(header []string, required []string) (map[string]int, error) {
	allowed := make(map[string]struct{}, len(required))
	for _, name := range required {
		allowed[name] = struct{}{}
	}
	columns := make(map[string]int, len(header))
	var problems []string
	for index, raw := range header {
		name := strings.TrimSpace(strings.TrimPrefix(raw, "\ufeff"))
		if name == "" {
			problems = append(problems, fmt.Sprintf("column %d has an empty name", index+1))
			continue
		}
		if _, exists := columns[name]; exists {
			problems = append(problems, fmt.Sprintf("header repeats %q", name))
			continue
		}
		if _, exists := allowed[name]; !exists {
			problems = append(problems, fmt.Sprintf("unsupported column %q", name))
			continue
		}
		columns[name] = index
	}
	for _, name := range required {
		if _, exists := columns[name]; !exists {
			problems = append(problems, fmt.Sprintf("missing column %q", name))
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return nil, errors.New(strings.Join(problems, "; "))
	}
	return columns, nil
}
