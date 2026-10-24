package api

import (
	"context"
	"errors"
	"net/http"

	"non24.app/server/internal/chatgpt"
	syncmodel "non24.app/server/internal/sync"
)

// The assistant through the owner's ChatGPT plan (ADR-0066). The server holds
// the sign-in: a device asks it for the address to open in the browser, and
// passes back the one-time code the browser brings to the device's loopback
// address. No answer here carries a token.

type chatGPTStatusResponse struct {
	SchemaVersion string        `json:"schema_version"`
	ChatGPT       chatGPTStatus `json:"chatgpt"`
	// Revoked answers a disconnection: false when ChatGPT did not confirm it
	// withdrew the grant.
	Revoked *bool `json:"revoked,omitempty"`
}

type chatGPTStatus struct {
	// Offered is false when the configuration names another provider, which
	// a sign-in never replaces.
	Offered bool `json:"offered"`
	chatgpt.Status
}

type chatGPTSignInRequest struct {
	RedirectURI string `json:"redirectUri"`
}

type chatGPTFinishRequest struct {
	State    string `json:"state"`
	Code     string `json:"code"`
	ClientID string `json:"clientId"`
}

type chatGPTModelRequest struct {
	Model string `json:"model"`
}

const maxChatGPTBodyBytes = 16 * 1024

func (s *Server) handleChatGPTStatus(w http.ResponseWriter, _ *http.Request) {
	s.writeChatGPTStatus(w, nil)
}

func (s *Server) writeChatGPTStatus(w http.ResponseWriter, revoked *bool) {
	status := chatGPTStatus{}
	if s.chatgpt != nil {
		status = chatGPTStatus{Offered: true, Status: s.chatgpt.Status()}
	}
	writeJSON(w, http.StatusOK, chatGPTStatusResponse{SchemaVersion: syncmodel.SchemaVersion, ChatGPT: status, Revoked: revoked})
}

func (s *Server) handleChatGPTModels(w http.ResponseWriter, r *http.Request) {
	models, err := s.chatgpt.Models(r.Context())
	if err != nil {
		writeChatGPTError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schema_version": syncmodel.SchemaVersion, "models": models})
}

func (s *Server) handleChatGPTSignIn(w http.ResponseWriter, r *http.Request) {
	var req chatGPTSignInRequest
	if err := decodeBody(w, r, maxChatGPTBodyBytes, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	signIn, err := s.chatgpt.BeginSignIn(r.Context(), req.RedirectURI)
	if err != nil {
		writeChatGPTError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schema_version": syncmodel.SchemaVersion, "signIn": signIn})
}

func (s *Server) handleChatGPTFinish(w http.ResponseWriter, r *http.Request) {
	var req chatGPTFinishRequest
	if err := decodeBody(w, r, maxChatGPTBodyBytes, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	device := deviceFromContext(r.Context())
	if _, err := s.chatgpt.FinishSignIn(r.Context(), device.ID, req.State, req.Code, req.ClientID); err != nil {
		writeChatGPTError(w, err)
		return
	}
	s.writeChatGPTStatus(w, nil)
}

func (s *Server) handleChatGPTModel(w http.ResponseWriter, r *http.Request) {
	var req chatGPTModelRequest
	if err := decodeBody(w, r, maxChatGPTBodyBytes, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	device := deviceFromContext(r.Context())
	if _, err := s.chatgpt.ChooseModel(r.Context(), device.ID, req.Model); err != nil {
		writeChatGPTError(w, err)
		return
	}
	s.writeChatGPTStatus(w, nil)
}

func (s *Server) handleChatGPTDisconnect(w http.ResponseWriter, r *http.Request) {
	device := deviceFromContext(r.Context())
	revoked, err := s.chatgpt.Disconnect(r.Context(), device.ID)
	if err != nil {
		writeChatGPTError(w, err)
		return
	}
	s.writeChatGPTStatus(w, &revoked)
}

// writeChatGPTError answers with the problem a device words for the owner.
// None is a 401 or 403: those say the device itself was refused.
func writeChatGPTError(w http.ResponseWriter, err error) {
	var failed *chatgpt.Error
	if !errors.As(err, &failed) {
		if errors.Is(err, context.Canceled) {
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "the ChatGPT connection could not be kept"})
		return
	}
	status := http.StatusConflict
	switch failed.Problem {
	case chatgpt.UsageLimit:
		status = http.StatusTooManyRequests
	case chatgpt.Busy:
		status = http.StatusServiceUnavailable
	case chatgpt.Refused:
		status = http.StatusBadGateway
		if failed.Code == "invalid_redirect" || failed.Code == "invalid_code" {
			status = http.StatusBadRequest
		}
	}
	writeJSON(w, status, map[string]string{"error": "ChatGPT: " + string(failed.Problem), "problem": string(failed.Problem)})
}
