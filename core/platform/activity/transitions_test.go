package activity

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var base = time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)

func testConfig() Config {
	return Config{IdleThreshold: 15 * time.Minute, PollInterval: time.Minute, SuspendGap: 5 * time.Minute}
}

func states(transitions []Transition) []State {
	out := make([]State, 0, len(transitions))
	for _, t := range transitions {
		out = append(out, t.State)
	}
	return out
}

// poll feeds samples at the poll interval between two instants, the way the
// running collector does. Skipping samples would fabricate a clock gap and
// trip the suspend inference, which is itself the behaviour under test
// elsewhere.
func poll(m *Machine, from, to time.Time, idleAt func(time.Time) time.Duration) []Transition {
	var out []Transition
	for at := from; !at.After(to); at = at.Add(time.Minute) {
		out = append(out, m.Observe(Sample{At: at, IdleFor: idleAt(at), IdleKnown: true})...)
	}
	return out
}

func TestFirstSampleRecordsStartup(t *testing.T) {
	m := NewMachine(testConfig())
	got := m.Observe(Sample{At: base, IdleKnown: true})
	if len(got) != 1 || got[0].State != StateStartup {
		t.Fatalf("first transitions = %v, want a single startup", states(got))
	}
}

// TestSteadyStateProducesNoTransitions is the property that keeps this from
// becoming a high-frequency stream: polling every minute for an hour while
// someone works must record nothing after the initial state.
func TestSteadyStateProducesNoTransitions(t *testing.T) {
	m := NewMachine(testConfig())
	m.Observe(Sample{At: base, IdleFor: 0, IdleKnown: true})
	m.Observe(Sample{At: base.Add(time.Minute), IdleFor: 0, IdleKnown: true})

	for i := 2; i < 60; i++ {
		at := base.Add(time.Duration(i) * time.Minute)
		if got := m.Observe(Sample{At: at, IdleFor: 30 * time.Second, IdleKnown: true}); len(got) != 0 {
			t.Fatalf("minute %d produced %v; steady use must be silent", i, states(got))
		}
	}
}

// TestIdleIsBackdatedToWhenInputStopped matters because the interval is read
// as a sleep boundary: dating it to the poll that noticed would shift every
// boundary by up to a whole threshold.
func TestIdleIsBackdatedToWhenInputStopped(t *testing.T) {
	m := NewMachine(testConfig())
	m.Observe(Sample{At: base, IdleFor: 0, IdleKnown: true})
	m.Observe(Sample{At: base.Add(time.Minute), IdleFor: 0, IdleKnown: true})

	// Input stops at base+1m; every later poll reports a growing idle time.
	stopped := base.Add(time.Minute)
	got := poll(m, base.Add(2*time.Minute), base.Add(20*time.Minute), func(at time.Time) time.Duration {
		return at.Sub(stopped)
	})
	if len(got) != 1 || got[0].State != StateIdle {
		t.Fatalf("transitions = %v, want one idle", states(got))
	}
	if !got[0].At.Equal(stopped) {
		t.Errorf("idle recorded at %v, want %v (when input stopped)", got[0].At, stopped)
	}
}

func TestReturningToUseRecordsActiveWithTheIdleDuration(t *testing.T) {
	m := NewMachine(testConfig())
	m.Observe(Sample{At: base, IdleFor: 0, IdleKnown: true})
	m.Observe(Sample{At: base.Add(time.Minute), IdleFor: 0, IdleKnown: true})
	stopped := base.Add(time.Minute)
	returned := base.Add(8 * time.Hour)
	poll(m, base.Add(2*time.Minute), returned.Add(-time.Minute), func(at time.Time) time.Duration {
		return at.Sub(stopped)
	})

	got := m.Observe(Sample{At: returned, IdleFor: 0, IdleKnown: true})
	if len(got) != 1 || got[0].State != StateActive {
		t.Fatalf("transitions = %v, want one active", states(got))
	}
	// The idle stretch is the evidence. Roughly eight hours from when input
	// stopped to when it resumed.
	if got[0].PriorDuration < 7*time.Hour || got[0].PriorDuration > 9*time.Hour {
		t.Errorf("prior duration = %v, want about 8h of idle", got[0].PriorDuration)
	}
}

// TestClockGapBecomesSuspendResume covers hibernation: the process was not
// running, so calling the gap "idle" would assert something never observed.
func TestClockGapBecomesSuspendResume(t *testing.T) {
	m := NewMachine(testConfig())
	m.Observe(Sample{At: base, IdleFor: 0, IdleKnown: true})

	resumed := base.Add(7 * time.Hour)
	got := m.Observe(Sample{At: resumed, IdleFor: 0, IdleKnown: true})
	if len(got) < 2 {
		t.Fatalf("transitions = %v, want at least suspend and resume", states(got))
	}
	if got[0].State != StateSuspended || got[1].State != StateResumed {
		t.Fatalf("transitions = %v, want suspend then resume", states(got))
	}
	if !got[0].At.Equal(base) {
		t.Errorf("suspend recorded at %v, want the last sample %v", got[0].At, base)
	}
	if got[1].PriorDuration < 6*time.Hour {
		t.Errorf("resume prior duration = %v, want the full gap", got[1].PriorDuration)
	}
}

func TestOrdinaryPollIntervalIsNotASuspend(t *testing.T) {
	m := NewMachine(testConfig())
	m.Observe(Sample{At: base, IdleFor: 0, IdleKnown: true})
	got := m.Observe(Sample{At: base.Add(90 * time.Second), IdleFor: 0, IdleKnown: true})
	for _, transition := range got {
		if transition.State == StateSuspended {
			t.Fatalf("a 90s poll gap was treated as a suspend: %v", states(got))
		}
	}
}

// TestLockTakesPrecedenceOverIdle keeps a deliberate lock from reading as the
// same evidence as walking away mid-task.
func TestLockTakesPrecedenceOverIdle(t *testing.T) {
	m := NewMachine(testConfig())
	m.Observe(Sample{At: base, IdleFor: 0, IdleKnown: true, LockedKnown: true})
	got := m.Observe(Sample{
		At: base.Add(time.Minute), IdleFor: time.Hour, IdleKnown: true,
		Locked: true, LockedKnown: true,
	})
	if len(got) != 1 || got[0].State != StateLocked {
		t.Fatalf("transitions = %v, want only a lock", states(got))
	}
	if state, _ := m.State(); state != StateLocked {
		t.Errorf("state = %q, want locked", state)
	}
}

func TestUnknownSignalsProduceNoClaim(t *testing.T) {
	m := NewMachine(testConfig())
	m.Observe(Sample{At: base})
	// A source that measures nothing must not move the machine off startup.
	got := m.Observe(Sample{At: base.Add(time.Minute)})
	if len(got) != 0 {
		t.Errorf("a sample asserting nothing produced %v", states(got))
	}
}

func TestCloseRecordsShutdownOnce(t *testing.T) {
	m := NewMachine(testConfig())
	m.Observe(Sample{At: base, IdleKnown: true})
	got := m.Close(base.Add(time.Hour))
	if len(got) != 1 || got[0].State != StateShutdown {
		t.Fatalf("close = %v, want one shutdown", states(got))
	}
	if again := m.Close(base.Add(2 * time.Hour)); len(again) != 0 {
		t.Errorf("second close produced %v", states(again))
	}
}

// TestPayloadCarriesNoContent is the privacy assertion. The encoded body must
// contain only behavioural fields; there is no place for a name or a title,
// and this test fails if one is ever added.
func TestPayloadCarriesNoContent(t *testing.T) {
	transition := Transition{At: base, State: StateIdle, PriorDuration: 3 * time.Hour}
	body, err := transition.encode("windows")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	allowed := map[string]bool{"state": true, "priorSeconds": true, "os": true, "collectorVersion": true}
	for key := range decoded {
		if !allowed[key] {
			t.Errorf("activity payload carries non-allowlisted key %q", key)
		}
	}
	// Key names are the real assertion; a substring sweep over the whole body
	// would match "window" inside the legitimate "windows" OS value.
	for key := range decoded {
		lowered := strings.ToLower(key)
		for _, forbidden := range []string{"title", "window", "app", "process", "url", "file", "key", "text", "name"} {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("activity payload key %q suggests content capture", key)
			}
		}
	}
}

func TestEncodeRejectsAnUnknownState(t *testing.T) {
	if _, err := (Transition{At: base, State: State("watching-netflix")}).encode("windows"); err == nil {
		t.Fatal("an unknown state was encoded")
	}
}
