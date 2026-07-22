package schema

import (
	"path/filepath"
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

func TestMedicationContractV2DoesNotRedefineV1(t *testing.T) {
	v1, root := testSet(t)
	if err := v1.ValidateFile("medication-set.schema.json", filepath.Join(root, "testdata", "v1", "medication-set.json")); err != nil {
		t.Fatalf("v1 medication fixture no longer validates: %v", err)
	}
	v2, err := loadVersion(root, "v2")
	if err != nil {
		t.Fatal(err)
	}
	if err := v2.ValidateFile("medication-data-export.schema.json", filepath.Join(root, "testdata", "v2", "medication-data-export.json")); err != nil {
		t.Fatalf("v2 medication export fixture does not validate: %v", err)
	}
	bad := []byte(`{
		"schema_version": "v2",
		"generated_at": "2026-03-15T00:00:00Z",
		"medications": [{
			"medication_id": "med_test_01",
			"label": "Synthetic",
			"active": true,
			"schedule": {"kind": "fixed_clock", "civil_times": ["09:00"], "reminder_enabled": true},
			"created_at": "2026-03-01T00:00:00Z",
			"revision": 1,
			"updated_at": "2026-03-01T00:00:00Z"
		}]
	}`)
	if err := v2.ValidateBytes("medication-set.schema.json", bad); err == nil {
		t.Fatal("v2 fixed-clock schedule without an explicit zone was accepted")
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
