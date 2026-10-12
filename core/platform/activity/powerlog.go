package activity

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
	"time"

	"non24.app/core/domain"
)

// The computer's own record of its power (ADR-0060). Windows writes an entry
// to its System log when the computer goes to sleep, wakes, shuts down and
// starts, and keeps months of them. ZeitBoard asks for six entries by provider
// and number, and keeps the time and record number of each. Nothing else in
// the log is asked for, and nothing else in an entry is read.

// PowerLogVersion tags the transitions read from the log, so they can be told
// from the ones the collector saw.
const PowerLogVersion = "windows-power-log-v1"

// PowerLogSourceID names the separate consent to read the log.
const PowerLogSourceID domain.DataSourceID = "windows-power-log"

// PowerEvent is one power entry from the operating system's log.
type PowerEvent struct {
	At       time.Time
	State    State
	RecordID uint64
}

// PowerLog reads the operating system's record of its power.
type PowerLog interface {
	// After returns the entries written after since, oldest first, and at
	// most limit of them.
	After(since time.Time, limit int) ([]PowerEvent, error)
}

// DefaultPowerLog is this build's reader, or nil where there is none.
func DefaultPowerLog() PowerLog { return platformPowerLog() }

// powerEntries are the entries read: who writes them, their number, and the
// transition each one is.
var powerEntries = []struct {
	provider string
	id       uint32
	state    State
}{
	{"Microsoft-Windows-Kernel-Power", 42, StateSuspended},  // The system is entering sleep.
	{"Microsoft-Windows-Kernel-Power", 506, StateSuspended}, // Entering Modern Standby.
	{"Microsoft-Windows-Kernel-Power", 107, StateResumed},   // The system has resumed from sleep.
	{"Microsoft-Windows-Kernel-Power", 507, StateResumed},   // Leaving Modern Standby.
	{"Microsoft-Windows-Kernel-General", 13, StateShutdown}, // The operating system is shutting down.
	{"Microsoft-Windows-Kernel-General", 12, StateStartup},  // The operating system started.
}

// powerLogQuery asks the System log for those entries written after since.
func powerLogQuery(since time.Time) string {
	var providers []string
	numbers := map[string][]string{}
	for _, entry := range powerEntries {
		if _, seen := numbers[entry.provider]; !seen {
			providers = append(providers, entry.provider)
		}
		numbers[entry.provider] = append(numbers[entry.provider], fmt.Sprintf("EventID=%d", entry.id))
	}
	clauses := make([]string, len(providers))
	for i, provider := range providers {
		clauses[i] = fmt.Sprintf("(Provider[@Name='%s'] and (%s))", provider, strings.Join(numbers[provider], " or "))
	}
	return fmt.Sprintf("*[System[(%s) and TimeCreated[@SystemTime>'%s']]]",
		strings.Join(clauses, " or "), since.UTC().Format("2006-01-02T15:04:05.000Z"))
}

// renderedEntry is all of an entry's rendered XML that is read. Its data is
// not mapped, so the decoder passes over it.
type renderedEntry struct {
	System struct {
		Provider struct {
			Name string `xml:"Name,attr"`
		} `xml:"Provider"`
		EventID     uint32 `xml:"EventID"`
		TimeCreated struct {
			SystemTime string `xml:"SystemTime,attr"`
		} `xml:"TimeCreated"`
		EventRecordID uint64 `xml:"EventRecordID"`
	} `xml:"System"`
}

// parsePowerEntry reads one rendered entry, and reports false for anything
// that is not one of the six.
func parsePowerEntry(rendered []byte) (PowerEvent, bool) {
	var entry renderedEntry
	if err := xml.Unmarshal(rendered, &entry); err != nil {
		return PowerEvent{}, false
	}
	at, err := time.Parse(time.RFC3339Nano, entry.System.TimeCreated.SystemTime)
	if err != nil {
		return PowerEvent{}, false
	}
	for _, known := range powerEntries {
		if known.provider == entry.System.Provider.Name && known.id == entry.System.EventID {
			return PowerEvent{At: at.UTC(), State: known.state, RecordID: entry.System.EventRecordID}, true
		}
	}
	return PowerEvent{}, false
}

// PowerObservations turns entries read from the log into activity
// observations. Each is identified by its record and time, so reading the
// same entry again keeps nothing twice.
func PowerObservations(events []PowerEvent, zoneID string, recordedAt time.Time) ([]domain.SourceObservation, error) {
	observations := make([]domain.SourceObservation, 0, len(events))
	for _, event := range events {
		body, err := Transition{At: event.At, State: event.State}.encodeAs("windows", PowerLogVersion)
		if err != nil {
			return nil, err
		}
		instant, err := domain.NewZonedInstant(event.At.UTC(), zoneID)
		if err != nil {
			return nil, err
		}
		key := fmt.Sprintf("%d-%d", event.RecordID, event.At.UnixNano())
		observations = append(observations, domain.SourceObservation{
			ID:         domain.ObservationID("power-log-" + key),
			SourceID:   SourceID,
			ExternalID: "windows-power-log:" + key,
			Kind:       "activity",
			ObservedAt: instant,
			RecordedAt: recordedAt.UTC(),
			Evidence: domain.Evidence{
				Acquisition: domain.AcquisitionCollected,
				Status:      domain.StatusObserved,
				Algorithm:   PowerLogVersion,
				SourceIDs:   []domain.DataSourceID{SourceID},
				RecordedAt:  recordedAt.UTC(),
			},
			Payload: body,
		})
	}
	return observations, nil
}

// Transitions reads activity observations back as transitions, in time
// order. One it cannot read is left out rather than guessed at.
func Transitions(observations []domain.SourceObservation) []Transition {
	transitions := make([]Transition, 0, len(observations))
	for _, observation := range observations {
		var body payload
		if err := json.Unmarshal(observation.Payload, &body); err != nil || !State(body.State).valid() {
			continue
		}
		transitions = append(transitions, Transition{
			At:            observation.ObservedAt.UTC.UTC(),
			State:         State(body.State),
			PriorDuration: time.Duration(body.PriorSeconds) * time.Second,
			FromPowerLog:  observation.Evidence.Algorithm == PowerLogVersion,
		})
	}
	sort.SliceStable(transitions, func(i, j int) bool { return transitions[i].At.Before(transitions[j].At) })
	return transitions
}
