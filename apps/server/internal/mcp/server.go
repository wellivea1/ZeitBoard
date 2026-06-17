package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const ProtocolVersion = "2025-11-25"

type Server struct {
	Backend              *BackendClient
	Configured           bool
	UnavailableReason    string
	TotalRemaining       int
	ProposeRemaining     int
	probedAvailable      bool
	probedAvailableValid bool
}

func NewServer(cfg Config) *Server {
	return &Server{
		Backend:           cfg.BackendClient(),
		Configured:        cfg.Configured,
		UnavailableReason: cfg.UnavailableReason,
		TotalRemaining:    cfg.TotalCallBudget,
		ProposeRemaining:  cfg.ProposeCallBudget,
	}
}

func (s *Server) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	encoder := json.NewEncoder(output)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		response, ok := s.handleLine(ctx, line)
		if !ok {
			continue
		}
		if err := encoder.Encode(response); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleLine(ctx context.Context, line []byte) (rpcResponse, bool) {
	var req rpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return rpcErrorResponse(json.RawMessage("null"), -32700, "parse error"), true
	}
	if req.JSONRPC != "2.0" {
		return rpcErrorResponse(req.idOrNull(), -32600, "invalid JSON-RPC version"), true
	}
	if len(req.ID) == 0 {
		s.handleNotification(req)
		return rpcResponse{}, false
	}
	switch req.Method {
	case "initialize":
		return rpcResult(req.ID, map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{"listChanged": false},
			},
			"serverInfo": map[string]any{
				"name":    "zeitboard-mcp",
				"title":   "ZeitBoard MCP",
				"version": "0.1.0",
			},
			"instructions": "Read ZeitBoard projections and create pending schedule proposals only. No tool can approve or apply a proposal.",
		}), true
	case "ping":
		return rpcResult(req.ID, map[string]any{}), true
	case "tools/list":
		return rpcResult(req.ID, map[string]any{"tools": s.availableTools(ctx)}), true
	case "tools/call":
		result, err := s.callTool(ctx, req.Params)
		if err != nil {
			return rpcErrorResponse(req.ID, -32602, err.Error()), true
		}
		return rpcResult(req.ID, result), true
	default:
		return rpcErrorResponse(req.ID, -32601, "method not found"), true
	}
}

func (s *Server) handleNotification(req rpcRequest) {
	_ = req
}

func (s *Server) availableTools(ctx context.Context) []toolDefinition {
	if err := s.backendAvailable(ctx); err != nil {
		return []toolDefinition{}
	}
	return toolDefinitions()
}

func (s *Server) backendAvailable(ctx context.Context) error {
	if !s.Configured {
		if s.UnavailableReason == "" {
			return errors.New("backend is not configured")
		}
		return errors.New(s.UnavailableReason)
	}
	if s.probedAvailableValid {
		if s.probedAvailable {
			return nil
		}
		return errors.New("backend is unavailable")
	}
	if err := s.Backend.Available(ctx); err != nil {
		s.probedAvailable = false
		s.probedAvailableValid = true
		return err
	}
	s.probedAvailable = true
	s.probedAvailableValid = true
	return nil
}

func (s *Server) callTool(ctx context.Context, params json.RawMessage) (toolResult, error) {
	if err := s.backendAvailable(ctx); err != nil {
		return textError("ZeitBoard backend is unavailable or not configured; no tools are exposed."), nil
	}
	var req callToolParams
	if len(params) == 0 {
		return toolResult{}, errors.New("tools/call params are required")
	}
	dec := json.NewDecoder(bytes.NewReader(params))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return toolResult{}, errors.New("invalid tools/call params")
	}
	if req.Name == "" {
		return toolResult{}, errors.New("tool name is required")
	}
	if !knownTool(req.Name) {
		return toolResult{}, fmt.Errorf("unknown tool: %s", req.Name)
	}
	if err := s.consumeBudget(isProposeTool(req.Name)); err != nil {
		return textError(err.Error()), nil
	}
	if isProposeTool(req.Name) {
		return s.callProposeTool(ctx, req.Name, req.Arguments)
	}
	path, ok := readToolPath(req.Name)
	if !ok {
		return toolResult{}, fmt.Errorf("unknown tool: %s", req.Name)
	}
	body, err := s.Backend.Get(ctx, path)
	if err != nil {
		return textError(err.Error()), nil
	}
	return jsonResult(body), nil
}

func (s *Server) callProposeTool(ctx context.Context, name string, arguments json.RawMessage) (toolResult, error) {
	var args proposeArguments
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	dec := json.NewDecoder(bytes.NewReader(arguments))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&args); err != nil {
		return textError("Invalid propose tool arguments."), nil
	}
	payload, err := args.directProposalPayload(name)
	if err != nil {
		return textError(err.Error()), nil
	}
	body, err := s.Backend.Post(ctx, "/v1/proposals", payload)
	if err != nil {
		return textError(err.Error()), nil
	}
	return jsonResult(body), nil
}

func (s *Server) consumeBudget(propose bool) error {
	if s.TotalRemaining <= 0 {
		return errors.New("Tool call budget exceeded; the ZeitBoard MCP session is closed to further calls.")
	}
	if propose && s.ProposeRemaining <= 0 {
		return errors.New("Proposal call budget exceeded; no more propose tools may be called in this session.")
	}
	s.TotalRemaining--
	if propose {
		s.ProposeRemaining--
	}
	return nil
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (r rpcRequest) idOrNull() json.RawMessage {
	if len(r.ID) == 0 {
		return json.RawMessage("null")
	}
	return r.ID
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func rpcResult(id json.RawMessage, result any) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func rpcErrorResponse(id json.RawMessage, code int, message string) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}}
}

type callToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type toolResult struct {
	Content           []textContent   `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
	IsError           bool            `json:"isError,omitempty"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func jsonResult(data json.RawMessage) toolResult {
	return toolResult{
		Content:           []textContent{{Type: "text", Text: string(data)}},
		StructuredContent: data,
	}
}

func textError(message string) toolResult {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "Tool call failed."
	}
	return toolResult{
		Content: []textContent{{Type: "text", Text: message}},
		IsError: true,
	}
}
