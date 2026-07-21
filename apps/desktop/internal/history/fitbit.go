package history

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	storage "non24.app/core/storage/sqlite"
)

const fitbitTimeLayout = "2006-01-02 3:04PM"

var fitbitColumns = []string{
	"Start Time",
	"End Time",
	"Minutes Asleep",
	"Minutes Awake",
	"Number of Awakenings",
	"Time in Bed",
	"Minutes REM Sleep",
	"Minutes Light Sleep",
	"Minutes Deep Sleep",
}

type FitbitOptions struct {
	Directory         string
	ZoneID            string
	FromDate          string
	ThroughDate       string
	IncludeSuperseded bool
}

type FitbitReport struct {
	FilesRead        int
	FilesIgnored     int
	RowsRead         int
	RowsOutsideRange int
	DuplicateRows    int
	Observations     int
	NapRows          int
	EarliestStart    time.Time
	LatestStart      time.Time
}

func ConvertFitbitDirectory(options FitbitOptions) ([]storage.SleepObservationRecord, FitbitReport, error) {
	location, err := time.LoadLocation(strings.TrimSpace(options.ZoneID))
	if err != nil {
		return nil, FitbitReport{}, fmt.Errorf("load time zone %q: %w", options.ZoneID, err)
	}
	from, through, err := parseDateBounds(options.FromDate, options.ThroughDate, location)
	if err != nil {
		return nil, FitbitReport{}, err
	}
	files, ignored, err := findFitbitFiles(options.Directory, options.IncludeSuperseded)
	if err != nil {
		return nil, FitbitReport{}, err
	}
	if len(files) == 0 {
		return nil, FitbitReport{}, errors.New("no files named Fitbit Sleep Data*.csv were found")
	}

	report := FitbitReport{FilesIgnored: ignored}
	bySource := map[string]storage.SleepObservationRecord{}
	for _, path := range files {
		rows, err := readFitbitFile(path, location)
		if err != nil {
			return nil, FitbitReport{}, err
		}
		report.FilesRead++
		report.RowsRead += len(rows)
		for _, observation := range rows {
			if observation.StartAt.Before(from) || !observation.StartAt.Before(through) {
				report.RowsOutsideRange++
				continue
			}
			sourceID := observation.Provenance.SourceRecordID
			if existing, exists := bySource[sourceID]; exists {
				if !sameConvertedObservation(existing, observation) {
					return nil, FitbitReport{}, fmt.Errorf("source hash collision for %s", sourceID)
				}
				report.DuplicateRows++
				continue
			}
			bySource[sourceID] = observation
			if observation.Sleep.Classification == storage.SleepClassificationNap {
				report.NapRows++
			}
			if report.EarliestStart.IsZero() || observation.StartAt.Before(report.EarliestStart) {
				report.EarliestStart = observation.StartAt
			}
			if report.LatestStart.IsZero() || observation.StartAt.After(report.LatestStart) {
				report.LatestStart = observation.StartAt
			}
		}
	}
	observations := make([]storage.SleepObservationRecord, 0, len(bySource))
	for _, observation := range bySource {
		observations = append(observations, observation)
	}
	sort.Slice(observations, func(i, j int) bool { return observations[i].StartAt.Before(observations[j].StartAt) })
	report.Observations = len(observations)
	return observations, report, nil
}

func findFitbitFiles(directory string, includeSuperseded bool) ([]string, int, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return nil, 0, errors.New("Fitbit input directory is required")
	}
	var files []string
	ignored := 0
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		name := strings.ToLower(entry.Name())
		if strings.HasPrefix(name, "fitbit sleep data") && filepath.Ext(name) == ".csv" {
			if !includeSuperseded && hasSupersededDirectory(directory, path) {
				ignored++
				return nil
			}
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	sort.Strings(files)
	return files, ignored, nil
}

func hasSupersededDirectory(root, path string) bool {
	relative, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil {
		return false
	}
	for _, part := range strings.FieldsFunc(relative, func(r rune) bool { return r == '/' || r == '\\' }) {
		switch strings.ToLower(part) {
		case "old", "incomplete", "weekly":
			return true
		}
	}
	return false
}

func readFitbitFile(path string, location *time.Location) ([]storage.SleepObservationRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	section, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("%s: read section marker: %w", path, err)
	}
	if len(section) != 1 || strings.TrimSpace(strings.TrimPrefix(section[0], "\ufeff")) != "Sleep" {
		return nil, fmt.Errorf("%s: expected Fitbit Sleep section marker", path)
	}
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("%s: read header: %w", path, err)
	}
	columns, err := columnMap(header, fitbitColumns)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var observations []storage.SleepObservationRecord
	rowNumber := 2
	for {
		record, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		rowNumber++
		if readErr != nil {
			return nil, fmt.Errorf("%s row %d: %w", path, rowNumber, readErr)
		}
		if len(record) != len(header) {
			return nil, fmt.Errorf("%s row %d: expected %d columns; found %d", path, rowNumber, len(header), len(record))
		}
		start, err := parseFitbitTime(record[columns["Start Time"]], location)
		if err != nil {
			return nil, fmt.Errorf("%s row %d start time: %w", path, rowNumber, err)
		}
		end, err := parseFitbitTime(record[columns["End Time"]], location)
		if err != nil {
			return nil, fmt.Errorf("%s row %d end time: %w", path, rowNumber, err)
		}
		if !end.After(start) {
			return nil, fmt.Errorf("%s row %d: end time must be after start time", path, rowNumber)
		}
		observations = append(observations, fitbitObservation(start, end, location.String()))
	}
	return observations, nil
}

func parseFitbitTime(value string, location *time.Location) (time.Time, error) {
	value = strings.TrimSpace(value)
	parsed, err := time.ParseInLocation(fitbitTimeLayout, value, location)
	if err != nil {
		return time.Time{}, err
	}
	if parsed.In(location).Format(fitbitTimeLayout) != value {
		return time.Time{}, errors.New("local time does not exist in that time zone")
	}
	local := parsed.In(location)
	if sameCivilMinute(parsed.Add(-time.Hour).In(location), local) || sameCivilMinute(parsed.Add(time.Hour).In(location), local) {
		return time.Time{}, errors.New("local time is ambiguous at a daylight-saving transition; transcribe this row with an explicit RFC 3339 offset")
	}
	return parsed, nil
}

func fitbitObservation(start, end time.Time, zoneID string) storage.SleepObservationRecord {
	stable := strings.Join([]string{"fitbit", start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339), zoneID}, "|")
	hash := sha256.Sum256([]byte(stable))
	digest := hex.EncodeToString(hash[:])
	classification := storage.SleepClassificationPrincipal
	if end.Sub(start) < 3*time.Hour {
		classification = storage.SleepClassificationNap
	}
	return storage.SleepObservationRecord{
		ObservationID: "obs_fitbit_" + digest[:32],
		Kind:          storage.SleepKindEpisode,
		StartAt:       start,
		EndAt:         end,
		ZoneID:        zoneID,
		Sleep:         storage.SleepObservationDetails{Classification: classification},
		Provenance: storage.SleepObservationProvenance{
			AcquisitionMethod: storage.ProvenanceAcquisitionFileImport,
			EvidenceStatus:    storage.ProvenanceEvidenceDirectlyObserved,
			RecordedAt:        end,
			SourceRecordID:    "fitbit-" + digest,
		},
	}
}

func parseDateBounds(fromDate, throughDate string, location *time.Location) (time.Time, time.Time, error) {
	from, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(fromDate), location)
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("from date must use YYYY-MM-DD")
	}
	throughDay, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(throughDate), location)
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("through date must use YYYY-MM-DD")
	}
	through := throughDay.AddDate(0, 0, 1)
	if !through.After(from) {
		return time.Time{}, time.Time{}, errors.New("through date must not be before from date")
	}
	return from, through, nil
}

func sameConvertedObservation(left, right storage.SleepObservationRecord) bool {
	return left.StartAt.Equal(right.StartAt) && left.EndAt.Equal(right.EndAt) && left.ZoneID == right.ZoneID && left.Sleep.Classification == right.Sleep.Classification
}
