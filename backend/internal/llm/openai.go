package llm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

type OpenAIClient struct {
	client *openai.Client
	model  string
}

func NewOpenAI(apiKey, defaultModel string) *OpenAIClient {
	return &OpenAIClient{
		client: openai.NewClient(apiKey),
		model:  defaultModel,
	}
}

func (c *OpenAIClient) Name() string { return "openai" }

func (c *OpenAIClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	model := req.Model
	if model == "" {
		model = c.model
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}
	temperature := req.Temperature
	if temperature == 0 {
		temperature = 0.3
	}

	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: req.SystemPrompt},
		{Role: openai.ChatMessageRoleUser, Content: req.UserPrompt},
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			wait := time.Duration(1<<attempt) * time.Second
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			slog.WarnContext(ctx, "llm_retry", "attempt", attempt, "error", lastErr)
		}

		resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model:       model,
			Messages:    messages,
			MaxTokens:   maxTokens,
			Temperature: float32(temperature),
		})
		if err == nil {
			return &CompletionResponse{
				Content: resp.Choices[0].Message.Content,
				Model:   resp.Model,
				Usage: TokenUsage{
					InputTokens:  resp.Usage.PromptTokens,
					OutputTokens: resp.Usage.CompletionTokens,
				},
			}, nil
		}

		if isRetryable(err) {
			lastErr = err
			continue
		}
		return nil, fmt.Errorf("openai complete: %w", err)
	}
	return nil, fmt.Errorf("openai complete after 3 attempts: %w", lastErr)
}

func isRetryable(err error) bool {
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		return apiErr.HTTPStatusCode == 429 || apiErr.HTTPStatusCode >= 500
	}
	return false
}
