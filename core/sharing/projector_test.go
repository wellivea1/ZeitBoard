package sharing

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"non24.app/core/domain"
)

func TestSharePermissionsHideUnapprovedAndSensitiveFields(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	wake := domain.AvailabilityWindow{
		ID: "wake", Kind: domain.AvailabilityPredictedWake,
		Interval: domain.TimeRange{
			Start: domain.MustZonedInstant(now.Add(2*time.Hour), "UTC"),
			End:   domain.MustZonedInstant(now.Add(4*time.Hour), "UTC"),
		},
	}
	status := domain.TrustedStatus{
		LikelyState:               "likely asleep",
		NextWakeWindow:            &wake,
		UrgentContactInstructions: "SENSITIVE_URGENT_INSTRUCTION",
		PrivateMedicationNames:    []string{"SENSITIVE_MEDICATION"},
		PrivateDiagnosis:          "SENSITIVE_DIAGNOSIS",
		PrivateLocation:           "SENSITIVE_LOCATION",
	}
	profile := domain.ShareProfile{Label: "Family", Permissions: []domain.SharePermission{domain.ShareNextWakeWindow}}
	view, err := Project(profile, status, now)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, forbidden := range []string{"likelyState", "urgentContactInstructions", "SENSITIVE_MEDICATION", "SENSITIVE_DIAGNOSIS", "SENSITIVE_LOCATION"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("projection exposed %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, "nextEstimatedWakeWindow") {
		t.Fatalf("approved field missing: %s", text)
	}
}

func TestExpiredAndRevokedProfilesProduceNoView(t *testing.T) {
	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	revoked := now.Add(-time.Hour)
	for _, profile := range []domain.ShareProfile{{ExpiresAt: &expired}, {RevokedAt: &revoked}} {
		if _, err := Project(profile, domain.TrustedStatus{}, now); !errors.Is(err, ErrProfileUnavailable) {
			t.Fatalf("error = %v", err)
		}
	}
}
