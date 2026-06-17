package provider

import (
	"context"
	"errors"
	"net/http"
)

type chatCompletionsClient struct {
	httpClient
	providerName Name
}

func (c chatCompletionsClient) Complete(ctx context.Context, input Request) (Response, error) {
	body := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": input.System},
			{"role": "user", "content": prompt(input)},
		},
		"response_format": map[string]string{"type": "json_object"},
	}
	req, err := jsonRequest(http.MethodPost, c.endpoint, body)
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if c.providerName == OpenRouter {
		req.Header.Set("HTTP-Referer", "https://zeitboard.local")
		req.Header.Set("X-Title", "ZeitBoard")
	}

	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := c.doJSON(ctx, req, &decoded); err != nil {
		return Response{}, err
	}
	for _, choice := range decoded.Choices {
		if choice.Message.Content != "" {
			return Response{Text: choice.Message.Content}, nil
		}
	}
	return Response{}, errors.New("chat completion response contained no text")
}

func (c chatCompletionsClient) Status() Status {
	status := c.httpClient.Status()
	status.Provider = string(c.providerName)
	return status
}
