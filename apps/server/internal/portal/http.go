package portal

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

const (
	// maxPublicBodyBytes caps a request before any decoding happens.
	maxPublicBodyBytes = 4 << 10

	// sessionCookieName uses the __Host- prefix, which browsers only accept
	// with Secure, Path=/, and no Domain attribute.
	sessionCookieName = "__Host-zb_portal"

	// resolutionFloor is applied to link resolution whether it succeeds or
	// fails, so an enumerator cannot separate "real link" from "unknown
	// link" by response time. It is a bounded floor, not a constant-time
	// guarantee; docs/portal-design.md section 4 states that limit.
	resolutionFloor = 120 * time.Millisecond
)

type profileContextKey struct{}
type sessionContextKey struct{}

// Handler serves the public portal. It holds a portal store and nothing else:
// there is no field through which a private record could be reached.
type Handler struct {
	store           *Store
	now             func() time.Time
	publicOrigin    string
	resolutionFloor time.Duration
}

type HandlerConfig struct {
	Store *Store
	Now   func() time.Time

	// PublicOrigin is the exact scheme://host[:port] the portal is served
	// on. Mutating requests must carry it as Origin.
	PublicOrigin string

	// ResolutionFloor overrides the enumeration timing floor. Values at or
	// below zero use the package default, so a caller cannot disable the
	// floor by leaving the field unset; tests shorten it explicitly.
	ResolutionFloor time.Duration
}

func NewHandler(cfg HandlerConfig) (*Handler, error) {
	if cfg.Store == nil {
		return nil, errors.New("portal handler requires a store")
	}
	origin := strings.TrimSpace(strings.TrimSuffix(cfg.PublicOrigin, "/"))
	if origin == "" {
		return nil, errors.New("portal handler requires a public origin")
	}
	now := cfg.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	floor := cfg.ResolutionFloor
	if floor <= 0 {
		floor = resolutionFloor
	}
	return &Handler{store: cfg.Store, now: now, publicOrigin: origin, resolutionFloor: floor}, nil
}

// Routes returns the public mux. It is mounted only when the portal is
// enabled; when disabled the daemon never calls this and no /p/ route exists.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /p/assets/portal.css", h.baseChain(http.HandlerFunc(h.handleStylesheet)))
	mux.Handle("GET /p/{linkToken}", h.linkChain(http.HandlerFunc(h.handlePage)))
	mux.Handle("POST /p/{linkToken}/session", h.linkChain(h.requireOrigin(http.HandlerFunc(h.handleSession))))
	mux.Handle("GET /p/{linkToken}/availability", h.linkChain(h.requireSession(http.HandlerFunc(h.handleAvailability))))
	return mux
}

// baseChain applies the caps and headers that every public response needs,
// including responses that never resolve a link.
func (h *Handler) baseChain(next http.Handler) http.Handler {
	return h.limitBody(h.securityHeaders(next))
}

// linkChain is the full public order from docs/portal-design.md section 3:
// size cap, security headers, source throttling, link resolution, then the
// handler. The passcode-session gate is applied per route because the page
// itself must render for an unauthenticated visitor.
func (h *Handler) linkChain(next http.Handler) http.Handler {
	return h.limitBody(h.securityHeaders(h.throttle(h.resolveLink(next))))
}

func (h *Handler) limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxPublicBodyBytes)
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("Cache-Control", "no-store, max-age=0")
		header.Set("Content-Security-Policy",
			"default-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'; "+
				"img-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'")
		header.Set("Referrer-Policy", "no-referrer")
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		header.Set("Cross-Origin-Resource-Policy", "same-origin")
		// Search engines must never index a share link.
		header.Set("X-Robots-Tag", "noindex, nofollow, noarchive")
		next.ServeHTTP(w, r)
	})
}

// throttle applies the persisted per-source read limit before any expensive
// work. It runs before link resolution and therefore before the passcode KDF,
// which is what keeps argon2id from becoming a memory-exhaustion lever.
func (h *Handler) throttle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		sourceID, err := h.store.SourceID(ctx, r.RemoteAddr)
		if err != nil {
			h.writeGeneric(w, r, http.StatusServiceUnavailable)
			return
		}
		allowed, retryAfter, err := h.store.Allow(ctx, "read:"+sourceID, ReadLimitPerHour, ReadLimitWindow, h.now())
		if err != nil {
			h.writeGeneric(w, r, http.StatusServiceUnavailable)
			return
		}
		if !allowed {
			_ = h.store.RecordAccess(ctx, "", EventThrottled, sourceID, h.now())
			w.Header().Set("Retry-After", retryAfterSeconds(retryAfter))
			h.writeGeneric(w, r, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, sourceContextKey{}, sourceID)))
	})
}

type sourceContextKey struct{}

func sourceFromContext(ctx context.Context) string {
	value, _ := ctx.Value(sourceContextKey{}).(string)
	return value
}

// resolveLink turns the path token into a profile. Unknown, expired, and
// revoked links are indistinguishable here because ResolveLink returns one
// error for all three; this handler could not leak the difference if it tried.
func (h *Handler) resolveLink(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := h.now()
		ctx := r.Context()
		token := r.PathValue("linkToken")
		profile, err := h.store.ResolveLink(ctx, token, h.now())
		h.holdFloor(started)
		if err != nil {
			if errors.Is(err, ErrLinkNotUsable) {
				_ = h.store.RecordAccess(ctx, "", EventLinkRejected, sourceFromContext(ctx), h.now())
				h.writeGeneric(w, r, http.StatusGone)
				return
			}
			h.writeGeneric(w, r, http.StatusServiceUnavailable)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, profileContextKey{}, profile)))
	})
}

// holdFloor pads a resolution to the timing floor. It sleeps rather than
// busy-waits so a flood costs memory for a parked goroutine, not CPU.
func (h *Handler) holdFloor(started time.Time) {
	elapsed := h.now().Sub(started)
	if elapsed < h.resolutionFloor {
		time.Sleep(h.resolutionFloor - elapsed)
	}
}

// requireSession gates authenticated reads. The session must belong to the
// same profile the path token resolved to, so holding a session for one link
// grants nothing on another.
func (h *Handler) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		profile := profileFromContext(r.Context())
		session, ok := h.sessionFor(r, profile)
		if !ok {
			h.writeGeneric(w, r, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionContextKey{}, session)))
	})
}

func (h *Handler) sessionFor(r *http.Request, profile Profile) (Session, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return Session{}, false
	}
	session, err := h.store.ResolveSession(r.Context(), cookie.Value, h.now())
	if err != nil {
		return Session{}, false
	}
	if session.ProfileID != profile.ID {
		return Session{}, false
	}
	return session, true
}

// requireOrigin is the pre-session CSRF defence. A passcode POST cannot carry
// a synchronizer token yet, so browser attestation that the request came from
// this origin is what stops a third party from silently authenticating a
// visitor's browser to a link.
func (h *Handler) requireOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.isSameOriginMutation(r) {
			h.writeGeneric(w, r, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isSameOriginMutation decides whether a mutating request really came from a
// page this server served.
//
// Origin alone is not sufficient here. These responses set
// Referrer-Policy: no-referrer, and per the Fetch standard that makes a
// browser send `Origin: null` on a same-origin form POST rather than the real
// origin. Relaxing the referrer policy to recover Origin would put the link
// token into Referer headers, which section 9 of the design specifically
// avoids. Sec-Fetch-Site is unaffected by referrer policy and cannot be set by
// page script, so it is the primary signal; an exact Origin match remains the
// fallback for clients that predate Fetch Metadata.
func (h *Handler) isSameOriginMutation(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	// A present, non-null Origin must match no matter what else is claimed.
	if origin != "" && origin != "null" && !strings.EqualFold(origin, h.publicOrigin) {
		return false
	}
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin":
		return true
	case "":
		// No Fetch Metadata at all: fall back to a real Origin match. A
		// request with neither header is refused, so this fails closed.
		return origin != "" && strings.EqualFold(origin, h.publicOrigin)
	default:
		// cross-site, same-site, and none are all refused: a subdomain is not
		// this origin, and a POST is never user-initiated navigation.
		return false
	}
}

func profileFromContext(ctx context.Context) Profile {
	profile, _ := ctx.Value(profileContextKey{}).(Profile)
	return profile
}

func sessionFromContext(ctx context.Context) Session {
	session, _ := ctx.Value(sessionContextKey{}).(Session)
	return session
}

func retryAfterSeconds(value time.Duration) string {
	seconds := int(value.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	return itoa(seconds)
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 12)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}
