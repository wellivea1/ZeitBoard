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

// ValidateCorrection checks a correction against the v1 contract: who it is
// and what it reviewed, where it came from, and what it changes, in that
// order.
func ValidateCorrection(record Correction) error {
	if err := validateCorrectionIdentity(record); err != nil {
		return err
	}
	if err := validateCorrectionProvenance(record); err != nil {
		return err
	}
	return validateCorrectionChanges(record.Changes)
}

// validateCorrectionIdentity checks the correction's id, its target's, and
// the corrections it says it reviewed.
func validateCorrectionIdentity(record Correction) error {
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
	return nil
}

// validateCorrectionProvenance checks where a correction came from. A
// provider's revision claims no user's review, and corrects only the
// timestamps of a source conflict.
func validateCorrectionProvenance(record Correction) error {
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
	if record.AcquisitionMethod != "" && record.AcquisitionMethod != AcquisitionManual && record.AcquisitionMethod != AcquisitionHealthConnect {
		return errors.New("correction acquisition_method must be manual or health_connect")
	}
	if record.AcquisitionMethod == AcquisitionHealthConnect &&
		(record.Reason != CorrectionSourceConflict || record.Changes.SleepClassification != nil || record.Changes.Excluded != nil) {
		return errors.New("Health Connect revisions may only correct source-conflict timestamps")
	}
	return nil
}

// validateCorrectionChanges checks that a correction changes something, and
// only into what the contract allows.
func validateCorrectionChanges(changes CorrectionChanges) error {
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
	targets, err := foldTargetsFrom(observations)
	if err != nil {
		return nil, err
	}
	byID, latestSourceRevision, err := checkedCorrections(corrections, targets)
	if err != nil {
		return nil, err
	}
	if err := validateCorrectionGraph(byID); err != nil {
		return nil, err
	}
	manualCounts := map[string]int{}
	for _, correction := range activeCorrections(byID, targets.originalRevisions) {
		position, ok := targets.index[correction.TargetObservationID]
		if !ok {
			return nil, fmt.Errorf("correction %s targets unknown sleep observation %s", correction.CorrectionID, correction.TargetObservationID)
		}
		// A night takes one user correction at a time, and only one made
		// against the source as it stands now.
		if correction.AcquisitionMethod != AcquisitionHealthConnect {
			manualCounts[correction.TargetObservationID]++
			if manualCounts[correction.TargetObservationID] > 1 ||
				!reviewedLatest(correction, targets.originalRevisions, latestSourceRevision) {
				return nil, ErrCorrectionReviewRequired
			}
		}
		if err := applyCorrection(&targets.sessions[position], correction, targets.zones[correction.TargetObservationID]); err != nil {
			return nil, fmt.Errorf("correction %s: %w", correction.CorrectionID, err)
		}
	}
	return targets.sessions, nil
}

// foldTargets are the observations corrections apply to, by id: each as a
// session, in its zone, with the revision it was recorded at and its source.
type foldTargets struct {
	sessions          []domain.SleepSession
	index             map[string]int
	zones             map[string]string
	originalRevisions map[string]time.Time
	acquisitions      map[string]string
}

func foldTargetsFrom(observations []Observation) (foldTargets, error) {
	targets := foldTargets{
		sessions:          make([]domain.SleepSession, 0, len(observations)),
		index:             make(map[string]int, len(observations)),
		zones:             make(map[string]string, len(observations)),
		originalRevisions: make(map[string]time.Time, len(observations)),
		acquisitions:      make(map[string]string, len(observations)),
	}
	for _, observation := range observations {
		if _, exists := targets.index[observation.ObservationID]; exists {
			return foldTargets{}, fmt.Errorf("duplicate observation %s", observation.ObservationID)
		}
		session, err := SessionFromObservation(observation)
		if err != nil {
			return foldTargets{}, fmt.Errorf("observation %s: %w", observation.ObservationID, err)
		}
		targets.index[observation.ObservationID] = len(targets.sessions)
		targets.zones[observation.ObservationID] = observation.ZoneID
		targets.acquisitions[observation.ObservationID] = observation.Provenance.AcquisitionMethod
		targets.originalRevisions[observation.ObservationID] = observation.Provenance.RecordedAt
		targets.sessions = append(targets.sessions, session)
	}
	return targets, nil
}

// checkedCorrections are the corrections by id, each valid and given once,
// with the latest revision each provider has made of its observation.
func checkedCorrections(corrections []Correction, targets foldTargets) (map[string]Correction, map[string]time.Time, error) {
	byID := make(map[string]Correction, len(corrections))
	latestSourceRevision := make(map[string]time.Time, len(targets.originalRevisions))
	for id, revision := range targets.originalRevisions {
		latestSourceRevision[id] = revision
	}
	for _, correction := range corrections {
		if err := ValidateCorrection(correction); err != nil {
			return nil, nil, fmt.Errorf("correction %s: %w", correction.CorrectionID, err)
		}
		if _, exists := byID[correction.CorrectionID]; exists {
			return nil, nil, fmt.Errorf("duplicate correction %s", correction.CorrectionID)
		}
		if correction.AcquisitionMethod == AcquisitionHealthConnect {
			if targets.acquisitions[correction.TargetObservationID] != AcquisitionHealthConnect {
				return nil, nil, errors.New("Health Connect revision targets a different observation source")
			}
			if correction.CreatedAt.After(latestSourceRevision[correction.TargetObservationID]) {
				latestSourceRevision[correction.TargetObservationID] = correction.CreatedAt
			}
		}
		byID[correction.CorrectionID] = correction
	}
	return byID, latestSourceRevision, nil
}

// activeCorrections are the leaves of each chain, provider revisions first,
// then by when each was made. Provider revisions establish the source
// baseline independently of the device's observed lineage. User corrections
// form a separate overlay; superseding a source record must not discard its
// unedited endpoint.
func activeCorrections(byID map[string]Correction, originalRevisions map[string]time.Time) []Correction {
	superseded := make(map[string]struct{}, len(byID))
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
	active := make([]Correction, 0, len(byID))
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
	return active
}

// reviewedLatest reports whether a user's correction was made against the
// source as it stands now: the revision it names, or else the original.
func reviewedLatest(correction Correction, originalRevisions, latestSourceRevision map[string]time.Time) bool {
	reviewed := originalRevisions[correction.TargetObservationID]
	if correction.BasedOnSourceRevision != nil {
		reviewed = *correction.BasedOnSourceRevision
	}
	return reviewed.Equal(latestSourceRevision[correction.TargetObservationID])
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

// Overlap is a night that more than one record describes: the records
// ResolveOverlaps merged, earliest first, and the session that stands in
// their place.
type Overlap struct {
	ObservationIDs []string
	Merged         domain.SleepSession
}

// Overlaps lists the sessions ResolveOverlaps forms from more than one record,
// earliest first, exactly as the estimator receives them.
func Overlaps(sessions []domain.SleepSession) ([]Overlap, error) {
	resolved, err := ResolveOverlaps(sessions)
	if err != nil {
		return nil, err
	}
	var overlaps []Overlap
	for _, session := range resolved {
		if ids := mergedObservationIDs(session); len(ids) > 1 {
			overlaps = append(overlaps, Overlap{ObservationIDs: ids, Merged: session})
		}
	}
	return overlaps, nil
}

// mergedObservationIDs is each record a session's evidence names, once.
func mergedObservationIDs(session domain.SleepSession) []string {
	seen := map[domain.ObservationID]bool{}
	var ids []string
	for _, interval := range session.Intervals {
		for _, evidence := range []domain.Evidence{interval.StartEvidence, interval.EndEvidence} {
			for _, id := range evidence.ObservationIDs {
				if !seen[id] {
					seen[id] = true
					ids = append(ids, string(id))
				}
			}
		}
	}
	return ids
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
