package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Visitor time requests (ADR-0030) reach the owner through the synced backend,
// not the local store: the portal runs on the self-hosted instance. They are
// deliberately kept off the generic proposals surface, because approving one
// requires choosing an exact block and the generic decision route refuses
// them for that reason.

const visitorRequestLocalLayout = "2006-01-02T15:04"

// BackendVisitorRequestDTO is what the Approvals screen renders. It carries
// the visitor's own words, which stay inside the owner's trust zone: this DTO
// is never projected back to the portal or to any agent surface.
type BackendVisitorRequestDTO struct {
	ProposalID string `json:"proposalId"`
	LinkLabel  string `json:"linkLabel"`
	Handle     string `json:"handle,omitempty"`
	Message    string `json:"message,omitempty"`

	WindowLabel   string `json:"windowLabel"`
	DurationLabel string `json:"durationLabel,omitempty"`

	// Local bounds for the owner's block picker, in the desktop's own time
	// zone. The visitor asked in theirs; the owner decides in theirs.
	WindowStartLocal string `json:"windowStartLocal"`
	WindowEndLocal   string `json:"windowEndLocal"`
	DurationMinutes  int    `json:"durationMinutes"`

	BeyondHorizon      bool   `json:"beyondHorizon"`
	BeyondHorizonNote  string `json:"beyondHorizonNote,omitempty"`
	CreatedLabel       string `json:"createdLabel"`
	ExpiresLabel       string `json:"expiresLabel"`
	ApprovalDisclosure string `json:"approvalDisclosure"`
	DecisionToken      string `json:"decisionToken,omitempty"`
}

type BackendVisitorRequestsDTO struct {
	Status   string                     `json:"status"`
	Message  string                     `json:"message,omitempty"`
	Requests []BackendVisitorRequestDTO `json:"requests"`
}

type backendVisitorRequestRecord struct {
	ProposalID      string    `json:"proposalId"`
	ProfileID       string    `json:"profileId"`
	Label           string    `json:"label"`
	Status          string    `json:"status"`
	WindowStartAt   time.Time `json:"windowStartAt"`
	WindowEndAt     time.Time `json:"windowEndAt"`
	ZoneID          string    `json:"zoneId"`
	DurationMinutes int       `json:"durationMinutes"`
	BeyondHorizon   bool      `json:"beyondHorizon"`
	Handle          string    `json:"handle"`
	Message         string    `json:"message"`
	CreatedAt       time.Time `json:"createdAt"`
	ExpiresAt       time.Time `json:"expiresAt"`
	DecisionToken   string    `json:"decisionToken"`
	Disclosure      string    `json:"disclosure"`
}

type backendVisitorRequestListResponse struct {
	Requests []backendVisitorRequestRecord `json:"requests"`
}

// DecideBackendVisitorRequestInput carries the owner's answer. StartLocal and
// EndLocal are `2006-01-02T15:04` in the desktop's local zone and are required
// only for an approval.
type DecideBackendVisitorRequestInput struct {
	ProposalID string `json:"proposalId"`
	Decision   string `json:"decision"`
	Token      string `json:"token"`
	StartLocal string `json:"startLocal"`
	EndLocal   string `json:"endLocal"`
}

// GetBackendVisitorRequests lists open requests from share links.
func (a *App) GetBackendVisitorRequests() (BackendVisitorRequestsDTO, error) {
	cfg, token, err := a.requireBackendSync()
	if err != nil {
		return BackendVisitorRequestsDTO{Status: "off", Requests: []BackendVisitorRequestDTO{}}, nil
	}
	return a.fetchBackendVisitorRequests(a.applicationContext(), cfg, token), nil
}

func (a *App) fetchBackendVisitorRequests(ctx context.Context, cfg backendSyncConfig, token string) BackendVisitorRequestsDTO {
	client := a.newDesktopBackendClient(cfg, token)
	var response backendVisitorRequestListResponse
	if err := client.getJSON(ctx, "/v1/portal/requests", &response); err != nil {
		// A backend without the portal enabled has no such route. That is a
		// normal configuration, not an error worth alarming the user about.
		if isBackendRouteAbsent(err) {
			return BackendVisitorRequestsDTO{Status: "off", Requests: []BackendVisitorRequestDTO{}}
		}
		return BackendVisitorRequestsDTO{
			Status:   "error",
			Message:  sanitizeBackendError(err),
			Requests: []BackendVisitorRequestDTO{},
		}
	}
	requests := make([]BackendVisitorRequestDTO, 0, len(response.Requests))
	for _, record := range response.Requests {
		if record.Status != "pending" {
			continue
		}
		requests = append(requests, backendVisitorRequestDTO(record))
	}
	return BackendVisitorRequestsDTO{Status: "ok", Requests: requests}
}

func backendVisitorRequestDTO(record backendVisitorRequestRecord) BackendVisitorRequestDTO {
	label := strings.TrimSpace(record.Label)
	if label == "" {
		label = "An unnamed link"
	}
	dto := BackendVisitorRequestDTO{
		ProposalID:         record.ProposalID,
		LinkLabel:          label,
		Handle:             record.Handle,
		Message:            record.Message,
		WindowLabel:        civilWindow(record.WindowStartAt, record.WindowEndAt, record.ZoneID),
		WindowStartLocal:   record.WindowStartAt.Local().Format(visitorRequestLocalLayout),
		WindowEndLocal:     record.WindowEndAt.Local().Format(visitorRequestLocalLayout),
		DurationMinutes:    record.DurationMinutes,
		BeyondHorizon:      record.BeyondHorizon,
		CreatedLabel:       "Asked " + record.CreatedAt.Local().Format("Jan 2, 3:04 PM"),
		ExpiresLabel:       "expires " + record.ExpiresAt.Local().Format("Jan 2, 3:04 PM"),
		ApprovalDisclosure: record.Disclosure,
		DecisionToken:      record.DecisionToken,
	}
	if record.DurationMinutes > 0 {
		dto.DurationLabel = fmt.Sprintf("%d minutes", record.DurationMinutes)
	}
	if record.BeyondHorizon {
		dto.BeyondHorizonNote = "This date is further ahead than the estimate reaches, so there is no availability to check it against."
	}
	return dto
}

// DecideBackendVisitorRequest records the owner's answer. Approving requires an
// exact block inside the requested window; the backend re-checks that, so a
// mistake here is refused rather than silently accepted.
func (a *App) DecideBackendVisitorRequest(input DecideBackendVisitorRequestInput) (BackendVisitorRequestsDTO, error) {
	cfg, token, err := a.requireBackendSync()
	if err != nil {
		return BackendVisitorRequestsDTO{
			Status:   "error",
			Message:  sanitizeBackendError(err),
			Requests: []BackendVisitorRequestDTO{},
		}, nil
	}
	payload := map[string]any{"decision": input.Decision, "token": input.Token}
	if input.Decision == "approved" {
		start, end, parseErr := parseVisitorSlot(input.StartLocal, input.EndLocal)
		if parseErr != nil {
			return BackendVisitorRequestsDTO{
				Status:   "error",
				Message:  parseErr.Error(),
				Requests: []BackendVisitorRequestDTO{},
			}, nil
		}
		payload["startAt"] = start.Format(time.RFC3339)
		payload["endAt"] = end.Format(time.RFC3339)
	}

	ctx := a.applicationContext()
	client := a.newDesktopBackendClient(cfg, token)
	var decided map[string]json.RawMessage
	path := "/v1/portal/requests/" + url.PathEscape(input.ProposalID) + "/decision"
	if err := client.postJSON(ctx, path, payload, &decided); err != nil {
		result := a.fetchBackendVisitorRequests(ctx, cfg, token)
		result.Status = "error"
		result.Message = sanitizeBackendError(err)
		return result, nil
	}
	return a.fetchBackendVisitorRequests(ctx, cfg, token), nil
}

// parseVisitorSlot reads the owner's chosen block in the desktop's own zone. A
// local time that does not exist on that date is refused rather than nudged,
// for the same reason the public form refuses one.
func parseVisitorSlot(startLocal, endLocal string) (time.Time, time.Time, error) {
	start, err := time.ParseInLocation(visitorRequestLocalLayout, strings.TrimSpace(startLocal), time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("Choose a start time for the block.")
	}
	end, err := time.ParseInLocation(visitorRequestLocalLayout, strings.TrimSpace(endLocal), time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("Choose an end time for the block.")
	}
	if start.Format(visitorRequestLocalLayout) != strings.TrimSpace(startLocal) ||
		end.Format(visitorRequestLocalLayout) != strings.TrimSpace(endLocal) {
		return time.Time{}, time.Time{}, errors.New("That local time does not exist on that date.")
	}
	if !end.After(start) {
		return time.Time{}, time.Time{}, errors.New("The block must end after it starts.")
	}
	return start.UTC(), end.UTC(), nil
}

// visitorRequestActionID mirrors the server's store.ActionVisitorRequest. The
// desktop cannot import the server module, so the constant is duplicated and
// pinned by TestVisitorRequestActionIDMatchesServer.
const visitorRequestActionID = "place_visitor_request"

// isBackendRouteAbsent reports whether an error is a 404 from the backend,
// which is what a daemon with the portal disabled returns for sharing routes.
func isBackendRouteAbsent(err error) bool {
	return err != nil && strings.Contains(err.Error(), "404")
}
