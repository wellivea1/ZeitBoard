package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	storage "non24.app/core/storage/sqlite"
)

func (p *syncPullResponse) UnmarshalJSON(data []byte) error {
	type response syncPullResponse
	var value response
	var required map[string]json.RawMessage
	if err := json.Unmarshal(data, &required); err != nil {
		return err
	}
	if len(required["cursor"]) == 0 || string(required["cursor"]) == "null" || len(required["records"]) == 0 || string(required["records"]) == "null" {
		return errors.New("incomplete sync pull response")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	*p = syncPullResponse(value)
	return nil
}

func (p *syncEraseResponse) UnmarshalJSON(data []byte) error {
	type response syncEraseResponse
	var value response
	var required map[string]json.RawMessage
	if err := json.Unmarshal(data, &required); err != nil {
		return err
	}
	for _, field := range []string{"cursor", "erased", "tombstones"} {
		if len(required[field]) == 0 || string(required[field]) == "null" {
			return errors.New("incomplete sync erasure response")
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	*p = syncEraseResponse(value)
	return nil
}

// Validate the complete envelope before advancing a cursor, including records
// originally written by this device. Restores must be able to recover those too.
func validatePullEnvelopePage(page syncPullResponse, since int64) error {
	invalid := errors.New("server returned an invalid sync page; no records were applied")
	if page.Cursor < since || len(page.Records) > storage.MaxSyncPullPageSize {
		return invalid
	}
	last := since
	seen := map[string]bool{}
	for _, item := range page.Records {
		if item.Seq <= last || item.Seq > page.Cursor || item.RecordID == "" || item.DeviceID == "" || item.CreatedAt.IsZero() || seen[item.RecordID] {
			return invalid
		}
		last = item.Seq
		seen[item.RecordID] = true
		if !envelopeNamesItself(item) {
			return invalid
		}
	}
	if page.Cursor != last {
		return invalid
	}
	return nil
}

// envelopeIDFields name, for each kind, the payload field that holds its
// record's id.
var envelopeIDFields = map[string]string{
	"observation":                        "observation_id",
	"correction":                         "correction_id",
	"placement":                          "placement_id",
	storage.SyncKindMedicationEvent:      "event_id",
	storage.SyncKindMedicationCorrection: "correction_id",
	storage.SyncKindContextMarker:        "marker_id",
	storage.SyncKindSourceReport:         "report_id",
	"tombstone":                          "record_id",
}

// envelopeNamesItself reports whether a record is of a kind ZeitBoard knows,
// and its payload names the record's own id. A revision's record id is its
// owner's id plus _r<revision>.
func envelopeNamesItself(item syncEnvelope) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(item.Payload, &fields); err != nil {
		return false
	}
	var id string
	switch item.Kind {
	case "task", storage.SyncKindMedication:
		owner := "task_id"
		if item.Kind == storage.SyncKindMedication {
			owner = "medication_id"
		}
		var revision int
		return json.Unmarshal(fields[owner], &id) == nil && json.Unmarshal(fields["revision"], &revision) == nil &&
			revision >= 1 && item.RecordID == fmt.Sprintf("%s_r%d", id, revision)
	}
	field, known := envelopeIDFields[item.Kind]
	return known && json.Unmarshal(fields[field], &id) == nil && id == item.RecordID
}
