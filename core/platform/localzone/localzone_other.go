//go:build !windows

package localzone

import (
	"os"
	"strings"
	"time"
)

// platformID takes TZ when it named the zone Go loaded, and otherwise the
// zone /etc/localtime links to.
func platformID() string {
	if name := time.Local.String(); name != "Local" {
		return name
	}
	target, err := os.Readlink("/etc/localtime")
	if err != nil {
		return ""
	}
	if index := strings.LastIndex(target, "zoneinfo/"); index >= 0 {
		return target[index+len("zoneinfo/"):]
	}
	return ""
}
