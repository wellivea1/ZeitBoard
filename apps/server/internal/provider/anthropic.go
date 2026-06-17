package provider

import (
	"context"
	"errors"
	"net/http"
)

type anthropicClient struct {
	httpClient
}

func (c anthropicClient) Complete(ctx context.Context, input Request) (Response, error) {
	body := map[string]any{
		"model":      c.model,
		"max_tokens": 1200,
		"system":     input.System,
		"messages": []map[string]string{
			{"role": "user", "content": prompt(input)},
		},
	}
	req, err := jsonRequest(http.MethodPost, c.endpoint, body)
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	var decoded struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := c.doJSON(ctx, req, &decoded); err != nil {
		return Response{}, err
	}
	for _, content := range decoded.Content {
		if content.Text != "" {
			return Response{Text: content.Text}, nil
		}
	}
	return Response{}, errors.New("anthropic response contained no text")
}

func (c anthropicClient) Status() Status {
	return c.httpClient.Status()
}
