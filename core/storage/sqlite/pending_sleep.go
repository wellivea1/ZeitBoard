package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// A pending sleep is a recorded intent, not an observation.
//
// One-tap logging marks "I am going to sleep" now and "I woke up" later, and
// only the pair describes an episode. Appending something at the first tap
// would put a row in the append-only log whose end had not happened yet, and
// correcting it afterwards would leave a permanent record of a boundary nobody
// observed. So the first tap parks a single row here and the observation is
// written when the second one lands.
//
// There is at most one. A second "going to sleep" replaces the first, because
// the newer tap is the current intent.

// PendingSleepRecord is the parked onset.
type PendingSleepRecord struct {
	StartedAt time.Time
	ZoneID    string
	MarkedAt  time.Time
}

// SetPendingSleep replaces any parked onset with this one.
func (s *Store) SetPendingSleep(ctx context.Context, record PendingSleepRecord) error {
	if record.StartedAt.IsZero() {
		return errors.New("pending sleep requires a start time")
	}
	if strings.TrimSpace(record.ZoneID) == "" {
		return errors.New("pending sleep requires a zone")
	}
	markedAt := record.MarkedAt
	if markedAt.IsZero() {
		markedAt = record.StartedAt
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO local_sleep_pending(id, started_at, zone_id, marked_at)
		VALUES(1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			started_at = excluded.started_at,
			zone_id = excluded.zone_id,
			marked_at = excluded.marked_at`,
		record.StartedAt.UTC().Format(time.RFC3339Nano),
		record.ZoneID,
		markedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

// PendingSleep returns the parked onset, or nil when there is none.
func (s *Store) PendingSleep(ctx context.Context) (*PendingSleepRecord, error) {
	var startedAt, zoneID, markedAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT started_at, zone_id, marked_at FROM local_sleep_pending WHERE id = 1`).
		Scan(&startedAt, &zoneID, &markedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	started, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return nil, err
	}
	marked, err := time.Parse(time.RFC3339Nano, markedAt)
	if err != nil {
		return nil, err
	}
	return &PendingSleepRecord{
		StartedAt: started.UTC(),
		ZoneID:    zoneID,
		MarkedAt:  marked.UTC(),
	}, nil
}

// ClearPendingSleep discards the parked onset. It is called once the episode
// has been appended, and when the person abandons it.
func (s *Store) ClearPendingSleep(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM local_sleep_pending WHERE id = 1`)
	return err
}
