package main

import (
	"os"
	"testing"
)

// defaultZoneID is where the tests' computer is: they state civil times in
// New York whatever zone the machine running them is in.
const defaultZoneID = "America/New_York"

func TestMain(m *testing.M) {
	systemZone = func() string { return defaultZoneID }
	os.Exit(m.Run())
}
