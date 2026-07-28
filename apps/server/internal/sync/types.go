package syncmodel

import (
	"encoding/json"
	"time"
)

const (
	SchemaVersion     = "v1"
	MaxRecordsPerPush = 100
	MaxPullRecords    = 500
	MaxPayloadBytes   = 64 * 1024
	MaxRequestBytes   = 1024 * 1024
)

type Kind string

const (
	KindObservation Kind = "observation"
	KindCorrection  Kind = "correction"
	// KindTask carries one immutable revision of a user-owned task. Tasks are
	// mutable planning items, so each edit appends a new record whose id is
	// "<task_id>_r<revision>"; consumers keep the highest revision per task
	// (ADR-0020). Deleting a task erases all its pushed revisions (ADR-0017).
	KindTask Kind = "task"
	// KindTombstone marks an erased record in the pull stream. Tombstones are
	// minted only by the server's erase endpoint — clients cannot push them —
	// and their metadata-only payload carries the erased id plus its original
	// non-sensitive kind when known.
	KindTombstone Kind = "tombstone"
)

type PushRequest struct {
	SchemaVersion string       `json:"schema_version"`
	Records       []PushRecord `json:"records"`
}

type PushRecord struct {
	RecordID  string          `json:"recordId"`
	Kind      Kind            `json:"kind"`
	CreatedAt time.Time       `json:"createdAt"`
	Payload   json.RawMessage `json:"payload"`
}

type PushResponse struct {
	SchemaVersion string `json:"schema_version"`
	Cursor        int64  `json:"cursor"`
	Accepted      int    `json:"accepted"`
}

type Envelope struct {
	Seq       int64           `json:"seq"`
	RecordID  string          `json:"recordId"`
	Kind      Kind            `json:"kind"`
	DeviceID  string          `json:"deviceId"`
	CreatedAt time.Time       `json:"createdAt"`
	Payload   json.RawMessage `json:"payload"`
}

type PullResponse struct {
	SchemaVersion string     `json:"schema_version"`
	Cursor        int64      `json:"cursor"`
	Records       []Envelope `json:"records"`
}

type TombstonePayload struct {
	RecordID   string `json:"record_id"`
	RecordKind Kind   `json:"record_kind,omitempty"`
}

type EraseRequest struct {
	SchemaVersion string   `json:"schema_version"`
	RecordIDs     []string `json:"record_ids"`
}

type EraseResponse struct {
	SchemaVersion string `json:"schema_version"`
	Erased        int    `json:"erased"`
	Tombstones    int    `json:"tombstones"`
	Cursor        int64  `json:"cursor"`
}
