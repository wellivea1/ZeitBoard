//go:build windows

package regional

import "golang.org/x/sys/windows/registry"

// The short time format is the one the taskbar clock uses, and the one the
// person sets under Region › Regional format.
const internationalKey = `Control Panel\International`

func platformTwentyFourHour() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, internationalKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	pattern, _, err := key.GetStringValue("sShortTime")
	if err != nil {
		return false
	}
	return twentyFourHourPattern(pattern)
}
