package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"non24.app/core/recompute"
	"non24.app/server/internal/portal"
	"non24.app/server/internal/portalbridge"
	"non24.app/server/internal/store"
	syncmodel "non24.app/server/internal/sync"
)

// Recomputer is the analysis loop, narrowed to the two things an HTTP handler
// may ask of it. There is deliberately no "materialize now and tell me what
// changed": a request handler's job is to say that something changed, not to
// decide when the consequences are computed.
type Recomputer interface {
	// Request notes that inputs changed. It must not block and must not fail:
	// the work is derivable from the inputs, so a dropped request is recovered
	// by the next run rather than lost.
	Request(reason recompute.Reason)

	// RunNow recomputes and waits. It is for the handlers that must not return
	// until the result exists.
	RunNow(ctx context.Context, reason recompute.Reason) error
}

// PortalConfig is everything the owner-facing sharing surface needs.
type PortalConfig struct {
	Store        *portal.Store
	PublicOrigin string
	Materializer portalbridge.Materializer
	Requests     *portalbridge.RequestBridge

	// Recompute is required. Publishing the projection from two places would
	// mean two different ideas of when it was last updated, and the older stamp
	// would lose to the newer one regardless of which was true.
	Recompute Recomputer
}

// portalAdmin is the owner-authenticated half of sharing. It is registered
// only when the portal is configured, so a daemon with the portal off has no
// sharing routes at all.
type portalAdmin struct {
	store     *portal.Store
	origin    string
	requests  *portalbridge.RequestBridge
	recompute Recomputer
}

// WithPortal enables the share-link administration routes. The public /p/ mux
// is mounted separately by the daemon; this option governs only the owner
// surface.
func WithPortal(cfg PortalConfig) Option {
	return func(s *Server) {
		s.portal = &portalAdmin{
			store:     cfg.Store,
			origin:    strings.TrimSuffix(cfg.PublicOrigin, "/"),
			requests:  cfg.Requests,
			recompute: cfg.Recompute,
		}
	}
}

// sharingDisclosure is required by exposure-gate item 7. It states the
// measured uncertainty and the residual risk of live sharing in the same
// breath as the link, so an owner cannot create one without seeing both.
const sharingDisclosure = "Anyone with this link and passcode can see broad windows when you are likely awake, and can watch those windows change over time. " +
	"The estimate is often off by about 2 hours and sometimes more. " +
	"Screenshots and anything a recipient remembers cannot be revoked."

type createPortalProfileRequest struct {
	Label         string        `json:"label"`
	Passcode      string        `json:"passcode"`
	ExpiresInDays int           `json:"expiresInDays"`
	Grants        portal.Grants `json:"grants"`
}

type createPortalProfileResponse struct {
	SchemaVersion string    `json:"schema_version"`
	ProfileID     string    `json:"profileId"`
	LinkURL       string    `json:"linkUrl"`
	ExpiresAt     time.Time `json:"expiresAt"`
	Disclosure    string    `json:"disclosure"`
}

type portalProfileDTO struct {
	ProfileID string                 `json:"profileId"`
	Label     string                 `json:"label"`
	State     string                 `json:"state"`
	CreatedAt time.Time              `json:"createdAt"`
	ExpiresAt time.Time              `json:"expiresAt"`
	Grants    portal.Grants          `json:"grants"`
	Access    []portalAccessCountDTO `json:"access"`
}

type portalAccessCountDTO struct {
	Event      string    `json:"event"`
	Count      int       `json:"count"`
	LastAccess time.Time `json:"lastAccess"`
}

type portalProfileListResponse struct {
	SchemaVersion string             `json:"schema_version"`
	Profiles      []portalProfileDTO `json:"profiles"`
	Disclosure    string             `json:"disclosure"`
}

func (s *Server) handleCreatePortalProfile(w http.ResponseWriter, r *http.Request) {
	var req createPortalProfileRequest
	if err := decodeBody(w, r, maxDeviceBodyBytes, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	if req.ExpiresInDays <= 0 {
		writeError(w, http.StatusBadRequest, "expiresInDays must be positive")
		return
	}
	now := s.now()
	expiresAt := now.Add(time.Duration(req.ExpiresInDays) * 24 * time.Hour)

	profileID, err := portal.NewProfileID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "share profile creation failed")
		return
	}
	token, err := portal.NewLinkToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "share profile creation failed")
		return
	}
	if err := s.portal.store.CreateProfile(r.Context(), portal.CreateProfileInput{
		ProfileID: profileID,
		Token:     token,
		Passcode:  req.Passcode,
		Grants:    req.Grants,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}); err != nil {
		// Validation failures here are the caller's fault (short passcode,
		// over-long lifetime); they carry no secret and are safe to relay.
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.PutPortalLabel(r.Context(), profileID, req.Label, now.Format(time.RFC3339Nano)); err != nil {
		writeError(w, http.StatusInternalServerError, "share profile creation failed")
		return
	}
	// Recompute before returning the link, so the recipient does not open a
	// blank page. This is the one sharing path that waits: every other
	// consequence of a change is scheduled.
	if err := s.portal.recompute.RunNow(r.Context(), recompute.ReasonSharing); err != nil {
		writeError(w, http.StatusInternalServerError, "share profile creation failed")
		return
	}

	writeJSON(w, http.StatusCreated, createPortalProfileResponse{
		SchemaVersion: syncmodel.SchemaVersion,
		ProfileID:     profileID,
		LinkURL:       s.portal.origin + "/p/" + token,
		ExpiresAt:     expiresAt,
		Disclosure:    sharingDisclosure,
	})
}

func (s *Server) handleListPortalProfiles(w http.ResponseWriter, r *http.Request) {
	now := s.now()
	states, err := s.portal.store.ListProfiles(r.Context(), now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "share profile list failed")
		return
	}
	labels, err := s.store.PortalLabels(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "share profile list failed")
		return
	}
	since := now.Add(-30 * 24 * time.Hour)

	profiles := make([]portalProfileDTO, 0, len(states))
	for _, state := range states {
		summaries, err := s.portal.store.SummarizeAccess(r.Context(), state.ID, since)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "share profile list failed")
			return
		}
		access := make([]portalAccessCountDTO, 0, len(summaries))
		for _, summary := range summaries {
			access = append(access, portalAccessCountDTO{
				Event:      string(summary.Event),
				Count:      summary.Count,
				LastAccess: summary.LastAccess,
			})
		}
		profiles = append(profiles, portalProfileDTO{
			ProfileID: state.ID,
			Label:     labels[state.ID],
			State:     profileState(state),
			CreatedAt: state.CreatedAt,
			ExpiresAt: state.ExpiresAt,
			Grants:    state.Grants,
			Access:    access,
		})
	}
	writeJSON(w, http.StatusOK, portalProfileListResponse{
		SchemaVersion: syncmodel.SchemaVersion,
		Profiles:      profiles,
		Disclosure:    sharingDisclosure,
	})
}

func profileState(state portal.ProfileState) string {
	switch {
	case state.Revoked:
		return "revoked"
	case state.Expired:
		return "expired"
	default:
		return "active"
	}
}

func (s *Server) handleRevokePortalProfile(w http.ResponseWriter, r *http.Request) {
	profileID := strings.TrimSpace(r.PathValue("id"))
	if profileID == "" {
		writeError(w, http.StatusBadRequest, "share profile id is required")
		return
	}
	if err := s.portal.store.RevokeProfile(r.Context(), profileID, s.now()); err != nil {
		if errors.Is(err, portal.ErrProfileNotFound) {
			writeError(w, http.StatusNotFound, "share profile not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "share profile revoke failed")
		return
	}
	// Revocation takes effect immediately at the link, which is what matters.
	// The recompute is bookkeeping: the revoked profile leaves the consumer set,
	// so the next run stops publishing to it.
	s.requestRecompute(recompute.ReasonSharing)
	writeJSON(w, http.StatusOK, map[string]string{
		"schema_version": syncmodel.SchemaVersion,
		"status":         "revoked",
	})
}

// handleErasePortalProfile removes the owner-side label and the portal-side
// access audit for a link. Revocation stops access; this erases the record
// that the link existed, matching the ADR-0014/0017 erasure right.
func (s *Server) handleErasePortalProfile(w http.ResponseWriter, r *http.Request) {
	profileID := strings.TrimSpace(r.PathValue("id"))
	if profileID == "" {
		writeError(w, http.StatusBadRequest, "share profile id is required")
		return
	}
	if err := s.portal.store.RevokeProfile(r.Context(), profileID, s.now()); err != nil && !errors.Is(err, portal.ErrProfileNotFound) {
		writeError(w, http.StatusInternalServerError, "share profile erase failed")
		return
	}
	if err := s.portal.store.EraseAudit(r.Context(), profileID); err != nil {
		writeError(w, http.StatusInternalServerError, "share profile erase failed")
		return
	}
	if err := s.store.DeletePortalLabel(r.Context(), profileID); err != nil {
		writeError(w, http.StatusInternalServerError, "share profile erase failed")
		return
	}
	s.requestRecompute(recompute.ReasonSharing)
	writeJSON(w, http.StatusOK, map[string]string{
		"schema_version": syncmodel.SchemaVersion,
		"status":         "erased",
	})
}

type visitorRequestDTO struct {
	ProposalID      string    `json:"proposalId"`
	ProfileID       string    `json:"profileId"`
	Label           string    `json:"label"`
	Status          string    `json:"status"`
	WindowStartAt   time.Time `json:"windowStartAt"`
	WindowEndAt     time.Time `json:"windowEndAt"`
	ZoneID          string    `json:"zoneId"`
	DurationMinutes int       `json:"durationMinutes,omitempty"`
	BeyondHorizon   bool      `json:"beyondHorizon"`
	Handle          string    `json:"handle,omitempty"`
	Message         string    `json:"message,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	ExpiresAt       time.Time `json:"expiresAt"`
	DecisionToken   string    `json:"decisionToken,omitempty"`
	Disclosure      string    `json:"disclosure"`
}

// approvalDisclosure is required by design section 6: approving reveals the
// exact block to the visitor. That is necessary coordination information, and
// the owner must be told it before choosing, not after.
const approvalDisclosure = "Approving tells them the exact time you pick. Declining tells them only that the time did not work — never why."

type visitorRequestListResponse struct {
	SchemaVersion string              `json:"schema_version"`
	Requests      []visitorRequestDTO `json:"requests"`
}

// handleListVisitorRequests returns open visitor requests with the private
// handle and message the owner needs to judge them. This is an owner-side,
// device-authenticated route; none of this text is public.
func (s *Server) handleListVisitorRequests(w http.ResponseWriter, r *http.Request) {
	now := s.now()
	page, err := s.store.ListProposalPage(r.Context(), store.ProposalPageCursor{}, store.MaxProposalPageLimit, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "visitor request list failed")
		return
	}
	labels, err := s.store.PortalLabels(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "visitor request list failed")
		return
	}

	requests := make([]visitorRequestDTO, 0)
	for _, record := range page.Records {
		if record.ActionID != store.ActionVisitorRequest {
			continue
		}
		payload, decodeErr := store.VisitorProposalPayloadOf(record)
		if decodeErr != nil {
			writeError(w, http.StatusInternalServerError, "visitor request list failed")
			return
		}
		requests = append(requests, visitorRequestDTO{
			ProposalID:      record.ID,
			ProfileID:       payload.ProfileID,
			Label:           labels[payload.ProfileID],
			Status:          string(record.Status),
			WindowStartAt:   payload.WindowStart,
			WindowEndAt:     payload.WindowEnd,
			ZoneID:          payload.ZoneID,
			DurationMinutes: payload.DurationMinutes,
			BeyondHorizon:   payload.BeyondHorizon,
			Handle:          payload.Handle,
			Message:         payload.Message,
			CreatedAt:       record.CreatedAt,
			ExpiresAt:       record.ExpiresAt,
			DecisionToken:   record.DecisionToken,
			Disclosure:      approvalDisclosure,
		})
	}
	writeJSON(w, http.StatusOK, visitorRequestListResponse{
		SchemaVersion: syncmodel.SchemaVersion,
		Requests:      requests,
	})
}

type visitorDecisionRequest struct {
	Decision string     `json:"decision"`
	Token    string     `json:"token"`
	StartAt  *time.Time `json:"startAt,omitempty"`
	EndAt    *time.Time `json:"endAt,omitempty"`
}

func (s *Server) handleDecideVisitorRequest(w http.ResponseWriter, r *http.Request) {
	device := deviceFromContext(r.Context())
	proposalID := strings.TrimSpace(r.PathValue("id"))
	var req visitorDecisionRequest
	if err := decodeBody(w, r, maxDeviceBodyBytes, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	slot := store.VisitorSlot{}
	if req.StartAt != nil {
		slot.StartAt = *req.StartAt
	}
	if req.EndAt != nil {
		slot.EndAt = *req.EndAt
	}

	record, err := s.store.DecideVisitorProposal(r.Context(), proposalID, device.ID,
		store.ProposalStatus(req.Decision), req.Token, slot, s.now())
	switch {
	case errors.Is(err, store.ErrVisitorSlotOutOfWindow):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, store.ErrNotVisitorProposal):
		writeError(w, http.StatusConflict, "that proposal is not a visitor request")
		return
	case errors.Is(err, store.ErrExpiredApprovalToken):
		writeError(w, http.StatusGone, "approval token expired")
		return
	case errors.Is(err, store.ErrUsedApprovalToken), errors.Is(err, store.ErrProposalNotPending):
		writeError(w, http.StatusConflict, "request already decided")
		return
	case errors.Is(err, store.ErrInvalidApprovalToken):
		writeError(w, http.StatusUnauthorized, "invalid approval token")
		return
	case errors.Is(err, store.ErrProposalNotFound):
		writeError(w, http.StatusNotFound, "request not found")
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, "request decision failed")
		return
	}

	// Deliver the answer immediately so the visitor is not left waiting on the
	// next timer tick. The decision is already durable either way.
	s.pumpPortalBridge(r)

	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": syncmodel.SchemaVersion,
		"proposal":       record,
	})
}

// pumpPortalBridge runs one bridge cycle. Failures are logged rather than
// returned: the decision is committed, and the outbox guarantees delivery on a
// later pass.
func (s *Server) pumpPortalBridge(r *http.Request) {
	if s.portal == nil || s.portal.requests == nil {
		return
	}
	if err := s.portal.requests.Pump(r.Context()); err != nil {
		log.Printf("portal: bridge pump failed: %v", err)
	}
}

// requestRecompute reports that analysis inputs changed.
//
// It used to republish the projection inline, inside the request. That bound
// the work to the caller's context, so a device that hung up mid-push cancelled
// the recomputation its own data had just made necessary, and it charged every
// push for a computation that a burst of pushes only needs once. Handing the
// reason to the scheduler fixes both, and costs the guarantee that the
// projection is current the instant the response returns — which was never a
// guarantee worth having, since nothing was reading it that fast.
func (s *Server) requestRecompute(reason recompute.Reason) {
	if s.portal == nil || s.portal.recompute == nil {
		return
	}
	s.portal.recompute.Request(reason)
}
