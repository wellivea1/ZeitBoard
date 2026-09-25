package api

import (
	"errors"
	"net/http"

	"non24.app/core/sleepv1"
	"non24.app/server/internal/projection"
	"non24.app/server/internal/store"
)

func (s *Server) handleSleepReview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id := r.PathValue("id")
	if len(id) > 64 || r.URL.RawQuery != "" {
		writeError(w, http.StatusBadRequest, "invalid sleep review request")
		return
	}
	records, cursor, err := s.store.SleepReviewSnapshot(r.Context(), id)
	if errors.Is(err, store.ErrCompanionHistoryLimit) {
		writeError(w, http.StatusConflict, "sleep review exceeds the supported history size")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "sleep review could not be read")
		return
	}
	var observation *sleepv1.Observation
	corrections := []sleepv1.Correction{}
	for _, record := range records {
		switch record.Kind {
		case "observation":
			value, decodeErr := sleepv1.DecodeObservation(record.Payload)
			if decodeErr == nil {
				observation = &value
			} else {
				err = decodeErr
			}
		case "correction":
			value, decodeErr := sleepv1.DecodeCorrection(record.Payload)
			if decodeErr == nil {
				corrections = append(corrections, value)
			} else {
				err = decodeErr
			}
		}
	}
	if observation == nil {
		writeError(w, http.StatusNotFound, "sleep observation is unavailable")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "sleep review could not be read")
		return
	}
	review, err := sleepv1.Review(*observation, corrections)
	if err != nil {
		writeError(w, http.StatusConflict, "sleep records could not be prepared for review")
		return
	}
	writeJSON(w, http.StatusOK, projection.SleepReview(review, cursor, s.now()))
}
