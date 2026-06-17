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
