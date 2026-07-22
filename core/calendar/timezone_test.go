package calendar

import "testing"

func TestLoadLocationAcceptsPathPrefixedIANAZone(t *testing.T) {
	location, canonical, err := loadLocation("/mozilla.org/20050126_1/America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	if canonical != "America/New_York" || location.String() != canonical {
		t.Fatalf("location = %q, canonical = %q", location, canonical)
	}
}
