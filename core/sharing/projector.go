package sharing

import (
	"errors"
	"time"

	"non24.app/core/domain"
)

var ErrProfileUnavailable = errors.New("share profile is revoked or expired")

type TrustedView struct {
	ProfileLabel              string                      `json:"profileLabel"`
	GeneratedAt               time.Time                   `json:"generatedAt"`
	LikelyState               *string                     `json:"likelyState,omitempty"`
	NextEstimatedWakeWindow   *domain.AvailabilityWindow  `json:"nextEstimatedWakeWindow,omitempty"`
	BestContactWindow         *domain.AvailabilityWindow  `json:"bestContactWindow,omitempty"`
	SevenDayAvailability      []domain.AvailabilityWindow `json:"sevenDayAvailability,omitempty"`
	UrgentContactInstructions *string                     `json:"urgentContactInstructions,omitempty"`
	CalendarAvailability      []domain.AvailabilityWindow `json:"calendarAvailability,omitempty"`
}

func Project(profile domain.ShareProfile, status domain.TrustedStatus, now time.Time) (TrustedView, error) {
	if profile.RevokedAt != nil || (profile.ExpiresAt != nil && !now.Before(*profile.ExpiresAt)) {
		return TrustedView{}, ErrProfileUnavailable
	}
	permissions := make(map[domain.SharePermission]struct{}, len(profile.Permissions))
	for _, permission := range profile.Permissions {
		permissions[permission] = struct{}{}
	}
	view := TrustedView{ProfileLabel: profile.Label, GeneratedAt: now.UTC()}
	if allowed(permissions, domain.ShareLikelyState) {
		value := status.LikelyState
		view.LikelyState = &value
	}
	if allowed(permissions, domain.ShareNextWakeWindow) {
		view.NextEstimatedWakeWindow = cloneWindow(status.NextWakeWindow)
	}
	if allowed(permissions, domain.ShareBestContactWindow) {
		view.BestContactWindow = cloneWindow(status.BestContactWindow)
	}
	if allowed(permissions, domain.ShareSevenDayForecast) {
		view.SevenDayAvailability = append([]domain.AvailabilityWindow(nil), status.SevenDayAvailability...)
	}
	if allowed(permissions, domain.ShareUrgentInstructions) {
		value := status.UrgentContactInstructions
		view.UrgentContactInstructions = &value
	}
	if allowed(permissions, domain.ShareCalendarAvailability) {
		view.CalendarAvailability = append([]domain.AvailabilityWindow(nil), status.CalendarAvailability...)
	}
	return view, nil
}

func allowed(permissions map[domain.SharePermission]struct{}, permission domain.SharePermission) bool {
	_, ok := permissions[permission]
	return ok
}

func cloneWindow(source *domain.AvailabilityWindow) *domain.AvailabilityWindow {
	if source == nil {
		return nil
	}
	copyValue := *source
	copyValue.Confidence.Reasons = append([]string(nil), source.Confidence.Reasons...)
	return &copyValue
}
