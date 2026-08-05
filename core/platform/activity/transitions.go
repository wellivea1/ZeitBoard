package activity

import (
	"encoding/json"
	"fmt"
	"time"
)

// State is the closed set of desktop states this collector will ever record.
//
// The set is deliberately small and behavioural. It says whether the machine
// was being used, not what it was used for: there is no field here for an
// application, a window title, a document, or a keystroke, so no amount of
// downstream code can start recording one without changing this type and the
// privacy commitment that constrains it.
type State string

const (
	StateStartup   State = "startup"
	StateActive    State = "active"
	StateIdle      State = "idle"
	StateLocked    State = "locked"
	StateUnlocked  State = "unlocked"
	StateSuspended State = "suspended"
	StateResumed   State = "resumed"
	StateShutdown  State = "shutdown"
)

func (s State) valid() bool {
	switch s {
	case StateStartup, StateActive, StateIdle, StateLocked,
		StateUnlocked, StateSuspended, StateResumed, StateShutdown:
		return true
	default:
		return false
	}
}

// Transition is one recorded change. It carries a time, a state, and how long
// the previous state had lasted — nothing else.
type Transition struct {
	At    time.Time
	State State

	// PriorDuration is how long the machine had been in the previous state.
	// It is what makes a run of transitions usable as sleep evidence: a six
	// hour idle stretch between an idle and an active transition is the
	// signal, not the instants themselves.
	PriorDuration time.Duration
}

// Sample is one observation of machine state, as a platform source reports it.
type Sample struct {
	At time.Time

	// IdleFor is how long since the last user input. Platforms that cannot
	// measure it report zero and set IdleKnown false.
	IdleFor   time.Duration
	IdleKnown bool

	// Locked reports the session lock state when the platform can tell.
	Locked      bool
	LockedKnown bool
}

// Config bounds how eagerly transitions are produced.
type Config struct {
	// IdleThreshold is how long without input before the machine counts as
	// idle. Short values produce noise from ordinary pauses; the default is
	// tuned for "stepped away", not "stopped typing".
	IdleThreshold time.Duration

	// PollInterval is how often the source is sampled. It also sets the
	// resolution of every duration this package reports.
	PollInterval time.Duration

	// SuspendGap is the wall-clock jump between consecutive samples above
	// which the machine is assumed to have been suspended. A gap far larger
	// than the poll interval cannot be anything else.
	SuspendGap time.Duration
}

// DefaultConfig is the shipped tuning.
func DefaultConfig() Config {
	return Config{
		IdleThreshold: 15 * time.Minute,
		PollInterval:  time.Minute,
		SuspendGap:    5 * time.Minute,
	}
}

func (c Config) withDefaults() Config {
	base := DefaultConfig()
	if c.IdleThreshold <= 0 {
		c.IdleThreshold = base.IdleThreshold
	}
	if c.PollInterval <= 0 {
		c.PollInterval = base.PollInterval
	}
	if c.SuspendGap <= 0 {
		c.SuspendGap = base.SuspendGap
	}
	return c
}

// Machine turns a stream of samples into transitions. It is pure and has no
// clock, no I/O, and no platform dependency, so the interesting behaviour —
// debouncing, suspend inference, ordering — is testable without a desktop.
type Machine struct {
	config Config

	started    bool
	state      State
	stateSince time.Time
	lastSample time.Time
}

func NewMachine(config Config) *Machine {
	return &Machine{config: config.withDefaults()}
}

// State reports the current state, and whether any sample has been seen.
func (m *Machine) State() (State, bool) { return m.state, m.started }

// Observe feeds one sample and returns the transitions it caused, in order.
// Most samples produce none; that is the point.
func (m *Machine) Observe(sample Sample) []Transition {
	at := sample.At.UTC()
	if !m.started {
		m.started = true
		m.state = StateStartup
		m.stateSince = at
		m.lastSample = at
		return []Transition{{At: at, State: StateStartup}}
	}

	var out []Transition

	// A wall-clock jump far beyond the poll interval means the process was
	// not running: the machine slept, hibernated, or the host was paused.
	// Recording it as a suspend/resume pair is more honest than pretending
	// the intervening hours were idle, because the user may well have been
	// asleep and the machine was not merely unattended.
	if gap := at.Sub(m.lastSample); gap >= m.config.SuspendGap {
		suspendedAt := m.lastSample
		out = append(out,
			Transition{At: suspendedAt, State: StateSuspended, PriorDuration: suspendedAt.Sub(m.stateSince)},
			Transition{At: at, State: StateResumed, PriorDuration: gap},
		)
		m.state = StateResumed
		m.stateSince = at
	}

	if sample.LockedKnown {
		want := StateUnlocked
		if sample.Locked {
			want = StateLocked
		}
		if m.state != want {
			out = append(out, m.transitionTo(want, at))
		}
	}

	// Lock state takes precedence: a locked machine is not "idle", it is
	// locked, and conflating them would let a deliberate lock look like the
	// same evidence as walking away mid-task.
	if sample.IdleKnown && m.state != StateLocked {
		want := StateActive
		if sample.IdleFor >= m.config.IdleThreshold {
			want = StateIdle
		}
		if m.state != want {
			// Idle began when input stopped, not when the poll noticed. The
			// difference is up to a whole threshold, which matters when the
			// interval is being read as a sleep boundary.
			transitionAt := at
			if want == StateIdle {
				transitionAt = at.Add(-sample.IdleFor)
				if transitionAt.Before(m.stateSince) {
					transitionAt = m.stateSince
				}
			}
			out = append(out, m.transitionTo(want, transitionAt))
			m.stateSince = transitionAt
		}
	}

	m.lastSample = at
	return out
}

// Close records a shutdown transition, so a clean exit is distinguishable from
// the process disappearing.
func (m *Machine) Close(at time.Time) []Transition {
	if !m.started {
		return nil
	}
	at = at.UTC()
	transition := m.transitionTo(StateShutdown, at)
	m.started = false
	return []Transition{transition}
}

func (m *Machine) transitionTo(state State, at time.Time) Transition {
	prior := at.Sub(m.stateSince)
	if prior < 0 {
		prior = 0
	}
	m.state = state
	m.stateSince = at
	return Transition{At: at, State: state, PriorDuration: prior}
}

// payload is the recorded body of a transition observation. Every field is
// behavioural; there is nowhere to put content even by accident.
type payload struct {
	State            string `json:"state"`
	PriorSeconds     int64  `json:"priorSeconds,omitempty"`
	OS               string `json:"os"`
	CollectorVersion string `json:"collectorVersion"`
}

// CollectorVersion identifies the transition semantics, so evidence recorded
// under different rules can be told apart later.
const CollectorVersion = "activity-v2"

func (t Transition) encode(osName string) (json.RawMessage, error) {
	if !t.State.valid() {
		return nil, fmt.Errorf("unsupported activity state %q", string(t.State))
	}
	return json.Marshal(payload{
		State:            string(t.State),
		PriorSeconds:     int64(t.PriorDuration / time.Second),
		OS:               osName,
		CollectorVersion: CollectorVersion,
	})
}
