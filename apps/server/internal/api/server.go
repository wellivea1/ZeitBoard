package api

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"non24.app/core/domain"
	"non24.app/server/internal/assistant"
	"non24.app/server/internal/auth"
	"non24.app/server/internal/projection"
	"non24.app/server/internal/provider"
	"non24.app/server/internal/readmodel"
	"non24.app/server/internal/store"
	syncmodel "non24.app/server/internal/sync"
)

const (
	maxDeviceBodyBytes    = 16 * 1024
	proposalCursorVersion = byte(2)
)

type Server struct {
	store            *store.Store
	enrollmentSecret string
	assistant        *assistant.Service
	portal           *portalAdmin
	now              func() time.Time
}

type deviceContextKey struct{}

type Option func(*Server)

func WithProvider(llm provider.LLM, status provider.Status) Option {
	return func(s *Server) {
		s.assistant = assistant.New(llm, status, s.store)
	}
}

func New(st *store.Store, enrollmentSecret string, opts ...Option) *Server {
	server := &Server{
		store:            st,
		enrollmentSecret: enrollmentSecret,
		now:              func() time.Time { return time.Now().UTC() },
	}
	server.assistant = assistant.New(provider.DisabledClient{}, provider.DisabledClient{}.Status(), st)
	for _, opt := range opts {
		opt(server)
	}
	return server
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /v1/devices", s.handleDevices)
	mux.Handle("GET /v1/devices", s.requireDevice(http.HandlerFunc(s.handleListDevices)))
	mux.Handle("POST /v1/devices/{id}/revoke", s.requireDevice(http.HandlerFunc(s.handleRevokeDevice)))
	mux.Handle("GET /v1/status", s.requireDevice(http.HandlerFunc(s.handleStatus)))
	mux.Handle("POST /v1/assistant/message", s.requireDevice(http.HandlerFunc(s.handleAssistantMessage)))
	mux.Handle("GET /v1/proposals", s.requireDevice(http.HandlerFunc(s.handleListProposals)))
	mux.Handle("POST /v1/proposals", s.requireDevice(http.HandlerFunc(s.handleCreateProposal)))
	mux.Handle("POST /v1/proposals/{id}/decision", s.requireDevice(http.HandlerFunc(s.handleProposalDecision)))
	mux.Handle("GET /v1/overview", s.requireDevice(http.HandlerFunc(s.handleOverview)))
	mux.Handle("GET /v1/rhythm", s.requireDevice(http.HandlerFunc(s.handleRhythm)))
	mux.Handle("GET /v1/accuracy", s.requireDevice(http.HandlerFunc(s.handleAccuracy)))
	mux.Handle("POST /v1/sync/push", s.requireDevice(http.HandlerFunc(s.handlePush)))
	mux.Handle("GET /v1/sync/pull", s.requireDevice(http.HandlerFunc(s.handlePull)))
	mux.Handle("POST /v1/sync/erase", s.requireDevice(http.HandlerFunc(s.handleErase)))
	if s.portal != nil {
		// Registered only when the portal is configured. With the portal off
		// there is no sharing route to probe, on the owner surface or the
		// public one.
		mux.Handle("GET /v1/portal/profiles", s.requireDevice(http.HandlerFunc(s.handleListPortalProfiles)))
		mux.Handle("POST /v1/portal/profiles", s.requireDevice(http.HandlerFunc(s.handleCreatePortalProfile)))
		mux.Handle("POST /v1/portal/profiles/{id}/revoke", s.requireDevice(http.HandlerFunc(s.handleRevokePortalProfile)))
		mux.Handle("POST /v1/portal/profiles/{id}/erase", s.requireDevice(http.HandlerFunc(s.handleErasePortalProfile)))
	}
	return mux
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := s.store.ListDevices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "device list failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": syncmodel.SchemaVersion,
		"devices":        devices,
	})
}

func (s *Server) handleRevokeDevice(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "device id is required")
		return
	}
	if err := s.store.RevokeDevice(r.Context(), id, s.now()); err != nil {
		if errors.Is(err, store.ErrDeviceNotFound) {
			writeError(w, http.StatusNotFound, "device not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "device revoke failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"schema_version": syncmodel.SchemaVersion, "status": "revoked"})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": syncmodel.SchemaVersion,
		"assistant":      s.assistant.Status(),
	})
}

func (s *Server) handleAssistantMessage(w http.ResponseWriter, r *http.Request) {
	device := deviceFromContext(r.Context())
	var req assistant.MessageRequest
	if err := decodeBody(w, r, syncmodel.MaxRequestBytes, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	resp, err := s.assistant.HandleMessage(r.Context(), device, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid assistant request")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

type proposalListPagination struct {
	Limit      int    `json:"limit"`
	HasMore    bool   `json:"hasMore"`
	NextCursor string `json:"nextCursor,omitempty"`
}

type proposalListResponse struct {
	SchemaVersion string                 `json:"schema_version"`
	Proposals     []store.ProposalRecord `json:"proposals"`
	Pagination    proposalListPagination `json:"pagination"`
}

func (s *Server) handleListProposals(w http.ResponseWriter, r *http.Request) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid proposal list query")
		return
	}
	cursor, limit, err := parseProposalListQuery(query)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid proposal list query")
		return
	}
	listTime := s.now().UTC()
	if cursor.AfterRowID > 0 && cursor.AsOf.After(listTime) {
		writeError(w, http.StatusBadRequest, "invalid proposal list query")
		return
	}
	page, err := s.store.ListProposalPage(r.Context(), cursor, limit, listTime)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "proposal list failed")
		return
	}
	pagination := proposalListPagination{Limit: limit, HasMore: page.HasMore}
	if page.HasMore {
		pagination.NextCursor = encodeProposalCursor(page.NextCursor)
	}
	writeJSON(w, http.StatusOK, proposalListResponse{
		SchemaVersion: syncmodel.SchemaVersion,
		Proposals:     page.Records,
		Pagination:    pagination,
	})
}

func parseProposalListQuery(query map[string][]string) (store.ProposalPageCursor, int, error) {
	for key, values := range query {
		switch key {
		case "cursor", "limit":
		default:
			return store.ProposalPageCursor{}, 0, errors.New("unsupported proposal list parameter")
		}
		if len(values) != 1 || values[0] == "" {
			return store.ProposalPageCursor{}, 0, errors.New("proposal list parameters must have one value")
		}
	}

	cursor := store.ProposalPageCursor{}
	if values, ok := query["cursor"]; ok {
		decoded, err := decodeProposalCursor(values[0])
		if err != nil {
			return store.ProposalPageCursor{}, 0, err
		}
		cursor = decoded
	}

	limit := store.DefaultProposalPageLimit
	if values, ok := query["limit"]; ok {
		raw := values[0]
		for _, digit := range raw {
			if digit < '0' || digit > '9' {
				return store.ProposalPageCursor{}, 0, errors.New("proposal page limit must be decimal")
			}
		}
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > store.MaxProposalPageLimit || strconv.Itoa(parsed) != raw {
			return store.ProposalPageCursor{}, 0, errors.New("proposal page limit is out of range")
		}
		limit = parsed
	}
	return cursor, limit, nil
}

func encodeProposalCursor(cursor store.ProposalPageCursor) string {
	var raw [26]byte
	raw[0] = proposalCursorVersion
	if cursor.Active {
		raw[1] = 1
	}
	binary.BigEndian.PutUint64(raw[2:10], uint64(cursor.AfterRowID))
	binary.BigEndian.PutUint64(raw[10:18], uint64(cursor.ThroughRowID))
	binary.BigEndian.PutUint64(raw[18:26], uint64(cursor.AsOf.UTC().UnixNano()))
	return base64.RawURLEncoding.EncodeToString(raw[:])
}

func decodeProposalCursor(encoded string) (store.ProposalPageCursor, error) {
	if len(encoded) != base64.RawURLEncoding.EncodedLen(26) {
		return store.ProposalPageCursor{}, errors.New("invalid proposal cursor")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(raw) != 26 || raw[0] != proposalCursorVersion || raw[1] > 1 {
		return store.ProposalPageCursor{}, errors.New("invalid proposal cursor")
	}
	afterRowID := binary.BigEndian.Uint64(raw[2:10])
	throughRowID := binary.BigEndian.Uint64(raw[10:18])
	asOfNanos := binary.BigEndian.Uint64(raw[18:26])
	if afterRowID == 0 || afterRowID > uint64(1<<63-1) || throughRowID < afterRowID || throughRowID > uint64(1<<63-1) || asOfNanos == 0 || asOfNanos > uint64(1<<63-1) {
		return store.ProposalPageCursor{}, errors.New("invalid proposal cursor")
	}
	return store.ProposalPageCursor{
		AfterRowID:   int64(afterRowID),
		ThroughRowID: int64(throughRowID),
		Active:       raw[1] == 1,
		AsOf:         time.Unix(0, int64(asOfNanos)).UTC(),
	}, nil
}

func (s *Server) handleCreateProposal(w http.ResponseWriter, r *http.Request) {
	device := deviceFromContext(r.Context())
	var req assistant.DirectProposalRequest
	if err := decodeBody(w, r, syncmodel.MaxRequestBytes, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	resp, err := s.assistant.HandleDirectProposal(r.Context(), device, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid proposal request")
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

type proposalDecisionRequest struct {
	Decision string `json:"decision"`
	Token    string `json:"token"`
}

func (s *Server) handleProposalDecision(w http.ResponseWriter, r *http.Request) {
	device := deviceFromContext(r.Context())
	id := strings.TrimSpace(r.PathValue("id"))
	var req proposalDecisionRequest
	if err := decodeBody(w, r, maxDeviceBodyBytes, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	status := store.ProposalStatus(req.Decision)
	record, err := s.store.DecideProposal(r.Context(), id, device.ID, status, req.Token, s.now(), json.RawMessage(`{"source":"api"}`))
	switch {
	case errors.Is(err, store.ErrExpiredApprovalToken):
		writeError(w, http.StatusGone, "approval token expired")
		return
	case errors.Is(err, store.ErrUsedApprovalToken), errors.Is(err, store.ErrProposalNotPending):
		writeError(w, http.StatusConflict, "proposal already decided")
		return
	case errors.Is(err, store.ErrInvalidApprovalToken):
		writeError(w, http.StatusUnauthorized, "invalid approval token")
		return
	case errors.Is(err, store.ErrProposalNotFound):
		writeError(w, http.StatusNotFound, "proposal not found")
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, "proposal decision failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": syncmodel.SchemaVersion,
		"proposal":       record,
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.effectiveSleepSessions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "overview projection failed")
		return
	}
	response, err := s.projectionService().Overview(r.Context(), sessions)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "overview projection failed")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleRhythm(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.effectiveSleepSessions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "rhythm projection failed")
		return
	}
	response, err := s.projectionService().Rhythm(r.Context(), sessions)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "rhythm projection failed")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleAccuracy(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.effectiveSleepSessions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "accuracy projection failed")
		return
	}
	response, err := s.projectionService().Accuracy(r.Context(), sessions)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "accuracy projection failed")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) effectiveSleepSessions(ctx context.Context) ([]domain.SleepSession, error) {
	return (readmodel.SleepReader{Store: s.store}).EffectiveSleepSessions(ctx)
}

func (s *Server) projectionService() projection.Service {
	return projection.Service{Now: s.now}
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
	// An accepted push can change sleep inputs, so the shared projection is
	// republished before the response returns (design section 5).
	s.refreshPortal(r)
	writeJSON(w, http.StatusOK, syncmodel.PushResponse{
		SchemaVersion: syncmodel.SchemaVersion,
		Cursor:        cursor,
		Accepted:      accepted,
	})
}

// handleErase extends the user's erasure right (ADR-0014) to the synced
// instance: it hard-deletes the named records and mints tombstones so every
// device erases its copy and no stale push can resurrect them (ADR-0017).
func (s *Server) handleErase(w http.ResponseWriter, r *http.Request) {
	device := deviceFromContext(r.Context())
	var req syncmodel.EraseRequest
	if err := decodeBody(w, r, maxDeviceBodyBytes, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := syncmodel.ValidateEraseRequest(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid erase request")
		return
	}
	erased, tombstones, cursor, err := s.store.EraseSyncRecords(r.Context(), device.ID, req.RecordIDs, s.now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "sync erase failed")
		return
	}
	// Erasure must reach the shared projection too: a window derived from a
	// record the user just deleted may not keep being published.
	s.refreshPortal(r)
	writeJSON(w, http.StatusOK, syncmodel.EraseResponse{
		SchemaVersion: syncmodel.SchemaVersion,
		Erased:        erased,
		Tombstones:    tombstones,
		Cursor:        cursor,
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
