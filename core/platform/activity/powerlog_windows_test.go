//go:build windows

package activity

import (
	"testing"
	"time"
)

// Windows accepts the query and answers in order with the six entries alone.
// Nothing read is printed.
func TestTheSystemLogAnswersTheQuery(t *testing.T) {
	events, err := windowsPowerLog{}.After(time.Now().AddDate(0, 0, -90), 500)
	if err != nil {
		t.Fatal(err)
	}
	for i, event := range events {
		switch event.State {
		case StateSuspended, StateResumed, StateShutdown, StateStartup:
		default:
			t.Fatalf("entry %d read as %q", i, event.State)
		}
		if i > 0 && event.At.Before(events[i-1].At) {
			t.Fatalf("entry %d is out of order", i)
		}
	}
	if limited, err := (windowsPowerLog{}).After(time.Time{}, 1); err != nil || len(limited) > 1 {
		t.Fatalf("a limit of one gave %d entries, %v", len(limited), err)
	}
}
