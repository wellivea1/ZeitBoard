package sleepv1

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"

	"non24.app/core/domain"
)

// ReviewContext preserves the provider baseline and disputed manual edits so a
// user can resolve a refusal without presenting an unreviewed edit as fact.
type ReviewContext struct {
	Observation       Observation
	Corrections       []Correction
	ObservationID     string
	SourceRevision    time.Time
	Source            domain.SleepSession
	Effective         domain.SleepSession
	ManualCorrections []Correction
	NeedsReview       bool
}

func Review(observation Observation, corrections []Correction) (ReviewContext, error) {
	result := ReviewContext{Observation: observation, Corrections: corrections, ObservationID: observation.ObservationID, SourceRevision: observation.Provenance.RecordedAt, ManualCorrections: []Correction{}}
	provider := []Correction{}
	byID := map[string]Correction{}
	for _, correction := range corrections {
		if correction.TargetObservationID != observation.ObservationID {
			return result, errors.New("review includes another observation")
		}
		if err := ValidateCorrection(correction); err != nil {
			return result, err
		}
		if _, exists := byID[correction.CorrectionID]; exists {
			return result, errors.New("duplicate correction in review")
		}
		byID[correction.CorrectionID] = correction
		if correction.AcquisitionMethod == AcquisitionHealthConnect {
			provider = append(provider, correction)
			if correction.CreatedAt.After(result.SourceRevision) {
				result.SourceRevision = correction.CreatedAt
			}
		}
	}
	if err := validateCorrectionGraph(byID); err != nil {
		return result, err
	}
	source, err := Fold([]Observation{observation}, provider)
	if err != nil {
		return result, err
	}
	result.Source, result.Effective = source[0], source[0]
	superseded := map[string]bool{}
	for _, correction := range corrections {
		if correction.AcquisitionMethod != AcquisitionHealthConnect {
			for _, id := range correction.SupersedesCorrectionIDs {
				superseded[id] = true
			}
		}
	}
	for _, correction := range corrections {
		if correction.AcquisitionMethod != AcquisitionHealthConnect && !superseded[correction.CorrectionID] {
			result.ManualCorrections = append(result.ManualCorrections, correction)
		}
	}
	sort.Slice(result.ManualCorrections, func(i, j int) bool {
		return result.ManualCorrections[i].CorrectionID < result.ManualCorrections[j].CorrectionID
	})
	if len(result.ManualCorrections) > 256 {
		return result, errors.New("too many concurrent edits for one review")
	}
	effective, err := Fold([]Observation{observation}, corrections)
	if err != nil {
		result.NeedsReview = true
	} else {
		result.Effective = effective[0]
	}
	return result, nil
}

func (r ReviewContext) CorrectionIDs() []string {
	ids := make([]string, len(r.ManualCorrections))
	for i, correction := range r.ManualCorrections {
		ids[i] = correction.CorrectionID
	}
	return ids
}

// Token binds a form to the immutable source revision and manual heads shown.
func (r ReviewContext) Token() string {
	digest := sha256.Sum256([]byte(r.ObservationID + "\x00" + r.SourceRevision.UTC().Format(time.RFC3339Nano) + "\x00" + strings.Join(r.CorrectionIDs(), "\x00")))
	return hex.EncodeToString(digest[:])
}
