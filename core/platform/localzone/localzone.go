// Package localzone names this computer's time zone as an IANA zone id.
//
// Go's time.Local knows the local rules but, on Windows and wherever it was
// read from /etc/localtime, not their name: it is called "Local". ZeitBoard
// records carry a zone name, so the desktop asks the system for it.
package localzone

import (
	"time"
	_ "time/tzdata"
)

// ID returns this computer's IANA zone id, or "" when the system names none.
func ID() string {
	return valid(platformID())
}

func valid(id string) string {
	if id == "" || id == "Local" {
		return ""
	}
	if _, err := time.LoadLocation(id); err != nil {
		return ""
	}
	return id
}
