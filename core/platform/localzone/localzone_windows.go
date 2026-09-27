//go:build windows

package localzone

import (
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"non24.app/core/timezones"
)

var (
	icu                        = windows.NewLazySystemDLL("icu.dll")
	procUcalGetDefaultTimeZone = icu.NewProc("ucal_getDefaultTimeZone")
)

// platformID asks the ICU that ships with Windows 10 1903 and later, which
// maps the system zone to its IANA id. Older systems name their zone in the
// registry, which the Windows table maps for the common zones.
func platformID() string {
	if id := icuDefaultZone(); id != "" {
		return id
	}
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\TimeZoneInformation`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()
	name, _, err := key.GetStringValue("TimeZoneKeyName")
	if err != nil {
		return ""
	}
	id, _ := timezones.FromWindows(name)
	return id
}

// icuDefaultZone calls int32_t ucal_getDefaultTimeZone(UChar *result,
// int32_t capacity, UErrorCode *status); a positive status is a failure.
func icuDefaultZone() string {
	if procUcalGetDefaultTimeZone.Find() != nil {
		return ""
	}
	buffer := make([]uint16, 128)
	var status int32
	length, _, _ := procUcalGetDefaultTimeZone.Call(
		uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), uintptr(unsafe.Pointer(&status)),
	)
	size := int(int32(length))
	if status > 0 || size <= 0 || size > len(buffer) {
		return ""
	}
	return windows.UTF16ToString(buffer[:size])
}
