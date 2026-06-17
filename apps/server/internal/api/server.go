package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"non24.app/server/internal/auth"
	"non24.app/server/internal/store"
	syncmodel "non24.app/server/internal/sync"
)

const maxDeviceBodyBytes = 16 * 1024

type Server struct {
	store            *store.Store
	enrollmentSecret string
	now              func() time.Time
}

type deviceContextKey struct{}

func New(st *store.Store, enrollmentSecret string) *Server {
	return &Server{
		store:            st,
		enrollmentSecret: enrollmentSecret,
		now:              func() time.Time { return time.Now().UTC() },
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /v1/devices", s.handleDevices)
	mux.Handle("POST /v1/sync/push", s.requireDevice(http.HandlerFunc(s.handlePush)))
	mux.Handle("GET /v1/sync/pull", s.requireDevice(http.HandlerFunc(s.handlePull)))
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type registerDeviceRequest struct {
	EnrollmentSecret string `json:"enrollmentSecret"`
	Label            string `json:"label"`
}

type registerDeviceResponse struct {
	SchemaVersion string `json:"schema_version"`
	DeviceID      string `json:"deviceId"`
	Token         string `json:"token"`
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	var req registerDeviceRequest
	if err := decodeBody(w, r, maxDeviceBodyBytes, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	if !auth.SecretMatches(req.EnrollmentSecret, s.enrollmentSecret) {
		writeError(w, http.StatusForbidden, "invalid enrollment secret")
		return
	}
	if len(req.Label) > 80 {
		writeError(w, http.StatusBadRequest, "device label is too long")
		return
	}
	deviceID, err := auth.NewDeviceID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "device registration failed")
		return
	}
	token, err := auth.NewToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "device registration failed")
		return
	}
	if err := s.store.RegisterDevice(r.Context(), deviceID, req.Label, auth.HashToken(token), s.now()); err != nil {
		writeError(w, http.StatusInternalServerError, "device registration failed")
		return
	}
	writeJSON(w, http.StatusCreated, registerDeviceResponse{
		SchemaVersion: syncmodel.SchemaVersion,
		DeviceID:      deviceID,
		Token:         token,
	})
}

func (s *Server) requireDevice(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := auth.BearerToken(r.Header.Get("Authorization"))
		if !ok {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		device, err := s.store.FindDeviceByTokenHash(r.Context(), auth.HashToken(token))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		ctx := context.WithValue(r.Context(), deviceContextKey{}, device)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) handlePush(w http.ResponseWriter, r *http.Request) {
	device := deviceFromContext(r.Context())
	var req syncmodel.PushRequest
	if err := decodeBody(w, r, syncmodel.MaxRequestBytes, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := syncmodel.ValidatePushRequest(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid sync batch")
		return
	}
	cursor, accepted, err := s.store.Append(r.Context(), device.ID, req.Records)
	if errors.Is(err, store.ErrRecordConflict) {
		writeError(w, http.StatusConflict, "record id conflict")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "sync push failed")
		return
	}
	writeJSON(w, http.StatusOK, syncmodel.PushResponse{
		SchemaVersion: syncmodel.SchemaVersion,
		Cursor:        cursor,
		Accepted:      accepted,
	})
}

func (s *Server) handlePull(w http.ResponseWriter, r *http.Request) {
	since := int64(0)
	if raw := r.URL.Query().Get("since"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, "invalid cursor")
			return
		}
		since = parsed
	}
	records, cursor, err := s.store.Pull(r.Context(), since, syncmodel.MaxPullRecords)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "sync pull failed")
		return
	}
	writeJSON(w, http.StatusOK, syncmodel.PullResponse{
		SchemaVersion: syncmodel.SchemaVersion,
		Cursor:        cursor,
		Records:       records,
	})
}

func deviceFromContext(ctx context.Context) store.Device {
	device, _ := ctx.Value(deviceContextKey{}).(store.Device)
	return device
}

func decodeBody(w http.ResponseWriter, r *http.Request, maxBytes int64, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("request body must contain a single JSON value")
	}
	return nil
}

func writeDecodeError(w http.ResponseWriter, err error) {
	if strings.Contains(err.Error(), "request body too large") {
		writeError(w, http.StatusRequestEntityTooLarge, "request body is too large")
		return
	}
	writeError(w, http.StatusBadRequest, "invalid request body")
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
