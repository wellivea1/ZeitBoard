//go:build !windows

package localagent

// restrictToCurrentUser is a no-op off Windows: the 0600 mode set when the
// descriptor is staged already restricts it to the owning user there.
func restrictToCurrentUser(string) error { return nil }
