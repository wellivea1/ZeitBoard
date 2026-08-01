package portal

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

//go:embed assets/portal.css assets/pages.gohtml
var assetsFS embed.FS

var pageTemplates = template.Must(template.ParseFS(assetsFS, "assets/pages.gohtml"))

// pageData is the only value templates ever see. It has no field that could
// carry a private record, which is what makes the canary test in
// portal_leak_test.go a structural guarantee rather than a spot check.
type pageData struct {
	Title      string
	Refresh    bool
	Error      string
	FormAction string
	View       AvailabilityView

	// Request fields. Handle and Message inside RequestView are the visitor's
	// own words being shown back to them; they never appear on the
	// availability page, in a projection DTO, or in an audit row.
	Request       RequestView
	CSRFToken     string
	RecoveryCode  string
	ContinueURL   string
	LinkTokenPath string
	MinLocal      string
	CanRequest    bool
	RequestsPath  string
}

func (h *Handler) handleStylesheet(w http.ResponseWriter, r *http.Request) {
	data, err := assetsFS.ReadFile("assets/portal.css")
	if err != nil {
		h.writeGeneric(w, r, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *Handler) handlePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	profile := profileFromContext(ctx)
	if _, ok := h.sessionFor(r, profile); !ok {
		h.renderPasscode(w, r, "")
		return
	}
	_ = h.store.RecordAccess(ctx, profile.ID, EventPageView, sourceFromContext(ctx), h.now())

	snapshot, err := h.store.ReadSnapshot(ctx, profile.ID)
	if err != nil {
		// A missing snapshot is a legitimate state: the link exists but
		// nothing has been materialized for it yet. Anything else — a decrypt
		// failure, a corrupt row — renders the same empty page to the visitor
		// but must not vanish silently, because only the operator can act on
		// it. The profile id is opaque; the link token is never logged.
		if !errors.Is(err, ErrNoSnapshot) {
			log.Printf("portal: snapshot unreadable for profile %s: %v", profile.ID, err)
		}
		snapshot = Snapshot{}
	}
	view := BuildView(snapshot, h.now())
	h.renderPage(w, r, http.StatusOK, "dashboard", pageData{
		Title:        "Availability",
		Refresh:      true,
		View:         view,
		CanRequest:   profile.Grants.AllowRequests,
		RequestsPath: requestsPath(r.PathValue("linkToken")),
	})
}

func (h *Handler) renderPasscode(w http.ResponseWriter, r *http.Request, message string) {
	h.renderPage(w, r, http.StatusOK, "passcode", pageData{
		Title:      "Passcode required",
		FormAction: sessionPath(r.PathValue("linkToken")),
		Error:      message,
		View:       AvailabilityView{Notice: NoticeNotMedical},
	})
}

func (h *Handler) handleSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	profile := profileFromContext(ctx)
	sourceID := sourceFromContext(ctx)
	bucket := "passcode:" + profile.ID + ":" + sourceID

	if err := r.ParseForm(); err != nil {
		h.writeGeneric(w, r, http.StatusBadRequest)
		return
	}
	delay, err := h.store.PasscodeDelay(ctx, bucket, h.now())
	if err != nil {
		h.writeGeneric(w, r, http.StatusServiceUnavailable)
		return
	}
	if delay > 0 {
		_ = h.store.RecordAccess(ctx, profile.ID, EventThrottled, sourceID, h.now())
		w.Header().Set("Retry-After", retryAfterSeconds(delay))
		h.renderPage(w, r, http.StatusTooManyRequests, "passcode", pageData{
			Title:      "Passcode required",
			FormAction: sessionPath(r.PathValue("linkToken")),
			Error:      "Too many attempts. Wait a moment and try again.",
			View:       AvailabilityView{Notice: NoticeNotMedical},
		})
		return
	}

	passcode := r.PostFormValue("passcode")
	if err := h.store.VerifyPasscode(ctx, profile.ID, passcode); err != nil {
		if _, noteErr := h.store.NotePasscodeFailure(ctx, bucket, h.now()); noteErr != nil {
			h.writeGeneric(w, r, http.StatusServiceUnavailable)
			return
		}
		_ = h.store.RecordAccess(ctx, profile.ID, EventPasscodeRejected, sourceID, h.now())
		h.renderPage(w, r, http.StatusUnauthorized, "passcode", pageData{
			Title:      "Passcode required",
			FormAction: sessionPath(r.PathValue("linkToken")),
			Error:      "That passcode did not match.",
			View:       AvailabilityView{Notice: NoticeNotMedical},
		})
		return
	}

	if err := h.store.ClearPasscodeFailures(ctx, bucket); err != nil {
		h.writeGeneric(w, r, http.StatusServiceUnavailable)
		return
	}
	// Rotate: any session presented alongside this login is discarded rather
	// than reused, so a fixated cookie value cannot survive authentication.
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		_ = h.store.DeleteSession(ctx, cookie.Value)
	}
	token, err := h.store.CreateSession(ctx, profile, h.now())
	if err != nil {
		h.writeGeneric(w, r, http.StatusServiceUnavailable)
		return
	}
	_ = h.store.RecordAccess(ctx, profile.ID, EventPasscodeAccepted, sourceID, h.now())

	// MaxAge is derived from the handler's clock, not time.Now: mixing an
	// injected clock with the wall clock can produce a negative MaxAge, which
	// browsers read as "delete this cookie immediately".
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token.Session,
		Path:     "/",
		Expires:  token.ExpiresAt,
		MaxAge:   int(token.ExpiresAt.Sub(h.now()).Seconds()),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, pagePath(r.PathValue("linkToken")), http.StatusSeeOther)
}

// availabilityDTO is the exact public allowlist from docs/portal-design.md
// section 5. Adding a field widens the public boundary and must go through
// that review; TestAvailabilityDTOAllowlist fails otherwise.
type availabilityDTO struct {
	Version     int64       `json:"version"`
	Windows     []windowDTO `json:"windows"`
	GeneratedAt string      `json:"generatedAt"`
	HorizonEnd  string      `json:"horizonEnd"`
	Status      string      `json:"status"`
}

type windowDTO struct {
	StartAt string `json:"startAt"`
	EndAt   string `json:"endAt"`
	ZoneID  string `json:"zoneId"`
}

func (h *Handler) handleAvailability(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	profile := profileFromContext(ctx)
	_ = h.store.RecordAccess(ctx, profile.ID, EventAvailabilityRead, sourceFromContext(ctx), h.now())

	snapshot, err := h.store.ReadSnapshot(ctx, profile.ID)
	if err != nil {
		if !errors.Is(err, ErrNoSnapshot) {
			log.Printf("portal: snapshot unreadable for profile %s: %v", profile.ID, err)
		}
		writeJSON(w, http.StatusOK, availabilityDTO{
			Windows: []windowDTO{},
			Status:  StatusInsufficientData,
		})
		return
	}
	writeJSON(w, http.StatusOK, toAvailabilityDTO(snapshot, h.now()))
}

func toAvailabilityDTO(snapshot Snapshot, now time.Time) availabilityDTO {
	dto := availabilityDTO{
		Version:     snapshot.Version,
		Windows:     []windowDTO{},
		GeneratedAt: formatTime(snapshot.GeneratedAt),
		HorizonEnd:  formatTime(snapshot.HorizonEnd),
		Status:      snapshot.Status,
	}
	if dto.Status == "" {
		dto.Status = StatusInsufficientData
	}
	// Apply the same age rules the rendered page applies, so a JSON consumer
	// cannot present an out-of-date "awake now" the HTML would have withheld.
	if !snapshot.GeneratedAt.IsZero() && now.Sub(snapshot.GeneratedAt) >= UnavailableAfter {
		dto.Status = StatusInsufficientData
		return dto
	}
	if dto.Status != StatusAvailable {
		return dto
	}
	for _, window := range snapshot.Windows {
		if !window.EndAt.After(now) {
			continue
		}
		dto.Windows = append(dto.Windows, windowDTO{
			StartAt: formatTime(window.StartAt),
			EndAt:   formatTime(window.EndAt),
			ZoneID:  window.ZoneID,
		})
	}
	if len(dto.Windows) == 0 {
		dto.Status = StatusInsufficientData
	}
	return dto
}

// writeGeneric emits one uniform failure. The message never varies with the
// reason, so a caller cannot tell an unknown link from a revoked one.
func (h *Handler) writeGeneric(w http.ResponseWriter, r *http.Request, status int) {
	title, message := genericMessage(status)
	if wantsJSON(r) {
		writeJSON(w, status, map[string]string{"status": "unavailable", "message": message})
		return
	}
	h.renderPage(w, r, status, "generic", pageData{Title: title, Error: message})
}

func genericMessage(status int) (string, string) {
	switch status {
	case http.StatusTooManyRequests:
		return "Too many requests", "This link has received too many requests recently. Try again shortly."
	case http.StatusUnauthorized:
		return "Passcode required", "This page needs a passcode. Open the link again to enter it."
	case http.StatusForbidden:
		return "Request refused", "This request could not be accepted. Open the link directly and try again."
	default:
		// 410 and every unexpected status collapse here on purpose: unknown,
		// expired, and revoked links must be indistinguishable.
		return "Link unavailable", "This link is no longer available."
	}
}

func wantsJSON(r *http.Request) bool {
	if strings.HasSuffix(r.URL.Path, "/availability") {
		return true
	}
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json") && !strings.Contains(accept, "text/html")
}

func (h *Handler) renderPage(w http.ResponseWriter, r *http.Request, status int, name string, data pageData) {
	var buffer bytes.Buffer
	if err := pageTemplates.ExecuteTemplate(&buffer, name, data); err != nil {
		// Render into a buffer first so a template failure cannot emit a
		// half-written page with a 200 already committed.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("<!DOCTYPE html><title>Unavailable</title><p>This page is unavailable."))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buffer.Bytes())
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func pagePath(token string) string {
	return (&url.URL{Path: "/p/" + token}).String()
}

func sessionPath(token string) string {
	return (&url.URL{Path: "/p/" + token + "/session"}).String()
}
