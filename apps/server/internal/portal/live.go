package portal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// The live layer (portal-design sections 1, 3 and 8). An open page learns that
// its link's projection changed without asking every few seconds: the stream
// carries only a version and its freshness, and the page then fetches itself
// through the ordinary authenticated route. Streams are bounded per session,
// per link and in total; a page that cannot open one polls instead.
const (
	maxStreamsPerSession = 2
	maxStreamsPerProfile = 20
	maxStreamsTotal      = 200

	// streamHeartbeat keeps proxies from closing an idle stream; the design
	// requires one at least every 20 seconds.
	streamHeartbeat = 15 * time.Second

	// streamLifetime recycles a stream so a session that expires, or a
	// daemon that is draining, never holds one open indefinitely. The
	// browser reconnects on its own.
	streamLifetime = 30 * time.Minute

	// streamWriteDeadline replaces the daemon's 30-second response write
	// timeout for each event, which would otherwise kill every stream.
	streamWriteDeadline = 10 * time.Second
)

// liveHub tells open streams that something about their link changed. It
// carries no content: a stream that is told re-reads its own state.
type liveHub struct {
	mu        sync.Mutex
	byProfile map[string]map[chan struct{}]string
	bySession map[string]int
	total     int

	// closing ends every stream when the daemon shuts down; otherwise a
	// graceful shutdown would wait out each stream's lifetime.
	closing   chan struct{}
	closeOnce sync.Once
}

func newLiveHub() *liveHub {
	return &liveHub{
		byProfile: map[string]map[chan struct{}]string{},
		bySession: map[string]int{},
		closing:   make(chan struct{}),
	}
}

func (h *liveHub) close() {
	h.closeOnce.Do(func() { close(h.closing) })
}

// subscribe registers a stream, or refuses when a bound is reached.
func (h *liveHub) subscribe(profileID, sessionKey string) (<-chan struct{}, func(), bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.total >= maxStreamsTotal || h.bySession[sessionKey] >= maxStreamsPerSession ||
		len(h.byProfile[profileID]) >= maxStreamsPerProfile {
		return nil, nil, false
	}
	signal := make(chan struct{}, 1)
	if h.byProfile[profileID] == nil {
		h.byProfile[profileID] = map[chan struct{}]string{}
	}
	h.byProfile[profileID][signal] = sessionKey
	h.bySession[sessionKey]++
	h.total++
	var once sync.Once
	release := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			delete(h.byProfile[profileID], signal)
			if len(h.byProfile[profileID]) == 0 {
				delete(h.byProfile, profileID)
			}
			if h.bySession[sessionKey]--; h.bySession[sessionKey] <= 0 {
				delete(h.bySession, sessionKey)
			}
			h.total--
		})
	}
	return signal, release, true
}

// notify wakes every stream on a link. A stream already holding an unread
// signal needs no second one: it re-reads the latest state either way.
func (h *liveHub) notify(profileID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for signal := range h.byProfile[profileID] {
		select {
		case signal <- struct{}{}:
		default:
		}
	}
}

// streamState is everything an event may say: which projection is current
// and how old it is. Windows never travel on the stream.
type streamState struct {
	Version   int64  `json:"version"`
	Freshness string `json:"freshness"`
}

func freshnessOf(snapshot Snapshot, now time.Time) string {
	switch {
	case snapshot.GeneratedAt.IsZero():
		return "none"
	case now.Sub(snapshot.GeneratedAt) >= UnavailableAfter:
		return "unavailable"
	case now.Sub(snapshot.GeneratedAt) >= StaleAfter:
		return "stale"
	default:
		return "current"
	}
}

func (h *Handler) handleEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	profile := profileFromContext(ctx)
	session := sessionFromContext(ctx)
	signal, release, ok := h.store.live.subscribe(profile.ID, session.Key)
	if !ok {
		// The page falls back to polling; a 429 tells EventSource to stop.
		w.Header().Set("Retry-After", "60")
		h.writeGeneric(w, r, http.StatusTooManyRequests)
		return
	}
	defer release()

	controller := http.NewResponseController(w)
	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	// Reverse proxies must pass events through as they are written.
	header.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	write := func(chunk string) bool {
		_ = controller.SetWriteDeadline(time.Now().Add(streamWriteDeadline))
		if _, err := fmt.Fprint(w, chunk); err != nil {
			return false
		}
		return controller.Flush() == nil
	}
	sendState := func() bool {
		now := h.now()
		if _, expired, revoked, err := h.store.LookupProfile(ctx, profile.ID, now); err != nil || expired || revoked {
			// The link stopped working while the page was open. The page
			// reloads and gets what every visitor of a dead link gets.
			write("event: gone\ndata: {}\n\n")
			return false
		}
		if s, err := h.store.ResolveSessionByKey(ctx, session.Key, now); err != nil || s.ProfileID != profile.ID {
			write("event: gone\ndata: {}\n\n")
			return false
		}
		snapshot, err := h.store.ReadSnapshot(ctx, profile.ID)
		if err != nil {
			snapshot = Snapshot{}
		}
		data, _ := json.Marshal(streamState{Version: snapshot.Version, Freshness: freshnessOf(snapshot, now)})
		return write("event: state\ndata: " + string(data) + "\n\n")
	}

	// The browser waits this long before reconnecting after a recycle.
	if !write("retry: 5000\n\n") || !sendState() {
		return
	}
	heartbeat := time.NewTicker(streamHeartbeat)
	defer heartbeat.Stop()
	lifetime := time.NewTimer(streamLifetime)
	defer lifetime.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.store.live.closing:
			return
		case <-lifetime.C:
			return
		case <-signal:
			if !sendState() {
				return
			}
		case <-heartbeat.C:
			if !write(": heartbeat\n\n") {
				return
			}
		}
	}
}
