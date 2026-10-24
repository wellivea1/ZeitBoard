package chatgpt

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"
)

// maxOutputBytes bounds the text one answer may stream; the assistant reads
// far less.
const maxOutputBytes = 64 * 1024

// Models lists the models the plan offers, in ChatGPT's order. The list is
// read again after ten minutes.
func (a *Account) Models(ctx context.Context) ([]Model, error) {
	a.mu.Lock()
	if a.models != nil && a.now().Before(a.modelsAt.Add(modelsTTL)) {
		models := slices.Clone(a.models)
		a.mu.Unlock()
		return models, nil
	}
	a.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	response, _, err := a.authorized(ctx, func(token string) (*http.Request, error) {
		return a.modelsRequest(ctx, token)
	})
	if err != nil {
		return nil, err
	}
	body, err := readAnswer(response)
	if err != nil {
		return nil, err
	}
	models, err := readCatalog(body)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.models, a.modelsAt = slices.Clone(models), a.now()
	a.mu.Unlock()
	return models, nil
}

// readModels lists the models with an access token just granted.
func (a *Account) readModels(ctx context.Context, token string) ([]Model, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request, err := a.modelsRequest(ctx, token)
	if err != nil {
		return nil, err
	}
	status, body, err := a.send(request)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, answered(status, body)
	}
	return readCatalog(body)
}

func (a *Account) modelsRequest(ctx context.Context, token string) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.endpoints.API+"/models", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	return request, nil
}

// readAnswer reads at most maxAnswerBytes of an answer, and closes it.
func readAnswer(response *http.Response) ([]byte, error) {
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxAnswerBytes+1))
	if err != nil || len(body) > maxAnswerBytes {
		return nil, fail(Busy, "invalid_response")
	}
	return body, nil
}

// readCatalog reads ChatGPT's model catalog: the models it lists, each with a
// slug and a name, skipping any that does not read as one.
func readCatalog(body []byte) ([]Model, error) {
	var catalog struct {
		Models []struct {
			Slug        string `json:"slug"`
			DisplayName string `json:"display_name"`
			Visibility  string `json:"visibility"`
		} `json:"models"`
	}
	if json.Unmarshal(body, &catalog) != nil || catalog.Models == nil {
		return nil, fail(Busy, "invalid_model_catalog")
	}
	models := []Model{}
	for _, entry := range catalog.Models {
		if entry.Visibility != "list" || !catalogText(entry.Slug) || !catalogText(entry.DisplayName) {
			continue
		}
		models = append(models, Model{Slug: entry.Slug, DisplayName: entry.DisplayName})
	}
	return models, nil
}

func catalogText(value string) bool {
	return value != "" && len(value) <= 200 && strings.TrimSpace(value) == value &&
		utf8.ValidString(value) && !strings.ContainsFunc(value, isControl)
}

// ChooseModel sets the model the assistant asks, one the plan offers.
func (a *Account) ChooseModel(ctx context.Context, deviceID, slug string) (Status, error) {
	models, err := a.Models(ctx)
	if err != nil {
		return Status{}, err
	}
	if !slices.ContainsFunc(models, func(model Model) bool { return model.Slug == slug }) {
		return Status{}, fail(ModelUnavailable, "model_not_listed")
	}
	a.turns.Lock()
	defer a.turns.Unlock()
	next := a.current()
	next.Model = slug
	if err := a.keep(ctx, next, "assistant_chatgpt_model_chosen", deviceID); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}

// Respond asks the chosen model for text: instructions, as a system prompt
// is, and one user message. It streams, as the DevKit does, and asks OpenAI
// to keep nothing of it (store is false).
func (a *Account) Respond(ctx context.Context, instructions, input string) (string, error) {
	status := a.Status()
	if status.State != Connected {
		return "", fail(SignInEnded, "not_connected")
	}
	model := status.Model
	if model == "" {
		return "", fail(ModelUnavailable, "no_model")
	}
	body, err := json.Marshal(map[string]any{
		"model":        model,
		"instructions": instructions,
		"input":        []map[string]string{{"role": "user", "content": input}},
		"store":        false,
		"stream":       true,
	})
	if err != nil {
		return "", err
	}
	response, token, err := a.authorized(ctx, func(token string) (*http.Request, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoints.API+"/responses", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "text/event-stream")
		return request, nil
	})
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if kind := response.Header.Get("Content-Type"); kind != "" {
		if media, _, err := mime.ParseMediaType(kind); err != nil || media != "text/event-stream" {
			return "", fail(Busy, "invalid_stream")
		}
	}
	text, err := readStream(response.Body)
	if err != nil && errors.Is(ctx.Err(), context.Canceled) {
		return "", context.Canceled
	}
	// A refusal inside the stream ends the sign-in as one in the status does.
	var failed *Error
	if errors.As(err, &failed) && (failed.Problem == SignInEnded || failed.Problem == PlanNotShared) {
		a.endUnlessRefreshed(ctx, token, failed.Problem)
	}
	return text, err
}

// authorized sends a request with an access token, and gives the answer and
// the token it was sent with. A token ChatGPT refuses is dropped and the
// request sent once more with a fresh one; refused again, or refused for a
// reason that only signing in again mends, the sign-in ends. Any other
// refusal is returned as it is.
func (a *Account) authorized(ctx context.Context, build func(token string) (*http.Request, error)) (*http.Response, string, error) {
	for attempt := 0; ; attempt++ {
		token, err := a.token(ctx)
		if err != nil {
			return nil, "", err
		}
		request, err := build(token)
		if err != nil {
			return nil, "", err
		}
		response, err := a.client.Do(request)
		if err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil, "", context.Canceled
			}
			return nil, "", fail(Busy, "network_error")
		}
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return response, token, nil
		}
		body, _ := readAnswer(response)
		refusal := answered(response.StatusCode, body)
		planCode := strings.HasPrefix(refusal.Code, "subscription_sharing") || strings.HasPrefix(refusal.Code, "chatpass")
		if response.StatusCode == http.StatusUnauthorized && !planCode {
			a.forget(token)
			if attempt == 0 {
				continue
			}
			refusal.Problem = SignInEnded
		}
		if refusal.Problem == SignInEnded || refusal.Problem == PlanNotShared {
			a.endUnlessRefreshed(ctx, token, refusal.Problem)
		}
		return nil, "", refusal
	}
}

// readStream reads a streamed response's text until it completes.
func readStream(body io.Reader) (string, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxAnswerBytes)
	var text, data strings.Builder
	// event handles one event's data, reporting whether the response is done.
	event := func() (bool, error) {
		if data.Len() == 0 {
			return false, nil
		}
		payload := []byte(data.String())
		data.Reset()
		var parsed struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
		}
		if json.Unmarshal(payload, &parsed) != nil {
			return false, fail(Busy, "invalid_stream")
		}
		switch parsed.Type {
		case "response.output_text.delta":
			if text.Len()+len(parsed.Delta) > maxOutputBytes {
				return false, fail(TooLarge, "response_too_large")
			}
			text.WriteString(parsed.Delta)
		case "response.completed":
			return true, nil
		case "response.incomplete":
			return false, fail(TooLarge, "response_incomplete")
		case "response.failed", "error":
			return false, streamError(payload)
		}
		return false, nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			done, err := event()
			if err != nil {
				return "", err
			}
			if done {
				return text.String(), nil
			}
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			value := strings.TrimPrefix(line, "data:")
			data.WriteString(strings.TrimPrefix(value, " "))
		}
	}
	if scanner.Err() != nil {
		return "", fail(Busy, "stream_interrupted")
	}
	// The last event may end the stream without a blank line after it.
	done, err := event()
	if err != nil {
		return "", err
	}
	if done {
		return text.String(), nil
	}
	return "", fail(Busy, "stream_interrupted")
}

// streamError reads the code a failed response or an error event names.
func streamError(payload []byte) *Error {
	var failed struct {
		Response json.RawMessage `json:"response"`
	}
	if json.Unmarshal(payload, &failed) == nil && len(failed.Response) > 0 {
		payload = failed.Response
	}
	code, param := errorCode(payload)
	return &Error{Problem: classify(0, code), Code: code, Param: param}
}
