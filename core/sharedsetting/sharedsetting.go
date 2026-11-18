// Package sharedsetting defines the owner's choices that travel between their
// devices through their own server (ADR-0074): whether a status they report
// may show on share links by default, whether the sleep buttons also report a
// status, and the clock times are written on.
//
// A choice is kept as records. Each change the owner makes, on any device, is
// one immutable record naming the choice, its value and when it was made. For
// each choice the record made latest stands, on every device; two made in the
// same instant are told apart by their ids, so every device settles on the
// same one. A device keeps its own copy of each choice, takes a newer one when
// it arrives, and says when it made one of its own.
//
// It is defined and validated once, here, for the server's sync validation and
// the desktop's store; the phone mirrors it.
package sharedsetting

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// The choices that travel. Nothing else leaves a device: a choice that only
// matters on one device stays there.
const (
	// ShareStatusByDefault is whether a status the owner reports may show on
	// the share links they allowed to show one (ADR-0070).
	ShareStatusByDefault = "share_status_by_default"

	// FollowSleepTaps is whether "I am going to sleep" and "I woke up" also
	// report a status.
	FollowSleepTaps = "follow_sleep_taps"

	// Clock is the clock times are written on.
	Clock = "clock"
)

// The clock's values: each device's own format, or 12- or 24-hour everywhere.
const (
	ClockSystem = "system"
	Clock12     = "12"
	Clock24     = "24"
)

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,63}$`)

// Record is one choice the owner made on one device.
type Record struct {
	SettingID string    `json:"setting_id"`
	Name      string    `json:"name"`
	Value     Value     `json:"value"`
	ChangedAt time.Time `json:"changed_at"`
}

// Value is what a choice was set to: on or off, or for the clock one of its
// words.
type Value struct {
	on   *bool
	word string
}

// On is a choice turned on or off.
func On(on bool) Value { return Value{on: &on} }

// Word is a choice set to a word.
func Word(word string) Value { return Value{word: word} }

// Bool is the value of a choice that is on or off, and whether it is one.
func (v Value) Bool() (bool, bool) {
	if v.on == nil {
		return false, false
	}
	return *v.on, true
}

// Text is the value of a choice set to a word, and whether it is one.
func (v Value) Text() (string, bool) {
	return v.word, v.on == nil && v.word != ""
}

func (v Value) MarshalJSON() ([]byte, error) {
	if v.on != nil {
		return json.Marshal(*v.on)
	}
	if v.word == "" {
		return nil, errors.New("a setting has no value")
	}
	return json.Marshal(v.word)
}

func (v *Value) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var word string
		if err := json.Unmarshal(trimmed, &word); err != nil {
			return err
		}
		*v = Word(word)
		return nil
	}
	var on bool
	if err := json.Unmarshal(trimmed, &on); err != nil {
		return errors.New("a setting is on or off, or a word")
	}
	*v = On(on)
	return nil
}

// Equal reports whether two values are the same.
func (v Value) Equal(other Value) bool {
	a, aIsBool := v.Bool()
	b, bIsBool := other.Bool()
	if aIsBool || bIsBool {
		return aIsBool && bIsBool && a == b
	}
	return v.word == other.word
}

// Validate checks a record: a known choice, a value it can take, an id, and
// when it was made.
func (r Record) Validate() error {
	if !identifier.MatchString(r.SettingID) {
		return errors.New("setting_id must match the record identifier rule")
	}
	if r.ChangedAt.IsZero() {
		return errors.New("changed_at is required")
	}
	switch r.Name {
	case ShareStatusByDefault, FollowSleepTaps:
		if _, ok := r.Value.Bool(); !ok {
			return fmt.Errorf("%s is on or off", r.Name)
		}
	case Clock:
		if !ValidClock(r.Value.word) || r.Value.on != nil {
			return errors.New("the clock is system, 12 or 24")
		}
	default:
		return fmt.Errorf("%q is not a setting that travels between devices", r.Name)
	}
	return nil
}

// ValidClock reports whether value is one the clock can be set to.
func ValidClock(value string) bool {
	switch value {
	case ClockSystem, Clock12, Clock24:
		return true
	default:
		return false
	}
}

// Stands reports whether a stands over b, two records of one choice: the one
// made later, and of two made in the same instant the greater id.
func Stands(a, b Record) bool {
	if !a.ChangedAt.Equal(b.ChangedAt) {
		return a.ChangedAt.After(b.ChangedAt)
	}
	return a.SettingID > b.SettingID
}

// Current returns the record that stands for each choice with any record.
func Current(records []Record) map[string]Record {
	current := make(map[string]Record, 3)
	for _, record := range records {
		if standing, ok := current[record.Name]; !ok || Stands(record, standing) {
			current[record.Name] = record
		}
	}
	return current
}

// NextChangeAt is when a choice made now is recorded: now, or just after the
// one that stands when this device's clock is behind it, so the new choice is
// the newest wherever it arrives.
func NextChangeAt(now time.Time, standing *Record) time.Time {
	at := now.UTC().Truncate(time.Millisecond)
	if standing != nil && !at.After(standing.ChangedAt) {
		at = standing.ChangedAt.UTC().Truncate(time.Millisecond).Add(time.Millisecond)
	}
	return at
}
