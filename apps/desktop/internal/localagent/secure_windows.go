//go:build windows

package localagent

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// restrictToCurrentUser replaces the file's DACL with a single access-allowed
// entry for the current user and disables inheritance.
//
// This exists because os.Chmod(0600) does not do what its Unix spelling
// suggests on Windows: it only toggles the read-only attribute and leaves the
// inherited ACL untouched. The descriptor carries the local agent bearer
// token, so the restrictive-permissions claim has to be enforced with a real
// DACL or not claimed at all.
func restrictToCurrentUser(path string) error {
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil {
		return fmt.Errorf("read current user token: %w", err)
	}

	// One explicit grant: this user, full control, no inheritance from parent.
	access := []windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       windows.NO_INHERITANCE,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid),
		},
	}}

	acl, err := windows.ACLFromEntries(access, nil)
	if err != nil {
		return fmt.Errorf("build descriptor ACL: %w", err)
	}

	// PROTECTED_DACL_SECURITY_INFORMATION drops inherited entries, so a
	// permissive ACL on the parent directory cannot widen access to the token.
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, acl, nil,
	); err != nil {
		return fmt.Errorf("apply descriptor ACL: %w", err)
	}
	return nil
}
