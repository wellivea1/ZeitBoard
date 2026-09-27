package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

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

// ReadSleepReviewPage reads one numbered page of the sleep log, newest night
// first, and how many nights there are. The log pages by number, and in one
// owner's store walking the start index past an offset costs little.
func (s *Store) ReadSleepReviewPage(ctx context.Context, offset, limit int) ([]sleepv1.ReviewContext, int, error) {
	if offset < 0 || limit < 1 {
		return nil, 0, errors.New("a sleep log page needs a non-negative offset and a positive size")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	var total int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM local_sleep_observations`).Scan(&total); err != nil {
		return nil, 0, err
	}
	observations, err := sleepObservationsWhere(ctx, tx, `ORDER BY start_at DESC, observation_id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	reviews, err := reviewEach(ctx, tx, observations)
	if err != nil {
		return nil, 0, err
	}
	return reviews, total, tx.Commit()
}

// ReadSleepReviewsBetween reads the nights that, as corrected, touch [start,
// end), earliest first. A correction can move a night into or out of the
// range, so every corrected night is reviewed along with those recorded there.
func (s *Store) ReadSleepReviewsBetween(ctx context.Context, start, end time.Time) ([]sleepv1.ReviewContext, error) {
	if !start.Before(end) {
		return nil, errors.New("a sleep range needs its start before its end")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	observations, err := sleepObservationsWhere(ctx, tx, `WHERE (start_at < ? AND end_at > ?)
		OR observation_id IN (SELECT target_observation_id FROM local_sleep_corrections)
		ORDER BY start_at, observation_id`, formatSQLiteTime(end), formatSQLiteTime(start))
	if err != nil {
		return nil, err
	}
	reviews, err := reviewEach(ctx, tx, observations)
	if err != nil {
		return nil, err
	}
	touching := reviews[:0]
	for _, review := range reviews {
		interval := review.Effective.Intervals[0].Interval
		if interval.Start.UTC.Before(end) && interval.End.UTC.After(start) {
			touching = append(touching, review)
		}
	}
	return touching, tx.Commit()
}

func sleepObservationsWhere(ctx context.Context, query queryContext, clause string, args ...any) ([]SleepObservationRecord, error) {
	var records []SleepObservationRecord
	err := readJSONRows(ctx, query, `SELECT payload_json FROM local_sleep_observations `+clause, func(value []byte) error {
		var record SleepObservationRecord
		if err := json.Unmarshal(value, &record); err != nil {
			return err
		}
		records = append(records, record)
		return nil
	}, args...)
	return records, err
}

// reviewEach reviews each observation with its own corrections.
func reviewEach(ctx context.Context, query queryContext, observations []SleepObservationRecord) ([]sleepv1.ReviewContext, error) {
	reviews := make([]sleepv1.ReviewContext, 0, len(observations))
	for _, observation := range observations {
		corrections, err := sleepCorrectionsForObservation(ctx, query, observation.ObservationID)
		if err != nil {
			return nil, err
		}
		review, err := sleepv1.Review(observation, corrections)
		if err != nil {
			return nil, err
		}
		reviews = append(reviews, review)
	}
	return reviews, nil
}
