//go:build !windows

package activity

// Other systems keep no log ZeitBoard reads yet.
func platformPowerLog() PowerLog { return nil }
