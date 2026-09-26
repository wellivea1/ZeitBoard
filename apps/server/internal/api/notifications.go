package api

import (
	"net/http"
	"strconv"
	"time"

	syncmodel "non24.app/server/internal/sync"
)

// The notification feed (C6). An enrolled device that has chosen to be told
// about time requests reads events after its own cursor and words its own
// notifications; nothing a visitor wrote, and no label, crosses this route.

type notificationDTO struct {
	EventID   string    `json:"eventId"`
	Kind      string    `json:"kind"`
	Subject   string    `json:"subject"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type notificationFeedResponse struct {
	SchemaVersion string            `json:"schema_version"`
	Events        []notificationDTO `json:"events"`
	// Cursor is where the next read starts. A device that has just turned
	// notifications on starts from here without being told about the past.
	Cursor  int64 `json:"cursor"`
	HasMore bool  `json:"hasMore"`
}

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if query.Get("after") == "latest" {
		// A device turning notifications on starts at the head: it is told
		// about what happens next, not about the past week.
		head, err := s.store.NotificationHead(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "notification feed failed")
			return
		}
		writeJSON(w, http.StatusOK, notificationFeedResponse{
			SchemaVersion: syncmodel.SchemaVersion,
			Events:        []notificationDTO{},
			Cursor:        head,
		})
		return
	}
	after, err := strconv.ParseInt(query.Get("after"), 10, 64)
	if query.Get("after") == "" {
		after, err = 0, nil
	}
	if err != nil || after < 0 {
		writeError(w, http.StatusBadRequest, "invalid notification cursor")
		return
	}
	limit := 50
	if raw := query.Get("limit"); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed < 1 || parsed > 100 {
			writeError(w, http.StatusBadRequest, "invalid notification limit")
			return
		}
		limit = parsed
	}
	events, hasMore, err := s.store.NotificationsAfter(r.Context(), after, limit, s.now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "notification feed failed")
		return
	}
	response := notificationFeedResponse{
		SchemaVersion: syncmodel.SchemaVersion,
		Events:        make([]notificationDTO, 0, len(events)),
		Cursor:        after,
		HasMore:       hasMore,
	}
	for _, event := range events {
		response.Events = append(response.Events, notificationDTO{
			EventID:   event.EventID,
			Kind:      event.Kind,
			Subject:   event.Subject,
			CreatedAt: event.CreatedAt,
			ExpiresAt: event.ExpiresAt,
		})
		response.Cursor = event.Seq
	}
	if len(events) == 0 {
		// Nothing new: move the cursor past anything that expired unread.
		head, err := s.store.NotificationHead(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "notification feed failed")
			return
		}
		if head > response.Cursor {
			response.Cursor = head
		}
	}
	writeJSON(w, http.StatusOK, response)
}
