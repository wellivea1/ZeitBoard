//go:build !windows

package privatefile

import (
	"fmt"
	"os"
)

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// restrict narrows the mode to the owner. Unlike Windows, the mode is the
// access control here, so this is the whole mechanism rather than a hint.
func restrict(path string) error {
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("restrict %s: %w", path, err)
	}
	return nil
}

func restrictDir(path string) error {
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("restrict directory %s: %w", path, err)
	}
	return nil
}

func describe(path string) (Access, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Access{}, err
	}
	mode := info.Mode().Perm()
	// Directories need the execute bit to be usable at all, so the owner-only
	// question is whether anyone else has any bit set.
	return Access{
		Enforced:  true,
		OwnerOnly: mode&0o077 == 0,
		Inherited: false,
		Detail:    fmt.Sprintf("mode=%04o", mode),
	}, nil
}
