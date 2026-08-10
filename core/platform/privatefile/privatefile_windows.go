//go:build windows

package privatefile

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func currentUserSID() (*windows.SID, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("read current user token: %w", err)
	}
	return user.User.Sid, nil
}

// apply replaces the object's DACL with a single access-allowed entry for the
// current user and disables inheritance.
func apply(path string, inheritance windows.ACCESS_MODE, flags uint32) error {
	sid, err := currentUserSID()
	if err != nil {
		return err
	}
	access := []windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        inheritance,
		Inheritance:       flags,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}}
	acl, err := windows.ACLFromEntries(access, nil)
	if err != nil {
		return fmt.Errorf("build ACL for %s: %w", path, err)
	}
	// PROTECTED_DACL_SECURITY_INFORMATION drops the inherited entries, so a
	// permissive ACL on the parent cannot widen access to this file.
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, acl, nil,
	); err != nil {
		return fmt.Errorf("apply ACL to %s: %w", path, err)
	}
	return nil
}

func restrict(path string) error {
	return apply(path, windows.GRANT_ACCESS, windows.NO_INHERITANCE)
}

func restrictDir(path string) error {
	// Children inherit the same single grant, so a file written by code that
	// does not call Restrict is still owner-only.
	return apply(path, windows.GRANT_ACCESS,
		windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT)
}

func describe(path string) (Access, error) {
	sd, err := windows.GetNamedSecurityInfo(
		path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return Access{}, fmt.Errorf("read security info for %s: %w", path, err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return Access{}, fmt.Errorf("read DACL for %s: %w", path, err)
	}
	if dacl == nil {
		// A NULL DACL grants everyone everything. Reporting it as such is the
		// whole point of reading permissions back.
		return Access{Enforced: true, Detail: "no DACL: access is unrestricted"}, nil
	}
	control, _, err := sd.Control()
	if err != nil {
		return Access{}, fmt.Errorf("read control flags for %s: %w", path, err)
	}
	sid, err := currentUserSID()
	if err != nil {
		return Access{}, err
	}

	result := Access{
		Enforced:  true,
		Inherited: control&windows.SE_DACL_PROTECTED == 0,
		OwnerOnly: true,
	}
	trustees := make([]string, 0, dacl.AceCount)
	for index := uint16(0); index < dacl.AceCount; index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(index), &ace); err != nil {
			return Access{}, fmt.Errorf("read ACE %d of %s: %w", index, path, err)
		}
		entry := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		trustees = append(trustees, entry.String())
		if !entry.Equals(sid) {
			result.OwnerOnly = false
		}
	}
	result.Detail = fmt.Sprintf("entries=%v protected=%v", trustees, !result.Inherited)
	return result, nil
}
