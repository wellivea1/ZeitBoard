// Package regional reads the regional formats the person chose for their
// computer (ADR-0057). It reads only; ZeitBoard never changes them.
package regional

// TwentyFourHour reports whether the computer writes the time of day on a
// twenty-four-hour clock. Where it cannot tell, it reports false, the clock
// ZeitBoard wrote before it asked.
func TwentyFourHour() bool { return platformTwentyFourHour() }

// twentyFourHourPattern reads a Windows time pattern, such as "h:mm tt" or
// "HH:mm": a capital H is an hour on the twenty-four-hour clock.
func twentyFourHourPattern(pattern string) bool {
	// Literal text in a pattern is quoted, and a quoted H is not an hour.
	quoted := false
	for _, character := range pattern {
		switch {
		case character == '\'':
			quoted = !quoted
		case !quoted && character == 'H':
			return true
		}
	}
	return false
}
