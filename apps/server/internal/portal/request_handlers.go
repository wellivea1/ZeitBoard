package portal

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const requestCookieName = "__Host-zb_request"

// RequestView is the rendered form of a visitor's own request. It contains the
// visitor's own text — which they wrote and may re-read — and never anything
// about the owner beyond the decision itself.
type RequestView struct {
	ID            string
	StatusLabel   string
	StatusDetail  string
	WindowLabel   string
	DurationLabel string
	Handle        string
	Message       string
	DecidedLabel  string
	BeyondHorizon bool
	Warning       string
	Open          bool
}

// BeyondHorizonWarning is required on any request reaching past the forecast.
// The owner chose to allow those requests rather than cap them, so the honest
// alternative to refusing is saying plainly that the estimate does not run
// that far.
const BeyondHorizonWarning = "That date is further ahead than this estimate can reach, so there is no availability to check it against. You can still ask, and they can still answer."

func (h *Handler) handleCreateRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	profile := profileFromContext(ctx)

	if !profile.Grants.AllowRequests {
		h.writeGeneric(w, r, http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		h.renderRequestForm(w, r, "That form could not be read. Try again.")
		return
	}
	if !h.store.MatchesCSRF(sessionValueFrom(r), r.PostFormValue("csrf")) {
		// A session exists here, so the synchronizer token is available and
		// checked in addition to the same-origin attestation.
		h.writeGeneric(w, r, http.StatusForbidden)
		return
	}

	input, err := parseRequestForm(r, profile)
	if err != nil {
		h.renderRequestForm(w, r, humanRequestError(err))
		return
	}

	var horizonEnd time.Time
	if snapshot, snapErr := h.store.ReadSnapshot(ctx, profile.ID); snapErr == nil {
		horizonEnd = snapshot.HorizonEnd
	}
	validated, beyondHorizon, err := ValidateRequest(input, h.now(), horizonEnd)
	if err != nil {
		h.renderRequestForm(w, r, humanRequestError(err))
		return
	}

	created, err := h.store.CreateRequest(ctx, profile, sessionValueFrom(r), validated, beyondHorizon, h.now())
	if err != nil {
		if errors.Is(err, ErrRequestLimit) || errors.Is(err, ErrRequestInvalid) {
			h.renderRequestForm(w, r, humanRequestError(err))
			return
		}
		log.Printf("portal: request creation failed for profile %s: %v", profile.ID, err)
		h.writeGeneric(w, r, http.StatusServiceUnavailable)
		return
	}

	// Nudge the owner side to deliver now rather than on the next timer tick,
	// then re-read so the confirmation reports the state that is actually
	// true. If delivery failed, the request stays queued and says so.
	if h.notifyCreated != nil {
		h.notifyCreated()
	}
	stored := created.Request
	if refreshed, readErr := h.store.ReadRequest(ctx, profile.ID, created.Request.ID); readErr == nil {
		stored = refreshed
	}

	// The requester secret travels in the fragment, which browsers never send
	// to a server and proxies never log. The no-script path below also shows
	// it once as a recovery code.
	target := requestPath(r.PathValue("linkToken"), created.Request.ID) + "#s=" + url.QueryEscape(created.Secret)
	h.renderPage(w, r, http.StatusOK, "request-created", pageData{
		Title:         "Request sent",
		Request:       h.requestView(stored),
		RecoveryCode:  created.Secret,
		ContinueURL:   target,
		View:          AvailabilityView{Notice: NoticeNotMedical},
		LinkTokenPath: requestPath(r.PathValue("linkToken"), created.Request.ID),
	})
}

// handleRequestSession exchanges the requester secret for a request-scoped
// cookie. It is a POST so the secret never enters a URL the server sees.
func (h *Handler) handleRequestSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	profile := profileFromContext(ctx)
	requestID := r.PathValue("requestID")

	if err := r.ParseForm(); err != nil {
		h.writeGeneric(w, r, http.StatusBadRequest)
		return
	}
	secret := strings.TrimSpace(r.PostFormValue("secret"))
	token, err := h.store.AuthorizeRequestSecret(ctx, profile.ID, requestID, secret, h.now())
	if err != nil {
		// A wrong secret and an unknown request are the same response, so a
		// holder of the shared link cannot enumerate other visitors' requests.
		h.writeGeneric(w, r, http.StatusGone)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     requestCookieName,
		Value:    token.Session,
		Path:     "/",
		Expires:  token.ExpiresAt,
		MaxAge:   int(token.ExpiresAt.Sub(h.now()).Seconds()),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, requestPath(r.PathValue("linkToken"), requestID), http.StatusSeeOther)
}

func (h *Handler) handleRequestStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	profile := profileFromContext(ctx)
	requestID := r.PathValue("requestID")

	authorized, err := h.store.AuthorizesRequest(ctx, requestCookieValue(r), profile.ID, requestID, h.now())
	if err != nil || !authorized {
		// No proof of authorship: offer the recovery-code form rather than
		// confirming or denying that this request exists.
		h.renderPage(w, r, http.StatusOK, "request-claim", pageData{
			Title:         "Check a request",
			FormAction:    requestSessionPath(r.PathValue("linkToken"), requestID),
			View:          AvailabilityView{Notice: NoticeNotMedical},
			LinkTokenPath: requestPath(r.PathValue("linkToken"), requestID),
		})
		return
	}

	request, err := h.store.ReadRequest(ctx, profile.ID, requestID)
	if err != nil {
		h.writeGeneric(w, r, http.StatusGone)
		return
	}
	h.renderPage(w, r, http.StatusOK, "request-status", pageData{
		Title:   "Your request",
		Refresh: true,
		Request: h.requestView(request),
		View:    AvailabilityView{Notice: NoticeNotMedical},
	})
}

func (h *Handler) requestView(request Request) RequestView {
	location := windowLocation(request.ZoneID)
	view := RequestView{
		ID:            request.ID,
		WindowLabel:   describeDay(request.WindowStart, h.now(), location) + ", " + describeRange(request.WindowStart, request.WindowEnd, location),
		Handle:        request.Handle,
		Message:       request.Message,
		BeyondHorizon: request.BeyondHorizon,
	}
	if request.DurationMinutes > 0 {
		view.DurationLabel = fmt.Sprintf("%d minutes", request.DurationMinutes)
	}
	if request.BeyondHorizon {
		view.Warning = BeyondHorizonWarning
	}
	switch request.Status {
	case RequestQueued:
		view.Open = true
		view.StatusLabel = "Waiting to be delivered"
		// Deliberately not "sent": the portal has the request but has not yet
		// confirmed it reached the owner's queue, and saying otherwise would
		// be a claim the visitor cannot check.
		view.StatusDetail = "Your request is saved and is on its way to them."
	case RequestPending:
		view.Open = true
		view.StatusLabel = "Waiting for an answer"
		view.StatusDetail = "They have your request. You will see the answer here."
	case RequestApproved:
		view.StatusLabel = "Accepted"
		view.StatusDetail = "They picked a time inside the window you asked for."
		view.DecidedLabel = describeDay(request.DecidedStart, h.now(), location) + ", " +
			describeRange(request.DecidedStart, request.DecidedEnd, location)
	case RequestDeclined:
		// No reason is given, ever. A reason would disclose the owner's sleep
		// state, calendar, or health, none of which a visitor is owed.
		view.StatusLabel = "Declined"
		view.StatusDetail = "They could not take that time."
	default:
		view.StatusLabel = "Closed"
		view.StatusDetail = "This request is no longer open."
	}
	return view
}

func parseRequestForm(r *http.Request, profile Profile) (RequestInput, error) {
	zoneID := strings.TrimSpace(r.PostFormValue("zone_id"))
	if zoneID == "" {
		zoneID = "UTC"
	}
	location, err := time.LoadLocation(zoneID)
	if err != nil {
		return RequestInput{}, fmt.Errorf("%w: unknown time zone", ErrRequestInvalid)
	}
	start, err := parseLocalInput(r.PostFormValue("window_start"), location)
	if err != nil {
		return RequestInput{}, err
	}
	end, err := parseLocalInput(r.PostFormValue("window_end"), location)
	if err != nil {
		return RequestInput{}, err
	}
	duration := 0
	if raw := strings.TrimSpace(r.PostFormValue("duration_minutes")); raw != "" {
		parsed, convErr := strconv.Atoi(raw)
		if convErr != nil {
			return RequestInput{}, fmt.Errorf("%w: length must be a number of minutes", ErrRequestInvalid)
		}
		duration = parsed
	}
	_ = profile
	return RequestInput{
		WindowStart:     start,
		WindowEnd:       end,
		ZoneID:          zoneID,
		DurationMinutes: duration,
		Handle:          r.PostFormValue("handle"),
		Message:         r.PostFormValue("message"),
	}, nil
}

// parseLocalInput reads an HTML datetime-local value in the visitor's chosen
// zone. A local time that does not exist (spring-forward) or is ambiguous
// (fall-back) is rejected rather than silently resolved, because guessing
// which of two instants someone meant is how scheduling bugs are born.
func parseLocalInput(value string, location *time.Location) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("%w: a start and end are required", ErrRequestInvalid)
	}
	layouts := []string{"2006-01-02T15:04", "2006-01-02T15:04:05"}
	var parsed time.Time
	var err error
	for _, layout := range layouts {
		parsed, err = time.ParseInLocation(layout, value, location)
		if err == nil {
			break
		}
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: that time could not be read", ErrRequestInvalid)
	}
	if parsed.Format("2006-01-02T15:04") != value[:min(len(value), 16)] {
		return time.Time{}, fmt.Errorf("%w: that local time does not exist on that date", ErrRequestInvalid)
	}
	return parsed.UTC(), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// humanRequestError surfaces the caller's own mistake and nothing else. The
// wrapped sentinel is stripped so a visitor sees guidance, not an error chain.
func humanRequestError(err error) string {
	message := err.Error()
	for _, prefix := range []string{ErrRequestInvalid.Error() + ": ", ErrRequestLimit.Error() + ": "} {
		if strings.HasPrefix(message, prefix) {
			return strings.ToUpper(message[len(prefix):len(prefix)+1]) + message[len(prefix)+1:] + "."
		}
	}
	return "That request could not be accepted."
}

func (h *Handler) renderRequestForm(w http.ResponseWriter, r *http.Request, message string) {
	ctx := r.Context()
	profile := profileFromContext(ctx)
	snapshot, err := h.store.ReadSnapshot(ctx, profile.ID)
	if err != nil {
		snapshot = Snapshot{}
	}
	status := http.StatusOK
	if message != "" {
		status = http.StatusBadRequest
	}
	h.renderPage(w, r, status, "request-form", pageData{
		Title:      "Ask for a time",
		Error:      message,
		FormAction: requestsPath(r.PathValue("linkToken")),
		CSRFToken:  h.store.CSRFToken(sessionValueFrom(r)),
		View:       BuildView(snapshot, h.now()),
		MinLocal:   h.now().Format("2006-01-02T15:04"),
	})
}

func (h *Handler) handleRequestForm(w http.ResponseWriter, r *http.Request) {
	if !profileFromContext(r.Context()).Grants.AllowRequests {
		h.writeGeneric(w, r, http.StatusForbidden)
		return
	}
	h.renderRequestForm(w, r, "")
}

func sessionValueFrom(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func requestCookieValue(r *http.Request) string {
	cookie, err := r.Cookie(requestCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func requestsPath(linkToken string) string {
	return (&url.URL{Path: "/p/" + linkToken + "/requests"}).String()
}

func requestPath(linkToken, requestID string) string {
	return (&url.URL{Path: "/p/" + linkToken + "/requests/" + requestID}).String()
}

func requestSessionPath(linkToken, requestID string) string {
	return (&url.URL{Path: "/p/" + linkToken + "/requests/" + requestID + "/session"}).String()
}
