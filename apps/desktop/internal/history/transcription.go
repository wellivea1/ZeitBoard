package history

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	storage "non24.app/core/storage/sqlite"
)

var transcriptionColumns = []string{
	"source_record_id",
	"start_local",
	"end_local",
	"zone_id",
	"classification",
}

type TranscriptionReport struct {
	Rows         int
	Observations int
}

func ConvertTranscriptionFile(path string) ([]storage.SleepObservationRecord, TranscriptionReport, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, TranscriptionReport{}, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if errors.Is(err, io.EOF) {
		return nil, TranscriptionReport{}, errors.New("transcription CSV has no header")
	}
	if err != nil {
		return nil, TranscriptionReport{}, err
	}
	columns, err := columnMap(header, transcriptionColumns)
	if err != nil {
		return nil, TranscriptionReport{}, err
	}
	report := TranscriptionReport{}
	seenSources := map[string]int{}
	var observations []storage.SleepObservationRecord
	rowNumber := 1
	var problems []string
	for {
		record, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		rowNumber++
		report.Rows++
		if readErr != nil {
			problems = append(problems, fmt.Sprintf("row %d: %v", rowNumber, readErr))
			break
		}
		if len(record) != len(header) {
			problems = append(problems, fmt.Sprintf("row %d: expected %d columns; found %d", rowNumber, len(header), len(record)))
			continue
		}
		observation, rowProblems := transcriptionObservation(record, columns)
		for _, problem := range rowProblems {
			problems = append(problems, fmt.Sprintf("row %d: %s", rowNumber, problem))
		}
		if len(rowProblems) > 0 {
			continue
		}
		sourceID := observation.Provenance.SourceRecordID
		if prior, exists := seenSources[sourceID]; exists {
			problems = append(problems, fmt.Sprintf("row %d: source_record_id repeats row %d", rowNumber, prior))
			continue
		}
		seenSources[sourceID] = rowNumber
		observations = append(observations, observation)
	}
	if len(problems) > 0 {
		return nil, report, errors.New(strings.Join(problems, "\n"))
	}
	report.Observations = len(observations)
	return observations, report, nil
}

func transcriptionObservation(record []string, columns map[string]int) (storage.SleepObservationRecord, []string) {
	value := func(name string) string { return strings.TrimSpace(record[columns[name]]) }
	sourceID := value("source_record_id")
	zoneID := value("zone_id")
	classification := value("classification")
	var problems []string
	if sourceID == "" {
		problems = append(problems, "source_record_id is required")
	} else if len([]byte(sourceID)) > 128 {
		problems = append(problems, "source_record_id exceeds 128 bytes")
	}
	location, err := time.LoadLocation(zoneID)
	if err != nil {
		problems = append(problems, fmt.Sprintf("invalid IANA zone_id %q", zoneID))
		location = time.UTC
	}
	start, startErr := parseOwnerTime(value("start_local"), location)
	if startErr != nil {
		problems = append(problems, "start_local: "+startErr.Error())
	}
	end, endErr := parseOwnerTime(value("end_local"), location)
	if endErr != nil {
		problems = append(problems, "end_local: "+endErr.Error())
	}
	if startErr == nil && endErr == nil && !end.After(start) {
		problems = append(problems, "end_local must be after start_local")
	}
	if classification != storage.SleepClassificationPrincipal && classification != storage.SleepClassificationNap {
		problems = append(problems, "classification must be principal or nap")
	}
	if len(problems) > 0 {
		return storage.SleepObservationRecord{}, problems
	}
	hash := sha256.Sum256([]byte("transcription|" + sourceID))
	digest := hex.EncodeToString(hash[:])
	return storage.SleepObservationRecord{
		ObservationID: "obs_transcript_" + digest[:32],
		Kind:          storage.SleepKindEpisode,
		StartAt:       start,
		EndAt:         end,
		ZoneID:        zoneID,
		Sleep:         storage.SleepObservationDetails{Classification: classification},
		Provenance: storage.SleepObservationProvenance{
			AcquisitionMethod: storage.ProvenanceAcquisitionFileImport,
			EvidenceStatus:    storage.ProvenanceEvidenceUserReported,
			RecordedAt:        end,
			SourceRecordID:    sourceID,
		},
	}, nil
}

func parseOwnerTime(value string, location *time.Location) (time.Time, error) {
	value = strings.TrimSpace(value)
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		_, suppliedOffset := parsed.Zone()
		_, locationOffset := parsed.In(location).Zone()
		if suppliedOffset != locationOffset {
			return time.Time{}, errors.New("explicit offset does not match zone_id at that instant")
		}
		return parsed, nil
	}
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02T15:04"} {
		parsed, err := time.ParseInLocation(layout, value, location)
		if err != nil {
			continue
		}
		if parsed.In(location).Format(layout) != value {
			return time.Time{}, errors.New("local time does not exist in that time zone; use an RFC 3339 timestamp with an explicit offset")
		}
		if sameCivilMinute(parsed.Add(-time.Hour).In(location), parsed.In(location)) || sameCivilMinute(parsed.Add(time.Hour).In(location), parsed.In(location)) {
			return time.Time{}, errors.New("local time is ambiguous at a daylight-saving transition; use an RFC 3339 timestamp with an explicit offset")
		}
		return parsed, nil
	}
	return time.Time{}, errors.New("use YYYY-MM-DD HH:MM or RFC 3339 with an explicit offset")
}

func sameCivilMinute(left, right time.Time) bool {
	return left.Year() == right.Year() && left.Month() == right.Month() && left.Day() == right.Day() && left.Hour() == right.Hour() && left.Minute() == right.Minute()
}
