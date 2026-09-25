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
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(item.Payload, &fields); err != nil {
			return invalid
		}
		var id string
		field := ""
		switch item.Kind {
		case "observation":
			field = "observation_id"
		case "correction":
			field = "correction_id"
		case "task":
			var revision int
			if json.Unmarshal(fields["task_id"], &id) != nil || json.Unmarshal(fields["revision"], &revision) != nil || revision < 1 || item.RecordID != fmt.Sprintf("%s_r%d", id, revision) {
				return invalid
			}
			continue
		case "tombstone":
			field = "record_id"
		default:
			return invalid
		}
		if json.Unmarshal(fields[field], &id) != nil || id != item.RecordID {
			return invalid
		}
	}
	if page.Cursor != last {
		return invalid
	}
	return nil
}
