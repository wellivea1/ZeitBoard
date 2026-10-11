//go:build !windows

package regional

// Away from Windows there is no regional format to read, so the clock stays
// the one ZeitBoard has always written until the person chooses another.
func platformTwentyFourHour() bool { return false }
