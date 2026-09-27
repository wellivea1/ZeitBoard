package main

import (
	"sync"

	"non24.app/core/platform/localzone"
)

// systemZone names this computer's zone once per run, as Go reads the local
// clock once per run: after the owner travels, new records take the new zone
// once the app restarts. It is UTC when the system names no zone, never a
// guessed place.
var systemZone = sync.OnceValue(func() string {
	if id := localzone.ID(); id != "" {
		return id
	}
	return "UTC"
})

// localZoneID is this computer's IANA zone: the zone new records carry when
// the screen does not give one, and the zone the desktop's own computations
// state civil times in.
func localZoneID() string { return systemZone() }
