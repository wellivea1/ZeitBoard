package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"non24.app/core/domain"
	storage "non24.app/core/storage/sqlite"
)

type RhythmMarkerInput struct {
	Kind       string `json:"kind"`
	StartLocal string `json:"startLocal"`
	EndLocal   string `json:"endLocal"`
	ZoneID     string `json:"zoneId"`
	Note       string `json:"note"`
}

type RhythmMarkerDeleteInput struct {
	MarkerID     string `json:"markerId"`
	Confirmation string `json:"confirmation"`
}

type RhythmMarkerDTO struct {
	MarkerID      string  `json:"markerId"`
	Kind          string  `json:"kind"`
	KindLabel     string  `json:"kindLabel"`
	StartAt       string  `json:"startAt"`
	EndAt         string  `json:"endAt,omitempty"`
	ZoneID        string  `json:"zoneId"`
	CivilDate     string  `json:"civilDate"`
	Hour          float64 `json:"hour"`
	StartLabel    string  `json:"startLabel"`
	EndLabel      string  `json:"endLabel,omitempty"`
	RangeLabel    string  `json:"rangeLabel"`
	Note          string  `json:"note,omitempty"`
	RecordedLabel string  `json:"recordedLabel"`
}

type RhythmMarkersDTO struct {
	Status       string            `json:"status"`
	Empty        bool              `json:"empty"`
	Message      string            `json:"message"`
	Markers      []RhythmMarkerDTO `json:"markers"`
	FixtureMode  bool              `json:"fixtureMode"`
	UpdatedLabel string            `json:"updatedLabel"`
}

type RhythmMarkerExportDTO struct {
	FileName       string `json:"fileName"`
	JSON           string `json:"json"`
	GeneratedAt    string `json:"generatedAt"`
	GeneratedLabel string `json:"generatedLabel"`
	MarkerCount    int    `json:"markerCount"`
}

func (a *App) GetRhythmMarkers() (RhythmMarkersDTO, error) {
	return a.rhythmMarkersAt(a.currentTime().UTC().Truncate(time.Second))
}

func (a *App) AddRhythmMarker(input RhythmMarkerInput) (RhythmMarkersDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return RhythmMarkersDTO{}, err
	}
	now := a.currentTime().UTC().Truncate(time.Second)
	kind, startAt, endAt, zoneID, note, err := rhythmMarkerValues(input, now)
	if err != nil {
		return RhythmMarkersDTO{}, err
	}
	record := storage.RhythmMarkerRecord{
		MarkerID: newLocalID("marker"),
		Kind:     kind,
		StartAt:  startAt,
		EndAt:    endAt,
		ZoneID:   zoneID,
		Note:     note,
		Provenance: storage.SleepObservationProvenance{
			AcquisitionMethod: storage.ProvenanceAcquisitionManual,
			EvidenceStatus:    storage.ProvenanceEvidenceUserReported,
			RecordedAt:        now,
		},
	}
	if err := store.CreateRhythmMarker(context.Background(), record); err != nil {
		return RhythmMarkersDTO{}, err
	}
	return a.rhythmMarkersAt(now)
}

func (a *App) DeleteRhythmMarker(input RhythmMarkerDeleteInput) (RhythmMarkersDTO, error) {
	if err := requireDeleteConfirmation(input.Confirmation); err != nil {
		return RhythmMarkersDTO{}, err
	}
	store, err := a.requireStore()
	if err != nil {
		return RhythmMarkersDTO{}, err
	}
	if err := store.DeleteRhythmMarker(context.Background(), strings.TrimSpace(input.MarkerID)); err != nil {
		return RhythmMarkersDTO{}, err
	}
	return a.GetRhythmMarkers()
}

func (a *App) ExportRhythmMarkers() (RhythmMarkerExportDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return RhythmMarkerExportDTO{}, err
	}
	now := a.currentTime().UTC().Truncate(time.Second)
	exported, err := store.ExportRhythmMarkers(context.Background(), now)
	if err != nil {
		return RhythmMarkerExportDTO{}, err
	}
	encoded, err := json.MarshalIndent(exported, "", "  ")
	if err != nil {
		return RhythmMarkerExportDTO{}, err
	}
	encoded = append(encoded, '\n')
	return RhythmMarkerExportDTO{
		FileName:       "zeitboard-rhythm-context-v1-" + now.Format("20060102") + ".json",
		JSON:           string(encoded),
		GeneratedAt:    now.Format(time.RFC3339),
		GeneratedLabel: now.Local().Format("Jan 2, 2006, 3:04 PM"),
		MarkerCount:    len(exported.Markers),
	}, nil
}

func (a *App) rhythmMarkersAt(now time.Time) (RhythmMarkersDTO, error) {
	return a.rhythmMarkersAtContext(a.applicationContext(), now)
}

func (a *App) rhythmMarkersAtContext(ctx context.Context, now time.Time) (RhythmMarkersDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return RhythmMarkersDTO{}, err
	}
	if ctx == nil {
		ctx = a.applicationContext()
	}
	records, err := store.ListRhythmMarkers(ctx)
	if err != nil {
		return RhythmMarkersDTO{}, err
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].StartAt.Equal(records[j].StartAt) {
			return records[i].MarkerID > records[j].MarkerID
		}
		return records[i].StartAt.After(records[j].StartAt)
	})
	markers := make([]RhythmMarkerDTO, 0, len(records))
	for _, record := range records {
		marker, err := rhythmMarkerDTO(record)
		if err != nil {
			return RhythmMarkersDTO{}, err
		}
		markers = append(markers, marker)
	}
	message := "No context markers yet. Markers are optional self-reports and do not change the rhythm estimate."
	if len(markers) > 0 {
		message = fmt.Sprintf("%d self-reported context %s. They do not change the rhythm estimate or establish cause.", len(markers), plural(len(markers), "marker", "markers"))
	}
	status := "ready"
	if len(markers) == 0 {
		status = "empty"
	}
	return RhythmMarkersDTO{
		Status:       status,
		Empty:        len(markers) == 0,
		Message:      message,
		Markers:      markers,
		FixtureMode:  false,
		UpdatedLabel: now.Local().Format("Updated Jan 2, 3:04 PM"),
	}, nil
}

func rhythmMarkerDTO(record storage.RhythmMarkerRecord) (RhythmMarkerDTO, error) {
	location, err := time.LoadLocation(record.ZoneID)
	if err != nil {
		return RhythmMarkerDTO{}, fmt.Errorf("load marker time zone %q: %w", record.ZoneID, err)
	}
	start := record.StartAt.In(location)
	startLabel := start.Format("Jan 2, 2006, 3:04 PM")
	endAt := ""
	endLabel := ""
	rangeLabel := startLabel + " onward"
	if record.EndAt != nil {
		end := record.EndAt.In(location)
		endAt = record.EndAt.UTC().Format(time.RFC3339)
		endLabel = end.Format("Jan 2, 2006, 3:04 PM")
		rangeLabel = startLabel + " to " + endLabel
	}
	return RhythmMarkerDTO{
		MarkerID:      record.MarkerID,
		Kind:          record.Kind,
		KindLabel:     rhythmMarkerKindLabel(record.Kind),
		StartAt:       record.StartAt.UTC().Format(time.RFC3339),
		EndAt:         endAt,
		ZoneID:        record.ZoneID,
		CivilDate:     start.Format("2006-01-02"),
		Hour:          localClockHour(start),
		StartLabel:    startLabel,
		EndLabel:      endLabel,
		RangeLabel:    rangeLabel,
		Note:          record.Note,
		RecordedLabel: record.Provenance.RecordedAt.In(location).Format("Jan 2, 2006, 3:04 PM"),
	}, nil
}

func rhythmMarkerValues(input RhythmMarkerInput, now time.Time) (string, time.Time, *time.Time, string, string, error) {
	kind := strings.TrimSpace(input.Kind)
	if rhythmMarkerKindLabel(kind) == "" {
		return "", time.Time{}, nil, "", "", errors.New("marker kind must be travel, illness, disruption, or forced_schedule")
	}
	zoneID := strings.TrimSpace(input.ZoneID)
	if zoneID == "" {
		return "", time.Time{}, nil, "", "", errors.New("an explicit IANA time zone is required")
	}
	location, err := time.LoadLocation(zoneID)
	if err != nil {
		return "", time.Time{}, nil, "", "", fmt.Errorf("load time zone %q: %w", zoneID, err)
	}
	start, err := resolveRhythmMarkerCivil(strings.TrimSpace(input.StartLocal), location)
	if err != nil {
		return "", time.Time{}, nil, "", "", fmt.Errorf("marker start: %w", err)
	}
	if start.UTC().After(now.UTC()) {
		return "", time.Time{}, nil, "", "", errors.New("rhythm markers cannot start in the future")
	}
	var endAt *time.Time
	if rawEnd := strings.TrimSpace(input.EndLocal); rawEnd != "" {
		end, err := resolveRhythmMarkerCivil(rawEnd, location)
		if err != nil {
			return "", time.Time{}, nil, "", "", fmt.Errorf("marker end: %w", err)
		}
		if !end.After(start) {
			return "", time.Time{}, nil, "", "", errors.New("marker end must be after marker start")
		}
		if end.UTC().After(now.UTC()) {
			return "", time.Time{}, nil, "", "", errors.New("rhythm markers cannot end in the future")
		}
		endUTC := end.UTC()
		endAt = &endUTC
	}
	note := strings.TrimSpace(input.Note)
	if len(note) > 500 {
		return "", time.Time{}, nil, "", "", errors.New("marker note must be 500 characters or fewer")
	}
	return kind, start.UTC(), endAt, zoneID, note, nil
}

func resolveRhythmMarkerCivil(value string, location *time.Location) (time.Time, error) {
	if value == "" {
		return time.Time{}, errors.New("local date and time are required")
	}
	var parsed time.Time
	var err error
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02T15:04:05"} {
		parsed, err = time.Parse(layout, value)
		if err == nil {
			resolution, resolveErr := domain.ResolveCivilTime(location, parsed.Year(), parsed.Month(), parsed.Day(), parsed.Hour(), parsed.Minute(), parsed.Second())
			if resolveErr != nil {
				return time.Time{}, resolveErr
			}
			return resolution.Time, nil
		}
	}
	return time.Time{}, errors.New("use a local date and time")
}

func rhythmMarkerKindLabel(kind string) string {
	switch kind {
	case storage.RhythmMarkerTravel:
		return "Travel / time-zone context"
	case storage.RhythmMarkerIllness:
		return "Illness / health disruption"
	case storage.RhythmMarkerDisruption:
		return "Sleep disruption / awakening"
	case storage.RhythmMarkerForcedSchedule:
		return "Forced schedule / obligation"
	default:
		return ""
	}
}
