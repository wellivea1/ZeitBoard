package localagent

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	SessionIDHeader       = "Mcp-Session-Id"
	ProtocolVersionHeader = "Mcp-Protocol-Version"
	maxRequestBytes       = 2 * 1024 * 1024
	maxSessions           = 32
	sessionTTL            = 30 * time.Minute
)

type sessionState struct {
	mu               sync.Mutex
	TotalRemaining   int
	ProposeRemaining int
	TouchedAt        time.Time
	Initialized      bool
}

type Handler struct {
	token      string
	capability Capability
	now        func() time.Time

	mu       sync.Mutex
	sessions map[string]*sessionState
}

func NewHandler(token string, capability Capability) (*Handler, error) {
	if len(token) < 32 {
		return nil, errors.New("local agent token is too short")
	}
	if capability == nil {
		return nil, errors.New("local agent capability is required")
	}
	return &Handler{
		token:      token,
		capability: capability,
		now:        func() time.Time { return time.Now().UTC() },
		sessions:   make(map[string]*sessionState),
	}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path != "/mcp" || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	if !requestIsLoopback(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	// Test for presence, not value: Header.Get returns "" both when the header
	// is absent and when it is present but empty, so the value check alone let
	// a request with a literal empty Origin through.
	if _, hasOrigin := r.Header["Origin"]; hasOrigin {
		http.Error(w, "browser origins are not accepted", http.StatusForbidden)
		return
	}
	if !h.authorized(r) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="ZeitBoard local agent"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodPost:
		h.handlePost(w, r)
	case http.MethodDelete:
		h.handleDelete(w, r)
	case http.MethodGet:
		w.Header().Set("Allow", "POST, DELETE")
		http.Error(w, "server-initiated streams are not supported", http.StatusMethodNotAllowed)
	default:
		w.Header().Set("Allow", "POST, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handlePost(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}
	accept := r.Header.Get("Accept")
	if !acceptsMediaType(accept, "application/json") || !acceptsMediaType(accept, "text/event-stream") {
		http.Error(w, "Accept must include application/json and text/event-stream", http.StatusNotAcceptable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	var req rpcRequest
	if err := decodeOne(body, &req); err != nil {
		h.writeRPC(w, http.StatusOK, rpcErrorResponse(json.RawMessage("null"), -32700, "parse error"))
		return
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		h.writeRPC(w, http.StatusOK, rpcErrorResponse(req.idOrNull(), -32600, "invalid request"))
		return
	}
	if len(req.ID) > 0 && !validRPCID(req.ID) {
		h.writeRPC(w, http.StatusOK, rpcErrorResponse(json.RawMessage("null"), -32600, "invalid request id"))
		return
	}
	if req.Method == "initialize" {
		h.initialize(w, r, req)
		return
	}

	session, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	if len(req.ID) == 0 {
		if req.Method == "notifications/initialized" {
			markInitialized(session)
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}

	response := h.handleRequest(r.Context(), session, req)
	h.writeRPC(w, http.StatusOK, response)
}

func (h *Handler) initialize(w http.ResponseWriter, r *http.Request, req rpcRequest) {
	if len(req.ID) == 0 {
		h.writeRPC(w, http.StatusOK, rpcErrorResponse(json.RawMessage("null"), -32600, "initialize requires an id"))
		return
	}
	if r.Header.Get(SessionIDHeader) != "" {
		http.Error(w, "initialize must not include a session id", http.StatusBadRequest)
		return
	}
	var params struct {
		ProtocolVersion string                     `json:"protocolVersion"`
		Capabilities    map[string]json.RawMessage `json:"capabilities"`
		ClientInfo      *struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"clientInfo"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || strings.TrimSpace(params.ProtocolVersion) == "" || len(params.ProtocolVersion) > 64 || params.Capabilities == nil || params.ClientInfo == nil || strings.TrimSpace(params.ClientInfo.Name) == "" || strings.TrimSpace(params.ClientInfo.Version) == "" || len(params.ClientInfo.Name) > 128 || len(params.ClientInfo.Version) > 128 {
		h.writeRPC(w, http.StatusOK, rpcErrorResponse(req.ID, -32602, "invalid initialize parameters"))
		return
	}
	sessionID, err := randomToken(24)
	if err != nil {
		h.writeRPC(w, http.StatusInternalServerError, rpcErrorResponse(req.ID, -32603, "could not create MCP session"))
		return
	}
	h.addSession(sessionID)
	w.Header().Set(SessionIDHeader, sessionID)
	h.writeRPC(w, http.StatusOK, rpcResult(req.ID, map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities": map[string]any{
			"tools": map[string]any{"listChanged": false},
		},
		"serverInfo": map[string]any{
			"name":    "zeitboard-desktop-local",
			"title":   "ZeitBoard Desktop Local Agent",
			"version": "0.1.0",
		},
		"instructions": "Read allowlisted desktop projections, refuse medical decisions, and create proposals only. No tool can approve or apply a health or schedule change. Appearance changes are direct, local, and reversible.",
	}))
}

func (h *Handler) handleRequest(ctx context.Context, session *sessionState, req rpcRequest) rpcResponse {
	if req.Method != "ping" && !isInitialized(session) {
		return rpcErrorResponse(req.ID, -32002, "server is not initialized")
	}
	switch req.Method {
	case "ping":
		return rpcResult(req.ID, map[string]any{})
	case "tools/list":
		return rpcResult(req.ID, map[string]any{"tools": ToolDefinitions(h.capability.ProposalsAvailable(ctx))})
	case "tools/call":
		return rpcResult(req.ID, h.callTool(ctx, session, req.Params))
	default:
		return rpcErrorResponse(req.ID, -32601, "method not found")
	}
}

func (h *Handler) callTool(ctx context.Context, session *sessionState, params json.RawMessage) ToolResult {
	var call callToolParams
	if len(params) == 0 || decodeOne(params, &call) != nil || call.Name == "" {
		return textError("Invalid tools/call parameters. No tool was run.")
	}
	proposalsAvailable := h.capability.ProposalsAvailable(ctx)
	if !KnownTool(call.Name, proposalsAvailable) {
		if IsProposeTool(call.Name) {
			return textError("Proposal tools need an enabled, enrolled self-hosted backend. No proposal was created.")
		}
		return textError("Unknown ZeitBoard tool.")
	}
	if err := consumeBudget(session, IsProposeTool(call.Name)); err != nil {
		return textError(err.Error())
	}
	if len(call.Arguments) == 0 {
		call.Arguments = json.RawMessage(`{}`)
	}
	data, err := h.capability.CallTool(ctx, call.Name, call.Arguments)
	if err != nil {
		return textError(safeToolError(err))
	}
	if !json.Valid(data) {
		return textError("ZeitBoard rejected an invalid local projection. No data was returned.")
	}
	return jsonResult(data)
}

func consumeBudget(session *sessionState, propose bool) error {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.TotalRemaining <= 0 {
		return errors.New("Tool call budget exceeded; this ZeitBoard MCP session is closed to further calls.")
	}
	if propose && session.ProposeRemaining <= 0 {
		return errors.New("Proposal call budget exceeded; no more proposal tools may be called in this session.")
	}
	session.TotalRemaining--
	if propose {
		session.ProposeRemaining--
	}
	return nil
}

func markInitialized(session *sessionState) {
	session.mu.Lock()
	session.Initialized = true
	session.mu.Unlock()
}

func isInitialized(session *sessionState) bool {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.Initialized
}

func validRPCID(id json.RawMessage) bool {
	if len(id) == 0 || len(id) > 256 || bytes.Equal(bytes.TrimSpace(id), []byte("null")) {
		return false
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(id))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return false
	}
	switch value.(type) {
	case string, json.Number:
		return true
	default:
		return false
	}
}

func (h *Handler) requireSession(w http.ResponseWriter, r *http.Request) (*sessionState, bool) {
	if r.Header.Get(ProtocolVersionHeader) != ProtocolVersion {
		http.Error(w, "unsupported or missing MCP protocol version", http.StatusBadRequest)
		return nil, false
	}
	sessionID := r.Header.Get(SessionIDHeader)
	if sessionID == "" {
		http.Error(w, "MCP session id is required", http.StatusBadRequest)
		return nil, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	h.pruneLocked(now)
	session, ok := h.sessions[sessionID]
	if !ok {
		http.Error(w, "MCP session not found", http.StatusNotFound)
		return nil, false
	}
	session.TouchedAt = now
	return session, true
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(ProtocolVersionHeader) != ProtocolVersion {
		http.Error(w, "unsupported or missing MCP protocol version", http.StatusBadRequest)
		return
	}
	sessionID := r.Header.Get(SessionIDHeader)
	if sessionID == "" {
		http.Error(w, "MCP session id is required", http.StatusBadRequest)
		return
	}
	h.mu.Lock()
	_, found := h.sessions[sessionID]
	delete(h.sessions, sessionID)
	h.mu.Unlock()
	if !found {
		http.Error(w, "MCP session not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) addSession(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	h.pruneLocked(now)
	if len(h.sessions) >= maxSessions {
		ids := make([]string, 0, len(h.sessions))
		for candidate := range h.sessions {
			ids = append(ids, candidate)
		}
		sort.Slice(ids, func(i, j int) bool {
			return h.sessions[ids[i]].TouchedAt.Before(h.sessions[ids[j]].TouchedAt)
		})
		delete(h.sessions, ids[0])
	}
	h.sessions[id] = &sessionState{TotalRemaining: DefaultTotalBudget, ProposeRemaining: DefaultProposeBudget, TouchedAt: now}
}

func (h *Handler) pruneLocked(now time.Time) {
	for id, session := range h.sessions {
		if now.Sub(session.TouchedAt) > sessionTTL {
			delete(h.sessions, id)
		}
	}
}

func (h *Handler) authorized(r *http.Request) bool {
	const prefix = "Bearer "
	value := r.Header.Get("Authorization")
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	provided := strings.TrimSpace(strings.TrimPrefix(value, prefix))
	return len(provided) == len(h.token) && subtle.ConstantTimeCompare([]byte(provided), []byte(h.token)) == 1
}

func requestIsLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func acceptsMediaType(value, wanted string) bool {
	for _, entry := range strings.Split(value, ",") {
		mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(entry))
		if err != nil || mediaType != wanted {
			continue
		}
		if quality, ok := params["q"]; ok {
			parsed, err := strconv.ParseFloat(quality, 64)
			if err != nil || parsed <= 0 || parsed > 1 {
				continue
			}
		}
		return true
	}
	return false
}

func decodeOne(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON content")
	}
	return nil
}

func (h *Handler) writeRPC(w http.ResponseWriter, status int, response rpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}
