package provider

import (
	"context"
	"errors"
	"net/http"
)

type openAIClient struct {
	httpClient
}

func (c openAIClient) Complete(ctx context.Context, input Request) (Response, error) {
	body := map[string]any{
		"model": c.model,
		"store": false,
		"input": []map[string]string{
			{"role": "system", "content": input.System},
			{"role": "user", "content": prompt(input)},
		},
	}
	req, err := jsonRequest(http.MethodPost, c.endpoint, body)
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	var decoded struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Text string `json:"text"`
				Type string `json:"type"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := c.doJSON(ctx, req, &decoded); err != nil {
		return Response{}, err
	}
	if decoded.OutputText != "" {
		return Response{Text: decoded.OutputText}, nil
	}
	for _, output := range decoded.Output {
		for _, content := range output.Content {
			if content.Text != "" {
				return Response{Text: content.Text}, nil
			}
		}
	}
	return Response{}, errors.New("openai response contained no text")
}

func (c openAIClient) Status() Status {
	return c.httpClient.Status()
}
