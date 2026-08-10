// Package privatefile restricts local files to the user running the process.
//
// It exists because `os.Chmod(0o600)` does not mean on Windows what its Unix
// spelling suggests: it toggles the read-only attribute and leaves the DACL
// inherited from the parent directory untouched. Every file this project calls
// private was created that way — the database holding every recorded sleep, the
// bearer token for the user's own server, the settings files, the exports — and
// on Windows the mode argument protected none of them.
//
// The rule this package encodes: a restrictive-permissions claim is enforced
// with a real DACL, or it is not made. It is the same rule ADR-0028 applied to
// the local agent's descriptor, applied to everything else that was left out.
//
// What this is not: encryption. An owner-only DACL stops another account on the
// same machine and a careless backup of the profile directory. It does not stop
// anyone who can read the disk offline, and it does not stop code running as
// this user. docs/privacy.md says so in those words.
package privatefile

import "errors"

// ErrUnsupported reports that this build cannot enforce ownership beyond the
// mode bits already set by the caller.
var ErrUnsupported = errors.New("privatefile: this platform relies on file mode")

// Restrict limits path to the current user, dropping inherited access.
//
// It is idempotent: applying it to a file that already carries the restriction
// succeeds and changes nothing.
func Restrict(path string) error { return restrict(path) }

// RestrictDir limits a directory to the current user and arranges for files
// created inside it to inherit that restriction, so a file written by code that
// has not been taught about this package is still not world-readable.
func RestrictDir(path string) error { return restrictDir(path) }

// RestrictExisting applies Restrict to each path that exists, ignoring the ones
// that do not. SQLite creates its write-ahead log and shared-memory file lazily
// and deletes them on a clean close, so the set of files carrying database
// content changes over the life of a connection.
func RestrictExisting(paths ...string) error {
	var firstErr error
	for _, path := range paths {
		if !exists(path) {
			continue
		}
		if err := Restrict(path); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Access describes what a file's permissions actually are, read back from the
// operating system rather than assumed from what was requested. Tests assert
// against this: a permission claim nobody reads back is a comment.
type Access struct {
	// OwnerOnly is true when no account other than the file's owner is granted
	// access.
	OwnerOnly bool

	// Inherited is true when the parent's permissions still apply, which means
	// a permissive parent can widen this file.
	Inherited bool

	// Enforced is false on platforms where this package cannot do better than
	// the mode bits. OwnerOnly then reflects the mode.
	Enforced bool

	// Detail is a human-readable rendering for test failure messages.
	Detail string
}

// Describe reads back the effective permissions of path.
func Describe(path string) (Access, error) { return describe(path) }
