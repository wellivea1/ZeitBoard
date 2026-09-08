package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"non24.app/core/sleepv1"
	"non24.app/server/internal/projection"
	"non24.app/server/internal/store"
)

func (s *Server) handleCompanion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	records, cursor, err := s.store.SleepSnapshot(r.Context())
	if errors.Is(err, store.ErrCompanionHistoryLimit) {
		writeJSON(w, http.StatusOK, projection.RefusedCompanion(cursor, s.now(), "history_limit", "The saved history exceeds the companion's supported snapshot size."))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "companion projection failed")
		return
	}
	var observations []sleepv1.Observation
	var corrections []sleepv1.Correction
	containsSynthetic := false
	for _, record := range records {
		switch record.Kind {
		case "observation":
			var kind struct {
				Kind string `json:"kind"`
			}
			if err = json.Unmarshal(record.Payload, &kind); err == nil && kind.Kind == sleepv1.KindEpisode {
				var observation sleepv1.Observation
				observation, err = sleepv1.DecodeObservation(record.Payload)
				observations = append(observations, observation)
				containsSynthetic = containsSynthetic || observation.Provenance.AcquisitionMethod == sleepv1.AcquisitionSynthetic
			}
		case "correction":
			var correction sleepv1.Correction
			correction, err = sleepv1.DecodeCorrection(record.Payload)
			corrections = append(corrections, correction)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "companion records could not be read")
			return
		}
	}
	sessions, err := sleepv1.Fold(observations, corrections)
	if err != nil {
		writeJSON(w, http.StatusOK, projection.RefusedCompanion(cursor, s.now(), "correction_review_required", "Saved sleep corrections need review before a forecast can be shown."))
		return
	}
	sessions, err = sleepv1.ResolveOverlaps(sessions)
	if err != nil {
		writeJSON(w, http.StatusOK, projection.RefusedCompanion(cursor, s.now(), "conflicting_observations", "Conflicting sleep observations need review."))
		return
	}
	response, err := s.projectionService().Companion(r.Context(), sessions, cursor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "companion projection failed")
		return
	}
	response.ContainsSyntheticData = containsSynthetic
	writeJSON(w, http.StatusOK, response)
}
