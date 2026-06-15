package schema

import (
	"testing"

	"non24.app/tools/internal/repo"
)

func testSet(t *testing.T) (*Set, string) {
	t.Helper()
	root, err := repo.Root()
	if err != nil {
		t.Fatal(err)
	}
	set, err := Load(root)
	if err != nil {
		t.Fatalf("load schemas: %v", err)
	}
	return set, root
}

// TestValidateAll is the positive case: every contract schema is well-formed
// and every checked-in fixture validates, matching the former check-jsonschema run.
func TestValidateAll(t *testing.T) {
	_, root := testSet(t)
	if err := ValidateAll(root); err != nil {
		t.Fatalf("contract validation failed: %v", err)
	}
}

// TestRejectsOverSharedTrustedView locks parity on the trusted-view
// if/then/else: a window present without being granted must be rejected.
func TestRejectsOverSharedTrustedView(t *testing.T) {
	set, _ := testSet(t)
	overShared := []byte(`{
		"schema_version": "v1",
		"generated_at": "2026-03-15T00:00:00Z",
		"expires_at": "2026-03-16T00:00:00Z",
		"granted_fields": ["confidence"],
		"confidence": "low",
		"predicted_sleep_window": {"earliest_at": "2026-03-15T12:00:00Z", "latest_at": "2026-03-15T13:00:00Z"},
		"notice": "Estimated windows are uncertain and are not medical advice."
	}`)
	if err := set.ValidateBytes("trusted-view.schema.json", overShared); err == nil {
		t.Fatal("expected rejection: predicted_sleep_window present but not granted")
	}
}

// TestRejectsRefusedEstimateWithForecasts locks parity on the phase-estimate
// oneOf: a refused result must not carry forecasts.
func TestRejectsRefusedEstimateWithForecasts(t *testing.T) {
	set, _ := testSet(t)
	bad := []byte(`{
		"schema_version": "v1",
		"status": "refused",
		"generated_at": "2026-03-15T00:00:00Z",
		"algorithm_version": "x",
		"refusal": {"code": "insufficient_data", "message": "m"},
		"forecasts": [{
			"cycle_index": 1,
			"predicted_sleep_window": {"earliest_at": "2026-03-15T12:00:00Z", "latest_at": "2026-03-15T13:00:00Z", "zone_id": "UTC"},
			"predicted_waking_window": {"earliest_at": "2026-03-15T20:00:00Z", "latest_at": "2026-03-15T21:00:00Z", "zone_id": "UTC"}
		}]
	}`)
	if err := set.ValidateBytes("phase-estimate.schema.json", bad); err == nil {
		t.Fatal("expected rejection: refused estimate must not include forecasts")
	}
}

// TestRejectsMalformedDateTime confirms format assertion is enabled, matching
// check-jsonschema's default format checking.
func TestRejectsMalformedDateTime(t *testing.T) {
	set, _ := testSet(t)
	bad := []byte(`{
		"schema_version": "v1",
		"generated_at": "not-a-timestamp",
		"expires_at": "2026-03-16T00:00:00Z",
		"granted_fields": [],
		"notice": "Estimated windows are uncertain and are not medical advice."
	}`)
	if err := set.ValidateBytes("trusted-view.schema.json", bad); err == nil {
		t.Fatal("expected rejection: generated_at is not a valid date-time")
	}
}
