package syncmodel

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"time"

	"non24.app/core/domain"
)

var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,63}$`)

func ValidatePushRequest(req *PushRequest) error {
	if req.SchemaVersion != SchemaVersion {
		return errors.New("unsupported schema version")
	}
	if len(req.Records) == 0 {
		return errors.New("at least one record is required")
	}
	if len(req.Records) > MaxRecordsPerPush {
		return errors.New("too many records")
	}

	seen := map[string]struct{}{}
	for i := range req.Records {
		record := &req.Records[i]
		if err := validateIdentifier(record.RecordID, "recordId"); err != nil {
			return err
		}
		if _, ok := seen[record.RecordID]; ok {
			return fmt.Errorf("duplicate recordId %q", record.RecordID)
		}
		seen[record.RecordID] = struct{}{}
		if record.Kind != KindObservation && record.Kind != KindCorrection {
			return errors.New("unsupported record kind")
		}
		if record.CreatedAt.IsZero() {
			return errors.New("createdAt is required")
		}
		record.CreatedAt = record.CreatedAt.UTC()
		if len(record.Payload) == 0 {
			return errors.New("payload is required")
		}
		if len(record.Payload) > MaxPayloadBytes {
			return errors.New("payload is too large")
		}
		compact, err := compactJSON(record.Payload)
		if err != nil {
			return errors.New("payload must be valid JSON")
		}
		record.Payload = compact
		if err := validatePayload(record.Kind, record.RecordID, record.Payload); err != nil {
			return err
		}
	}
	return nil
}

func validatePayload(kind Kind, recordID string, payload json.RawMessage) error {
	switch kind {
	case KindObservation:
		return validateObservation(recordID, payload)
	case KindCorrection:
		return validateCorrection(recordID, payload)
	default:
		return errors.New("unsupported record kind")
	}
}

func compactJSON(data []byte) (json.RawMessage, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, data); err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), buf.Bytes()...), nil
}

type observationPayload struct {
	ObservationID string           `json:"observation_id"`
	Kind          string           `json:"kind"`
	StartAt       time.Time        `json:"start_at"`
	EndAt         time.Time        `json:"end_at"`
	ZoneID        string           `json:"zone_id"`
	Sleep         *sleepPayload    `json:"sleep,omitempty"`
	Activity      *activityPayload `json:"activity,omitempty"`
	Provenance    provenance       `json:"provenance"`
}

type sleepPayload struct {
	Classification string `json:"classification"`
}

type activityPayload struct {
	Level string `json:"level"`
}

type provenance struct {
	AcquisitionMethod string    `json:"acquisition_method"`
	EvidenceStatus    string    `json:"evidence_status"`
	RecordedAt        time.Time `json:"recorded_at"`
	SourceRecordID    string    `json:"source_record_id,omitempty"`
}

func validateObservation(recordID string, payload json.RawMessage) error {
	var obs observationPayload
	if err := decodeStrict(payload, &obs); err != nil {
		return errors.New("invalid observation payload")
	}
	if obs.ObservationID != recordID {
		return errors.New("recordId must match observation_id")
	}
	if err := validateIdentifier(obs.ObservationID, "observation_id"); err != nil {
		return err
	}
	if obs.StartAt.IsZero() || obs.EndAt.IsZero() {
		return errors.New("observation start_at and end_at are required")
	}
	start, err := domain.NewZonedInstant(obs.StartAt, obs.ZoneID)
	if err != nil {
		return fmt.Errorf("observation start_at: %w", err)
	}
	end, err := domain.NewZonedInstant(obs.EndAt, obs.ZoneID)
	if err != nil {
		return fmt.Errorf("observation end_at: %w", err)
	}
	if err := (domain.TimeRange{Start: start, End: end}).Validate(); err != nil {
		return fmt.Errorf("observation interval: %w", err)
	}
	switch obs.Kind {
	case "sleep_episode":
		if obs.Sleep == nil || obs.Activity != nil {
			return errors.New("sleep_episode requires sleep and forbids activity")
		}
		if !oneOf(obs.Sleep.Classification, "principal", "nap", "unknown") {
			return errors.New("invalid sleep classification")
		}
	case "activity_interval":
		if obs.Activity == nil || obs.Sleep != nil {
			return errors.New("activity_interval requires activity and forbids sleep")
		}
		if !oneOf(obs.Activity.Level, "idle", "active", "unknown") {
			return errors.New("invalid activity level")
		}
	default:
		return errors.New("invalid observation kind")
	}
	if !oneOf(obs.Provenance.AcquisitionMethod, "manual", "health_connect", "os_activity", "file_import", "synthetic") {
		return errors.New("invalid acquisition method")
	}
	if !oneOf(obs.Provenance.EvidenceStatus, "directly_observed", "user_reported", "inferred") {
		return errors.New("invalid evidence status")
	}
	if obs.Provenance.RecordedAt.IsZero() {
		return errors.New("provenance recorded_at is required")
	}
	if len(obs.Provenance.SourceRecordID) > 128 {
		return errors.New("source_record_id is too long")
	}
	return nil
}

type correctionPayload struct {
	CorrectionID           string            `json:"correction_id"`
	TargetObservationID    string            `json:"target_observation_id"`
	SupersedesCorrectionID string            `json:"supersedes_correction_id,omitempty"`
	CreatedAt              time.Time         `json:"created_at"`
	Reason                 string            `json:"reason"`
	Changes                correctionChanges `json:"changes"`
}

type correctionChanges struct {
	StartAt             *time.Time `json:"start_at,omitempty"`
	EndAt               *time.Time `json:"end_at,omitempty"`
	SleepClassification *string    `json:"sleep_classification,omitempty"`
	Excluded            *bool      `json:"excluded,omitempty"`
}

func validateCorrection(recordID string, payload json.RawMessage) error {
	var correction correctionPayload
	if err := decodeStrict(payload, &correction); err != nil {
		return errors.New("invalid correction payload")
	}
	if correction.CorrectionID != recordID {
		return errors.New("recordId must match correction_id")
	}
	if err := validateIdentifier(correction.CorrectionID, "correction_id"); err != nil {
		return err
	}
	if err := validateIdentifier(correction.TargetObservationID, "target_observation_id"); err != nil {
		return err
	}
	if correction.SupersedesCorrectionID != "" {
		if err := validateIdentifier(correction.SupersedesCorrectionID, "supersedes_correction_id"); err != nil {
			return err
		}
	}
	if correction.CreatedAt.IsZero() {
		return errors.New("created_at is required")
	}
	if !oneOf(correction.Reason, "user_edit", "duplicate", "invalid_range", "source_conflict") {
		return errors.New("invalid correction reason")
	}
	if correction.Changes.StartAt == nil && correction.Changes.EndAt == nil &&
		correction.Changes.SleepClassification == nil && correction.Changes.Excluded == nil {
		return errors.New("correction changes must not be empty")
	}
	if correction.Changes.SleepClassification != nil &&
		!oneOf(*correction.Changes.SleepClassification, "principal", "nap", "unknown") {
		return errors.New("invalid sleep classification")
	}
	if correction.Changes.StartAt != nil && correction.Changes.EndAt != nil &&
		!correction.Changes.EndAt.After(*correction.Changes.StartAt) {
		return errors.New("correction end_at must be after start_at")
	}
	return nil
}

func decodeStrict(data []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("multiple JSON values are not allowed")
	}
	return nil
}

func validateIdentifier(value, field string) error {
	if !identifierPattern.MatchString(value) {
		return fmt.Errorf("%s is invalid", field)
	}
	return nil
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}
