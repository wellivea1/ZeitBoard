package regional

import "testing"

func TestATimePatternSaysWhichClock(t *testing.T) {
	for pattern, want := range map[string]bool{
		"h:mm tt":      false, // English (United States)
		"HH:mm":        true,  // English (United Kingdom), German, French
		"H:mm":         true,  // a single-digit hour on the 24-hour clock
		"tt h:mm":      false, // Korean
		"'Hour' h:mm":  false, // a quoted H is literal text
		"h 'h' mm":     false,
		"H 'h' mm":     true, // French (Canada)
		"":             false,
		"'unclosed H":  false,
		"h:mm tt 'H'H": true,
	} {
		if got := twentyFourHourPattern(pattern); got != want {
			t.Errorf("%q reads as 24-hour %v, want %v", pattern, got, want)
		}
	}
}
