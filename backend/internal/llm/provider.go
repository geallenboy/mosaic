package llm

import "context"

// Provider 是 LLM 调用的统一抽象接口
type Provider interface {
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
	Name() string
}

type CompletionRequest struct {
	SystemPrompt string
	UserPrompt   string
	Model        string
	MaxTokens    int
	Temperature  float64
}

type CompletionResponse struct {
	Content string
	Usage   TokenUsage
	Model   string
}

type TokenUsage struct {
	InputTokens  int
	OutputTokens int
}
