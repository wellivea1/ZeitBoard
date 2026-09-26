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
// not the local store: the portal runs on the self-hosted instance. They share the
// review queue with other proposals, but use a dedicated decision route because
// approval requires an exact reviewed block.

// BackendVisitorRequestDTO is what the Approvals screen renders. It carries
// the visitor's own words, which stay inside the owner's trust zone: this DTO
// is never projected back to the portal or to any agent surface.
type BackendVisitorRequestDTO struct {
	Status     string `json:"status"`
	ExpiresAt  string `json:"expiresAt"`
	ProposalID string `json:"proposalId"`
	LinkLabel  string `json:"linkLabel"`
	Handle     string `json:"handle,omitempty"`
	Message    string `json:"message,omitempty"`

	WindowLabel   string `json:"windowLabel"`
	DurationLabel string `json:"durationLabel,omitempty"`

	// Exact bounds are rendered in the owner's selected local zone.
	WindowStartAt   string `json:"windowStartAt"`
	WindowEndAt     string `json:"windowEndAt"`
	DurationMinutes int    `json:"durationMinutes"`

	BeyondHorizon      bool   `json:"beyondHorizon"`
	BeyondHorizonNote  string `json:"beyondHorizonNote,omitempty"`
	CreatedLabel       string `json:"createdLabel"`
	ExpiresLabel       string `json:"expiresLabel"`
	ApprovalDisclosure string `json:"approvalDisclosure"`
	DecisionToken      string `json:"decisionToken,omitempty"`

	// The request's thread (P5-c): the visitor's words and the owner's, in
	// order. CanMessage is whether a reply can still be added.
	Messages   []VisitorMessageDTO `json:"messages"`
	CanMessage bool                `json:"canMessage"`
}

type VisitorMessageDTO struct {
	Author       string `json:"author"`
	AuthorLabel  string `json:"authorLabel"`
	Body         string `json:"body"`
	CreatedLabel string `json:"createdLabel"`
}

// VisitorMessageInput is the owner's reply to one request's thread.
type VisitorMessageInput struct {
	ProposalID string `json:"proposalId"`
	Message    string `json:"message"`
}

type BackendVisitorRequestsDTO struct {
	DecisionRecorded bool                         `json:"decisionRecorded,omitempty"`
	PendingCount     int                          `json:"pendingCount"`
	NextExpiryAt     string                       `json:"nextExpiryAt"`
	Pagination       BackendProposalPaginationDTO `json:"pagination"`
	Status           string                       `json:"status"`
	Message          string                       `json:"message,omitempty"`
	Requests         []BackendVisitorRequestDTO   `json:"requests"`
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
	Messages        []struct {
		Author    string    `json:"author"`
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"createdAt"`
	} `json:"messages"`
	CanMessage bool `json:"canMessage"`
}

type backendVisitorRequestListResponse struct {
	SchemaVersion string                        `json:"schema_version"`
	PendingCount  int                           `json:"pendingCount"`
	NextExpiryAt  string                        `json:"nextExpiryAt"`
	Pagination    backendProposalPagination     `json:"pagination"`
	Requests      []backendVisitorRequestRecord `json:"requests"`
}

// DecideBackendVisitorRequestInput carries the owner's answer. StartAt and
// EndAt are exact RFC3339 instants selected in the owner's local zone and are required
// only for an approval.
type DecideBackendVisitorRequestInput struct {
	ProposalID string `json:"proposalId"`
	Decision   string `json:"decision"`
	Token      string `json:"token"`
	StartAt    string `json:"startAt"`
	EndAt      string `json:"endAt"`
}

// GetBackendVisitorRequests lists pending requests and history from share links.
func (a *App) GetBackendVisitorRequests() (BackendVisitorRequestsDTO, error) {
	return a.getBackendVisitorRequestPage("")
}

func (a *App) GetBackendVisitorRequestPage(input BackendProposalPageInput) (BackendVisitorRequestsDTO, error) {
	if len(input.Cursor) == 0 || len(input.Cursor) > 512 {
		return BackendVisitorRequestsDTO{}, errors.New("visitor request cursor is invalid")
	}
	return a.getBackendVisitorRequestPage(input.Cursor)
}

func (a *App) getBackendVisitorRequestPage(cursor string) (BackendVisitorRequestsDTO, error) {
	cfg, token, err := a.requireBackendSync()
	if err != nil {
		if cfg.Enabled {
			return BackendVisitorRequestsDTO{Status: "error", Message: sanitizeBackendError(err), Requests: []BackendVisitorRequestDTO{}}, nil
		}
		return BackendVisitorRequestsDTO{Status: "off", Requests: []BackendVisitorRequestDTO{}}, nil
	}
	return a.fetchBackendVisitorRequestPage(a.applicationContext(), cfg, token, cursor), nil
}

func (a *App) fetchBackendVisitorRequests(ctx context.Context, cfg backendSyncConfig, token string) BackendVisitorRequestsDTO {
	return a.fetchBackendVisitorRequestPage(ctx, cfg, token, "")
}

func (a *App) fetchBackendVisitorRequestPage(ctx context.Context, cfg backendSyncConfig, token, cursor string) BackendVisitorRequestsDTO {
	client := a.newDesktopBackendClient(cfg, token)
	var response backendVisitorRequestListResponse
	path := "/v1/portal/requests"
	if cursor != "" {
		path += "?" + url.Values{"cursor": []string{cursor}}.Encode()
	}
	if err := client.getJSON(ctx, path, &response); err != nil {
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
		requests = append(requests, backendVisitorRequestDTO(record))
	}
	return BackendVisitorRequestsDTO{Status: "ok", Requests: requests, PendingCount: response.PendingCount, NextExpiryAt: response.NextExpiryAt,
		Pagination: BackendProposalPaginationDTO{Limit: response.Pagination.Limit, HasMore: response.Pagination.HasMore, NextCursor: response.Pagination.NextCursor}}
}

func backendVisitorRequestDTO(record backendVisitorRequestRecord) BackendVisitorRequestDTO {
	label := strings.TrimSpace(record.Label)
	if label == "" {
		label = "An unnamed link"
	}
	dto := BackendVisitorRequestDTO{
		Status:             record.Status,
		ExpiresAt:          record.ExpiresAt.UTC().Format(time.RFC3339Nano),
		ProposalID:         record.ProposalID,
		LinkLabel:          label,
		Handle:             record.Handle,
		Message:            record.Message,
		WindowLabel:        civilWindow(record.WindowStartAt, record.WindowEndAt, record.ZoneID),
		WindowStartAt:      record.WindowStartAt.UTC().Format(time.RFC3339Nano),
		WindowEndAt:        record.WindowEndAt.UTC().Format(time.RFC3339Nano),
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
	dto.CanMessage = record.CanMessage
	dto.Messages = make([]VisitorMessageDTO, 0, len(record.Messages))
	for _, message := range record.Messages {
		author := "They wrote"
		if message.Author == "owner" {
			author = "You wrote"
		}
		dto.Messages = append(dto.Messages, VisitorMessageDTO{
			Author:       message.Author,
			AuthorLabel:  author,
			Body:         message.Body,
			CreatedLabel: message.CreatedAt.Local().Format("Jan 2, 3:04 PM"),
		})
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
		start, end, parseErr := parseVisitorSlot(input.StartAt, input.EndAt)
		if parseErr != nil {
			return BackendVisitorRequestsDTO{
				Status:   "error",
				Message:  parseErr.Error(),
				Requests: []BackendVisitorRequestDTO{},
			}, nil
		}
		payload["startAt"] = start.Format(time.RFC3339Nano)
		payload["endAt"] = end.Format(time.RFC3339Nano)
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
	result := a.fetchBackendVisitorRequests(ctx, cfg, token)
	result.DecisionRecorded = true
	if result.Status != "ok" {
		result.Status = "error"
		result.Message = "Decision recorded. The queue could not be refreshed; refresh it before another decision."
	}
	return result, nil
}

// ReplyToBackendVisitorRequest adds the owner's message to a request's thread
// and returns the refreshed queue, so the reply appears where it was typed.
func (a *App) ReplyToBackendVisitorRequest(input VisitorMessageInput) (BackendVisitorRequestsDTO, error) {
	message := strings.TrimSpace(input.Message)
	if message == "" {
		return BackendVisitorRequestsDTO{Status: "error", Message: "Write a message first.", Requests: []BackendVisitorRequestDTO{}}, nil
	}
	return a.visitorThreadAction(input.ProposalID, "messages", map[string]any{"message": message})
}

// EraseBackendVisitorThread deletes a request's messages now, rather than two
// weeks after its answer.
func (a *App) EraseBackendVisitorThread(input VisitorMessageInput) (BackendVisitorRequestsDTO, error) {
	return a.visitorThreadAction(input.ProposalID, "erase-thread", map[string]any{})
}

func (a *App) visitorThreadAction(proposalID, action string, payload map[string]any) (BackendVisitorRequestsDTO, error) {
	cfg, token, err := a.requireBackendSync()
	if err != nil {
		return BackendVisitorRequestsDTO{Status: "error", Message: sanitizeBackendError(err), Requests: []BackendVisitorRequestDTO{}}, nil
	}
	if strings.TrimSpace(proposalID) == "" {
		return BackendVisitorRequestsDTO{Status: "error", Message: "No request was selected.", Requests: []BackendVisitorRequestDTO{}}, nil
	}
	ctx := a.applicationContext()
	client := a.newDesktopBackendClient(cfg, token)
	var response map[string]json.RawMessage
	path := "/v1/portal/requests/" + url.PathEscape(proposalID) + "/" + action
	if err := client.postJSON(ctx, path, payload, &response); err != nil {
		result := a.fetchBackendVisitorRequests(ctx, cfg, token)
		result.Status = "error"
		result.Message = sanitizeBackendError(err)
		return result, nil
	}
	return a.fetchBackendVisitorRequests(ctx, cfg, token), nil
}

// The picker sends exact instants after resolving civil-time gaps and repeated
// hours. No host-zone guess can change a block the owner has reviewed.
func parseVisitorSlot(startValue, endValue string) (time.Time, time.Time, error) {
	start, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(startValue))
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("Choose an exact start time for the block.")
	}
	end, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(endValue))
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("Choose an exact end time for the block.")
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
