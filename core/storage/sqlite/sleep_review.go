package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"

	"non24.app/core/sleepv1"
)

func (s *Store) SleepReview(ctx context.Context, observationID string) (sleepv1.ReviewContext, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return sleepv1.ReviewContext{}, err
	}
	defer tx.Rollback()
	var payload []byte
	if err := tx.QueryRowContext(ctx, `SELECT payload_json FROM local_sleep_observations WHERE observation_id = ?`, observationID).Scan(&payload); err != nil {
		return sleepv1.ReviewContext{}, err
	}
	var observation SleepObservationRecord
	if err := json.Unmarshal(payload, &observation); err != nil {
		return sleepv1.ReviewContext{}, err
	}
	corrections, err := sleepCorrectionsForObservation(ctx, tx, observationID)
	if err != nil {
		return sleepv1.ReviewContext{}, err
	}
	review, err := sleepv1.Review(observation, corrections)
	if err != nil {
		return review, err
	}
	return review, tx.Commit()
}

// ReadSleepReviews keeps the log editable even when disputed corrections
// prevent an estimator snapshot. It never supplies fallback sessions to estimation.
func (s *Store) ReadSleepReviews(ctx context.Context) ([]sleepv1.ReviewContext, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	observations, err := listSleepObservations(ctx, tx)
	if err != nil {
		return nil, err
	}
	corrections, err := listSleepCorrections(ctx, tx)
	if err != nil {
		return nil, err
	}
	byTarget := map[string][]sleepv1.Correction{}
	for _, correction := range corrections {
		byTarget[correction.TargetObservationID] = append(byTarget[correction.TargetObservationID], correction)
	}
	reviews := make([]sleepv1.ReviewContext, 0, len(observations))
	for _, observation := range observations {
		review, err := sleepv1.Review(observation, byTarget[observation.ObservationID])
		if err != nil {
			return nil, err
		}
		reviews = append(reviews, review)
	}
	return reviews, tx.Commit()
}
