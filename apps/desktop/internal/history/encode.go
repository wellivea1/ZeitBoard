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

const TranscriptionTemplate = "source_record_id,start_local,end_local,zone_id,classification\r\n"

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
