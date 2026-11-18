package sharedsetting

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var at = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

func TestARecordCarriesAValueItsChoiceCanTake(t *testing.T) {
	for _, record := range []Record{
		{SettingID: "setting-share", Name: ShareStatusByDefault, Value: On(true), ChangedAt: at},
		{SettingID: "setting-follow", Name: FollowSleepTaps, Value: On(false), ChangedAt: at},
		{SettingID: "setting-clock", Name: Clock, Value: Word(Clock24), ChangedAt: at},
		{SettingID: "setting-system", Name: Clock, Value: Word(ClockSystem), ChangedAt: at},
	} {
		if err := record.Validate(); err != nil {
			t.Errorf("%s refused: %v", record.Name, err)
		}
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		var back Record
		if err := json.Unmarshal(encoded, &back); err != nil {
			t.Fatal(err)
		}
		if !back.Value.Equal(record.Value) || back.Name != record.Name || !back.ChangedAt.Equal(record.ChangedAt) {
			t.Errorf("%s did not survive the wire: %s", record.Name, encoded)
		}
	}
	for name, record := range map[string]Record{
		"a word for a switch":      {SettingID: "setting-a", Name: ShareStatusByDefault, Value: Word("on"), ChangedAt: at},
		"a switch for the clock":   {SettingID: "setting-b", Name: Clock, Value: On(true), ChangedAt: at},
		"a clock nobody has":       {SettingID: "setting-c", Name: Clock, Value: Word("36"), ChangedAt: at},
		"a choice that stays home": {SettingID: "setting-d", Name: "theme", Value: Word("dark"), ChangedAt: at},
		"no time":                  {SettingID: "setting-e", Name: Clock, Value: Word(Clock12)},
		"a bad id":                 {SettingID: "Setting E", Name: Clock, Value: Word(Clock12), ChangedAt: at},
	} {
		if err := record.Validate(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	var value Value
	if err := json.Unmarshal([]byte(`3`), &value); err == nil || !strings.Contains(err.Error(), "on or off") {
		t.Errorf("a number read as a setting: %v", err)
	}
}

// The newest choice stands on every device, and two made in one instant
// settle the same way everywhere.
func TestTheNewestChoiceStands(t *testing.T) {
	older := Record{SettingID: "setting-z", Name: Clock, Value: Word(Clock12), ChangedAt: at}
	newer := Record{SettingID: "setting-a", Name: Clock, Value: Word(Clock24), ChangedAt: at.Add(time.Second)}
	twin := Record{SettingID: "setting-b", Name: Clock, Value: Word(ClockSystem), ChangedAt: at.Add(time.Second)}
	share := Record{SettingID: "setting-s", Name: ShareStatusByDefault, Value: On(true), ChangedAt: at}

	current := Current([]Record{newer, share, older})
	if got := current[Clock]; got.SettingID != newer.SettingID {
		t.Fatalf("the clock that stands is %s", got.SettingID)
	}
	if got := current[ShareStatusByDefault]; got.SettingID != share.SettingID {
		t.Fatalf("sharing that stands is %s", got.SettingID)
	}
	for _, order := range [][]Record{{newer, twin}, {twin, newer}} {
		if got := Current(order)[Clock]; got.SettingID != twin.SettingID {
			t.Fatalf("two made in one instant settled on %s", got.SettingID)
		}
	}
	if _, ok := Current(nil)[FollowSleepTaps]; ok {
		t.Fatal("a choice nobody made stands")
	}
}

// A choice made on a device whose clock is behind still lands after the one
// that stands.
func TestANewChoiceIsMadeAfterTheOneThatStands(t *testing.T) {
	standing := Record{SettingID: "setting-a", Name: Clock, Value: Word(Clock24), ChangedAt: at.Add(500 * time.Microsecond)}
	if got := NextChangeAt(at.Add(-time.Hour), &standing); !got.After(standing.ChangedAt) {
		t.Fatalf("made at %s, not after %s", got, standing.ChangedAt)
	}
	if got := NextChangeAt(at.Add(time.Hour), &standing); !got.Equal(at.Add(time.Hour)) {
		t.Fatalf("a clock ahead of the standing choice moved to %s", got)
	}
	if got := NextChangeAt(at.Add(1234567), nil); !got.Equal(at.Add(time.Millisecond)) {
		t.Fatalf("made at %s, want the millisecond", got)
	}
}
