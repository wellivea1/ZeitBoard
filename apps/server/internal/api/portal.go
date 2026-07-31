package api

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"non24.app/server/internal/portal"
	"non24.app/server/internal/portalbridge"
	syncmodel "non24.app/server/internal/sync"
)

// portalAdmin is the owner-authenticated half of sharing. It is registered
// only when the portal is configured, so a daemon with the portal off has no
// sharing routes at all.
type portalAdmin struct {
	store        *portal.Store
	origin       string
	materializer portalbridge.Materializer
}

// WithPortal enables the share-link administration routes and wires
// materialization. The public /p/ mux is mounted separately by the daemon;
// this option governs only the owner surface.
func WithPortal(portalStore *portal.Store, publicOrigin string, materializer portalbridge.Materializer) Option {
	return func(s *Server) {
		s.portal = &portalAdmin{
			store:        portalStore,
			origin:       strings.TrimSuffix(publicOrigin, "/"),
			materializer: materializer,
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
	// Materialize immediately so the recipient does not open a blank page.
	if err := s.portal.materializer.MaterializeAll(r.Context()); err != nil {
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
	writeJSON(w, http.StatusOK, map[string]string{
		"schema_version": syncmodel.SchemaVersion,
		"status":         "erased",
	})
}

// refreshPortal republishes availability after private data changed. Failures
// are reported to the caller's log path rather than failing the sync request:
// a stale portal snapshot is a visible, honest state, but a rejected push
// would lose the device's data.
func (s *Server) refreshPortal(r *http.Request) {
	if s.portal == nil {
		return
	}
	if err := s.portal.materializer.MaterializeAll(r.Context()); err != nil {
		log.Printf("portal: snapshot refresh failed: %v", err)
	}
}
