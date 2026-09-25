// Package sleepv1 owns the current sleep wire records and their authoritative
// conversion into the effective domain read model.
package sleepv1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"time"

	"non24.app/core/domain"
	"non24.app/core/ingest"
)

const (
	KindEpisode = "sleep_episode"

	ClassificationPrincipal = "principal"
	ClassificationNap       = "nap"
	ClassificationUnknown   = "unknown"

	AcquisitionManual        = "manual"
	AcquisitionHealthConnect = "health_connect"
	AcquisitionOSActivity    = "os_activity"
	AcquisitionFileImport    = "file_import"
	AcquisitionSynthetic     = "synthetic"

	EvidenceDirectlyObserved = "directly_observed"
	EvidenceUserReported     = "user_reported"
	EvidenceInferred         = "inferred"

	CorrectionUserEdit       = "user_edit"
	CorrectionDuplicate      = "duplicate"
	CorrectionInvalidRange   = "invalid_range"
	CorrectionSourceConflict = "source_conflict"
)

var ErrCorrectionReviewRequired = errors.New("sleep corrections require review of the current source and manual edits")

var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,63}$`)

type Observation struct {
	ObservationID string     `json:"observation_id"`
	Kind          string     `json:"kind"`
	StartAt       time.Time  `json:"start_at"`
	EndAt         time.Time  `json:"end_at"`
	ZoneID        string     `json:"zone_id"`
	Sleep         Details    `json:"sleep"`
	Provenance    Provenance `json:"provenance"`
}

type Details struct {
	Classification string `json:"classification"`
}

type Provenance struct {
	AcquisitionMethod string    `json:"acquisition_method"`
	EvidenceStatus    string    `json:"evidence_status"`
	RecordedAt        time.Time `json:"recorded_at"`
	SourceRecordID    string    `json:"source_record_id,omitempty"`
}

type Correction struct {
	CorrectionID            string            `json:"correction_id"`
	TargetObservationID     string            `json:"target_observation_id"`
	SupersedesCorrectionIDs []string          `json:"supersedes_correction_ids,omitempty"`
	BasedOnSourceRevision   *time.Time        `json:"based_on_source_revision,omitempty"`
	CreatedAt               time.Time         `json:"created_at"`
	Reason                  string            `json:"reason"`
	AcquisitionMethod       string            `json:"acquisition_method,omitempty"`
	Changes                 CorrectionChanges `json:"changes"`
}

type CorrectionChanges struct {
	StartAt             *time.Time `json:"start_at,omitempty"`
	EndAt               *time.Time `json:"end_at,omitempty"`
	SleepClassification *string    `json:"sleep_classification,omitempty"`
	Excluded            *bool      `json:"excluded,omitempty"`
}

func DecodeObservation(payload []byte) (Observation, error) {
	var observation Observation
	if err := decodeStrict(payload, &observation); err != nil {
		return Observation{}, err
	}
	if err := ValidateObservation(observation); err != nil {
		return Observation{}, err
	}
	return observation, nil
}

func DecodeCorrection(payload []byte) (Correction, error) {
	var correction Correction
	if err := decodeStrict(payload, &correction); err != nil {
		return Correction{}, err
	}
	if err := ValidateCorrection(correction); err != nil {
		return Correction{}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return Correction{}, err
	}
	if _, present := fields["acquisition_method"]; present && correction.AcquisitionMethod == "" {
		return Correction{}, errors.New("correction acquisition_method cannot be empty or null")
	}
	if _, present := fields["based_on_source_revision"]; present && correction.BasedOnSourceRevision == nil {
		return Correction{}, errors.New("reviewed source revision cannot be null")
	}
	if raw, present := fields["supersedes_correction_ids"]; present && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return Correction{}, errors.New("reviewed correction identifiers cannot be null")
	}
	if correction.AcquisitionMethod == AcquisitionHealthConnect {
		if _, present := fields["supersedes_correction_ids"]; present {
			return Correction{}, errors.New("provider revision cannot claim manual review")
		}
	}
	return correction, nil
}

func ValidateObservation(record Observation) error {
	if !identifierPattern.MatchString(record.ObservationID) {
		return errors.New("observation_id must match the v1 identifier format")
	}
	if record.Kind != KindEpisode {
		return errors.New("sleep observation kind must be sleep_episode")
	}
	if !ValidClassification(record.Sleep.Classification) {
		return errors.New("sleep.classification must be principal, nap, or unknown")
	}
	if !ValidAcquisition(record.Provenance.AcquisitionMethod) {
		return errors.New("provenance.acquisition_method is not supported")
	}
	if !ValidEvidenceStatus(record.Provenance.EvidenceStatus) {
		return errors.New("provenance.evidence_status is not supported")
	}
	if record.Provenance.RecordedAt.IsZero() {
		return errors.New("provenance.recorded_at is required")
	}
	if len(record.Provenance.SourceRecordID) > 128 {
		return errors.New("provenance.source_record_id is too long")
	}
	start, err := domain.NewZonedInstant(record.StartAt, record.ZoneID)
	if err != nil {
		return fmt.Errorf("start_at: %w", err)
	}
	end, err := domain.NewZonedInstant(record.EndAt, record.ZoneID)
	if err != nil {
		return fmt.Errorf("end_at: %w", err)
	}
	if err := (domain.TimeRange{Start: start, End: end}).Validate(); err != nil {
		return fmt.Errorf("interval: %w", err)
	}
	return nil
}

func ValidateCorrection(record Correction) error {
	if !identifierPattern.MatchString(record.CorrectionID) {
		return errors.New("correction_id must match the v1 identifier format")
	}
	if !identifierPattern.MatchString(record.TargetObservationID) {
		return errors.New("target_observation_id must match the v1 identifier format")
	}
	if len(record.SupersedesCorrectionIDs) > 256 {
		return errors.New("too many reviewed corrections")
	}
	seen := map[string]bool{}
	for _, id := range record.SupersedesCorrectionIDs {
		if !identifierPattern.MatchString(id) || id == record.CorrectionID || seen[id] {
			return errors.New("reviewed correction identifiers must be unique and valid")
		}
		seen[id] = true
	}
	if record.BasedOnSourceRevision != nil && record.BasedOnSourceRevision.IsZero() {
		return errors.New("source revision must be an instant")
	}
	if record.AcquisitionMethod == AcquisitionHealthConnect && (record.BasedOnSourceRevision != nil || len(record.SupersedesCorrectionIDs) != 0) {
		return errors.New("provider revisions cannot claim a user's review")
	}
	if record.CreatedAt.IsZero() {
		return errors.New("created_at is required")
	}
	if !ValidCorrectionReason(record.Reason) {
		return errors.New("correction reason is not supported")
	}
	changes := record.Changes
	if record.AcquisitionMethod != "" && record.AcquisitionMethod != AcquisitionManual && record.AcquisitionMethod != AcquisitionHealthConnect {
		return errors.New("correction acquisition_method must be manual or health_connect")
	}
	if record.AcquisitionMethod == AcquisitionHealthConnect &&
		(record.Reason != CorrectionSourceConflict || changes.SleepClassification != nil || changes.Excluded != nil) {
		return errors.New("Health Connect revisions may only correct source-conflict timestamps")
	}
	count := 0
	if changes.StartAt != nil {
		count++
	}
	if changes.EndAt != nil {
		count++
	}
	if changes.SleepClassification != nil {
		if !ValidClassification(*changes.SleepClassification) {
			return errors.New("changes.sleep_classification must be principal, nap, or unknown")
		}
		count++
	}
	if changes.Excluded != nil {
		count++
	}
	if count == 0 {
		return errors.New("correction changes must not be empty")
	}
	if changes.StartAt != nil && changes.EndAt != nil && !changes.EndAt.After(*changes.StartAt) {
		return errors.New("changes.end_at must be after changes.start_at")
	}
	return nil
}

// Fold applies the active leaves of append-only correction chains to immutable
// observations. Correction instants are always rendered in the target zone.
func Fold(observations []Observation, corrections []Correction) ([]domain.SleepSession, error) {
	sessions := make([]domain.SleepSession, 0, len(observations))
	index := make(map[string]int, len(observations))
	zones := make(map[string]string, len(observations))
	originalRevisions := make(map[string]time.Time, len(observations))
	acquisitions := make(map[string]string, len(observations))
	for _, observation := range observations {
		if _, exists := index[observation.ObservationID]; exists {
			return nil, fmt.Errorf("duplicate observation %s", observation.ObservationID)
		}
		session, err := SessionFromObservation(observation)
		if err != nil {
			return nil, fmt.Errorf("observation %s: %w", observation.ObservationID, err)
		}
		index[observation.ObservationID] = len(sessions)
		zones[observation.ObservationID] = observation.ZoneID
		acquisitions[observation.ObservationID] = observation.Provenance.AcquisitionMethod
		originalRevisions[observation.ObservationID] = observation.Provenance.RecordedAt
		sessions = append(sessions, session)
	}

	byID := make(map[string]Correction, len(corrections))
	latestSourceRevision := make(map[string]time.Time, len(originalRevisions))
	for id, revision := range originalRevisions {
		latestSourceRevision[id] = revision
	}
	for _, correction := range corrections {
		if err := ValidateCorrection(correction); err != nil {
			return nil, fmt.Errorf("correction %s: %w", correction.CorrectionID, err)
		}
		if _, exists := byID[correction.CorrectionID]; exists {
			return nil, fmt.Errorf("duplicate correction %s", correction.CorrectionID)
		}
		acquisition := acquisitions[correction.TargetObservationID]
		if correction.AcquisitionMethod == AcquisitionHealthConnect {
			if acquisition != AcquisitionHealthConnect {
				return nil, errors.New("Health Connect revision targets a different observation source")
			}
			if correction.CreatedAt.After(latestSourceRevision[correction.TargetObservationID]) {
				latestSourceRevision[correction.TargetObservationID] = correction.CreatedAt
			}
		}
		byID[correction.CorrectionID] = correction
	}
	if err := validateCorrectionGraph(byID); err != nil {
		return nil, err
	}

	// Provider revisions establish the source baseline independently of the
	// device's observed lineage. User corrections form a separate overlay;
	// superseding a source record must not discard its unedited endpoint.
	superseded := make(map[string]struct{}, len(corrections))
	for _, correction := range byID {
		if correction.AcquisitionMethod == AcquisitionHealthConnect {
			continue
		}
		for _, parentID := range correction.SupersedesCorrectionIDs {
			if parent, ok := byID[parentID]; ok && parent.AcquisitionMethod != AcquisitionHealthConnect {
				superseded[parent.CorrectionID] = struct{}{}
			}
		}
	}
	active := make([]Correction, 0, len(corrections))
	for _, correction := range byID {
		if correction.AcquisitionMethod == AcquisitionHealthConnect && correction.CreatedAt.Before(originalRevisions[correction.TargetObservationID]) {
			continue
		}
		if _, inactive := superseded[correction.CorrectionID]; !inactive {
			active = append(active, correction)
		}
	}
	sort.Slice(active, func(i, j int) bool {
		leftSource, rightSource := active[i].AcquisitionMethod == AcquisitionHealthConnect, active[j].AcquisitionMethod == AcquisitionHealthConnect
		if leftSource != rightSource {
			return leftSource
		}
		if active[i].CreatedAt.Equal(active[j].CreatedAt) {
			return active[i].CorrectionID < active[j].CorrectionID
		}
		return active[i].CreatedAt.Before(active[j].CreatedAt)
	})
	manualCounts := map[string]int{}
	for _, correction := range active {
		position, ok := index[correction.TargetObservationID]
		if !ok {
			return nil, fmt.Errorf("correction %s targets unknown sleep observation %s", correction.CorrectionID, correction.TargetObservationID)
		}
		if correction.AcquisitionMethod != AcquisitionHealthConnect {
			manualCounts[correction.TargetObservationID]++
			if manualCounts[correction.TargetObservationID] > 1 {
				return nil, ErrCorrectionReviewRequired
			}
			reviewed := originalRevisions[correction.TargetObservationID]
			if correction.BasedOnSourceRevision != nil {
				reviewed = *correction.BasedOnSourceRevision
			}
			if !reviewed.Equal(latestSourceRevision[correction.TargetObservationID]) {
				return nil, ErrCorrectionReviewRequired
			}
		}
		if err := applyCorrection(&sessions[position], correction, zones[correction.TargetObservationID]); err != nil {
			return nil, fmt.Errorf("correction %s: %w", correction.CorrectionID, err)
		}
	}
	return sessions, nil
}

func SessionFromObservation(observation Observation) (domain.SleepSession, error) {
	if err := ValidateObservation(observation); err != nil {
		return domain.SleepSession{}, err
	}
	start, err := domain.NewZonedInstant(observation.StartAt, observation.ZoneID)
	if err != nil {
		return domain.SleepSession{}, err
	}
	end, err := domain.NewZonedInstant(observation.EndAt, observation.ZoneID)
	if err != nil {
		return domain.SleepSession{}, err
	}
	evidence := domain.Evidence{
		Acquisition:    acquisitionKind(observation.Provenance.AcquisitionMethod),
		Status:         evidenceStatus(observation.Provenance.EvidenceStatus),
		ObservationIDs: []domain.ObservationID{domain.ObservationID(observation.ObservationID)},
		RecordedAt:     observation.Provenance.RecordedAt.UTC(),
	}
	if observation.Provenance.SourceRecordID != "" {
		evidence.SourceIDs = []domain.DataSourceID{domain.DataSourceID(observation.Provenance.SourceRecordID)}
	}
	classification := domain.SleepClassification(observation.Sleep.Classification)
	return domain.SleepSession{
		ID:             domain.SleepSessionID(observation.ObservationID),
		Classification: classification,
		IsNap:          classification == domain.SleepClassificationNap,
		SourceLabel:    sourceLabel(observation.Provenance.AcquisitionMethod),
		CreatedAt:      observation.Provenance.RecordedAt.UTC(),
		Intervals: []domain.SleepInterval{{
			Interval:      domain.TimeRange{Start: start, End: end},
			StartEvidence: evidence,
			EndEvidence:   evidence,
		}},
	}, nil
}

// ResolveOverlaps retains the existing overlap resolver while preventing
// suppressed or non-principal reports from being merged into principal sleep.
func ResolveOverlaps(sessions []domain.SleepSession) ([]domain.SleepSession, error) {
	groups := map[domain.SleepClassification][]domain.SleepSession{}
	var result []domain.SleepSession
	for _, session := range sessions {
		if len(session.Intervals) == 0 {
			return nil, fmt.Errorf("sleep report %s has no interval", session.ID)
		}
		if err := session.Intervals[0].Interval.Validate(); err != nil {
			return nil, fmt.Errorf("sleep report %s: %w", session.ID, err)
		}
		if session.Suppressed {
			result = append(result, session)
			continue
		}
		classification := session.EffectiveClassification()
		groups[classification] = append(groups[classification], session)
	}
	for _, classification := range []domain.SleepClassification{
		domain.SleepClassificationPrincipal,
		domain.SleepClassificationNap,
		domain.SleepClassificationUnknown,
	} {
		resolved, err := ingest.ResolveOverlappingSleepReports(groups[classification])
		if err != nil {
			return nil, err
		}
		for i := range resolved {
			resolved[i].Classification = classification
			resolved[i].IsNap = classification == domain.SleepClassificationNap
		}
		result = append(result, resolved...)
	}
	sort.Slice(result, func(i, j int) bool {
		left := result[i].Intervals[0].Interval.Start.UTC
		right := result[j].Intervals[0].Interval.Start.UTC
		if left.Equal(right) {
			return result[i].ID < result[j].ID
		}
		return left.Before(right)
	})
	return result, nil
}

func applyCorrection(session *domain.SleepSession, correction Correction, zoneID string) error {
	if len(session.Intervals) == 0 {
		return errors.New("target session has no intervals")
	}
	interval := &session.Intervals[0]
	if correction.Changes.StartAt != nil {
		start, err := domain.NewZonedInstant(*correction.Changes.StartAt, zoneID)
		if err != nil {
			return err
		}
		interval.Interval.Start = start
		interval.StartEvidence = correctedEvidence(interval.StartEvidence, domain.CorrectionID(correction.CorrectionID), correction)
	}
	if correction.Changes.EndAt != nil {
		end, err := domain.NewZonedInstant(*correction.Changes.EndAt, zoneID)
		if err != nil {
			return err
		}
		interval.Interval.End = end
		interval.EndEvidence = correctedEvidence(interval.EndEvidence, domain.CorrectionID(correction.CorrectionID+"_end"), correction)
	}
	if correction.Changes.SleepClassification != nil {
		classification := domain.SleepClassification(*correction.Changes.SleepClassification)
		session.Classification = classification
		session.IsNap = classification == domain.SleepClassificationNap
	}
	if correction.Changes.Excluded != nil {
		session.Suppressed = *correction.Changes.Excluded
	}
	if err := interval.Interval.Validate(); err != nil {
		return fmt.Errorf("creates invalid interval: %w", err)
	}
	return nil
}

func validateCorrectionGraph(byID map[string]Correction) error {
	const (
		_ = iota
		visiting
		visited
	)
	state := make(map[string]int, len(byID))
	var visit func(string) error
	visit = func(id string) error {
		switch state[id] {
		case visiting:
			return fmt.Errorf("correction chain contains a cycle at %s", id)
		case visited:
			return nil
		}
		state[id] = visiting
		record := byID[id]
		for _, parentID := range record.SupersedesCorrectionIDs {
			if parent, ok := byID[parentID]; ok {
				if parent.TargetObservationID != record.TargetObservationID {
					return fmt.Errorf("correction %s reviews another observation", id)
				}
				if err := visit(parentID); err != nil {
					return err
				}
			}
		}
		state[id] = visited
		return nil
	}
	for id := range byID {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func correctedEvidence(source domain.Evidence, correctionID domain.CorrectionID, correction Correction) domain.Evidence {
	result := source
	result.SourceIDs = append([]domain.DataSourceID(nil), source.SourceIDs...)
	result.ObservationIDs = append([]domain.ObservationID(nil), source.ObservationIDs...)
	result.CorrectionIDs = append([]domain.CorrectionID(nil), source.CorrectionIDs...)
	result.Status = domain.StatusUserConfirmed
	result.RecordedAt = correction.CreatedAt.UTC()
	if correction.AcquisitionMethod == AcquisitionHealthConnect {
		result.Acquisition = domain.AcquisitionImported
		result.Status = domain.StatusObserved
		result.RecordedAt = correction.CreatedAt.UTC()
	}
	result.CorrectionIDs = append(result.CorrectionIDs, correctionID)
	return result
}

func ValidClassification(value string) bool {
	return value == ClassificationPrincipal || value == ClassificationNap || value == ClassificationUnknown
}

func ValidAcquisition(value string) bool {
	switch value {
	case AcquisitionManual, AcquisitionHealthConnect, AcquisitionOSActivity, AcquisitionFileImport, AcquisitionSynthetic:
		return true
	default:
		return false
	}
}

func ValidEvidenceStatus(value string) bool {
	switch value {
	case EvidenceDirectlyObserved, EvidenceUserReported, EvidenceInferred:
		return true
	default:
		return false
	}
}

func ValidCorrectionReason(value string) bool {
	switch value {
	case CorrectionUserEdit, CorrectionDuplicate, CorrectionInvalidRange, CorrectionSourceConflict:
		return true
	default:
		return false
	}
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("multiple JSON values are not allowed")
	}
	return nil
}

func acquisitionKind(value string) domain.AcquisitionKind {
	switch value {
	case AcquisitionHealthConnect, AcquisitionFileImport:
		return domain.AcquisitionImported
	case AcquisitionOSActivity:
		return domain.AcquisitionCollected
	default:
		return domain.AcquisitionManual
	}
}

func evidenceStatus(value string) domain.EvidenceStatus {
	switch value {
	case EvidenceDirectlyObserved:
		return domain.StatusObserved
	case EvidenceInferred:
		return domain.StatusInferred
	default:
		return domain.StatusUserConfirmed
	}
}

func sourceLabel(value string) string {
	switch value {
	case AcquisitionHealthConnect:
		return "Health Connect sleep"
	case AcquisitionFileImport:
		return "Imported sleep"
	case AcquisitionOSActivity:
		return "Device activity"
	case AcquisitionSynthetic:
		return "Synthetic sleep"
	default:
		return "Manual sleep log"
	}
}
