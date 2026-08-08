package recompute_test

import (
	"testing"
	"time"

	"non24.app/core/domain"
	"non24.app/core/recompute"
)

var base = time.Date(2026, 8, 6, 3, 0, 0, 0, time.UTC)

func session(id string, start, end time.Time, recorded time.Time) domain.SleepSession {
	return domain.SleepSession{
		ID:             domain.SleepSessionID(id),
		Classification: domain.SleepClassificationPrincipal,
		CreatedAt:      recorded,
		Intervals: []domain.SleepInterval{{
			Interval: domain.TimeRange{
				Start: domain.ZonedInstant{UTC: start, ZoneID: "UTC"},
				End:   domain.ZonedInstant{UTC: end, ZoneID: "UTC"},
			},
			StartEvidence: domain.Evidence{Status: domain.StatusObserved, RecordedAt: recorded},
			EndEvidence:   domain.Evidence{Status: domain.StatusObserved, RecordedAt: recorded},
		}},
	}
}

// TestFingerprintIgnoresOrder keeps the digest a property of the history rather
// than of the query that returned it. A read model is free to change its
// ordering; that must not read as new evidence.
func TestFingerprintIgnoresOrder(t *testing.T) {
	a := session("s1", base, base.Add(8*time.Hour), base)
	b := session("s2", base.Add(24*time.Hour), base.Add(32*time.Hour), base.Add(32*time.Hour))

	forward := recompute.Inputs{Sleep: []domain.SleepSession{a, b}}.Fingerprint()
	backward := recompute.Inputs{Sleep: []domain.SleepSession{b, a}}.Fingerprint()
	if forward != backward {
		t.Errorf("reordering the same history changed the fingerprint:\n%s\n%s", forward, backward)
	}
}

// TestFingerprintDoesNotMoveOnItsOwn is the property the whole mechanism rests
// on. A fingerprint that drifted with the clock would report a change on every
// run, and the orchestrator would restamp an unchanged projection forever.
func TestFingerprintDoesNotMoveOnItsOwn(t *testing.T) {
	inputs := recompute.Inputs{
		Sleep:     []domain.SleepSession{session("s1", base, base.Add(8*time.Hour), base)},
		Consumers: []string{"profile-a"},
	}
	first := inputs.Fingerprint()
	time.Sleep(2 * time.Millisecond)
	if second := inputs.Fingerprint(); first != second {
		t.Errorf("the fingerprint of unchanged inputs moved: %s then %s", first, second)
	}
}

// TestRecordedAtIsPartOfTheFingerprint matters because the freshness policy
// reads exactly that field. The same night re-recorded later is genuinely
// different input: it makes a withheld claim sayable again.
func TestRecordedAtIsPartOfTheFingerprint(t *testing.T) {
	night := recompute.Inputs{Sleep: []domain.SleepSession{
		session("s1", base, base.Add(8*time.Hour), base.Add(8*time.Hour)),
	}}
	reRecorded := recompute.Inputs{Sleep: []domain.SleepSession{
		session("s1", base, base.Add(8*time.Hour), base.Add(30*time.Hour)),
	}}

	if night.Fingerprint() == reRecorded.Fingerprint() {
		t.Error("re-recording the same night did not change the fingerprint")
	}
}

// TestConsumersAreInTheFingerprint: a new share link needs a row written even
// though the rhythm has not moved at all.
func TestConsumersAreInTheFingerprint(t *testing.T) {
	sleep := []domain.SleepSession{session("s1", base, base.Add(8*time.Hour), base)}
	one := (recompute.Inputs{Sleep: sleep, Consumers: []string{"profile-a"}}).Fingerprint()
	two := (recompute.Inputs{Sleep: sleep, Consumers: []string{"profile-a", "profile-b"}}).Fingerprint()
	if one == two {
		t.Error("adding a consumer did not change the fingerprint")
	}
}

// TestAdjacentValuesCannotCollide is the reason entries are length-prefixed
// before hashing. Without it, ["ab","c"] and ["a","bc"] hash identically, and
// two different histories would look like one.
func TestAdjacentValuesCannotCollide(t *testing.T) {
	if recompute.Digest([]string{"ab", "c"}) == recompute.Digest([]string{"a", "bc"}) {
		t.Error("entries ran into each other across the separator")
	}
}

func TestSuppressionChangesTheFingerprint(t *testing.T) {
	night := session("s1", base, base.Add(8*time.Hour), base)
	suppressed := night
	suppressed.Suppressed = true

	kept := (recompute.Inputs{Sleep: []domain.SleepSession{night}}).Fingerprint()
	hidden := (recompute.Inputs{Sleep: []domain.SleepSession{suppressed}}).Fingerprint()
	if kept == hidden {
		t.Error("suppressing a session did not change the fingerprint")
	}
}
