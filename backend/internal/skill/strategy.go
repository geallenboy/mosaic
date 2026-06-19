package skill

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"text/template"
	"time"

	"github.com/mosaic-app/mosaic/backend/internal/llm"
)

const strategySystemPrompt = `你是一位资深品牌策略顾问，擅长为商业项目制定清晰、可执行的品牌定位和营销策略。

输出规范：
- 语言：中文
- 格式：Markdown，使用标题（##）和列表
- 长度：1500-2000 字
- 策略建议必须基于提供的调研信息，不要泛泛而谈
- 结构：品牌定位 → 目标用户细分 → 核心差异化 → 营销主张 → 传播策略方向`

var strategyPromptTmpl = template.Must(template.New("strategy").Parse(`
项目信息：
- 行业：{{.Industry}}
- 目标市场：{{.Market}}
- 项目目标：{{.Objective}}
- 目标用户：{{.Audience}}
- 品牌语气：{{.ToneOfVoice}}
- 视觉方向：{{.VisualDirection}}

调研结论摘要：
{{.ResearchSummary}}

请基于以上信息，制定一份品牌定位与营销策略方案。`))

type StrategySkill struct {
	llm llm.Provider
}

func NewStrategySkill(provider llm.Provider) *StrategySkill {
	return &StrategySkill{llm: provider}
}

func (s *StrategySkill) Type() Type              { return TypeStrategy }
func (s *StrategySkill) SupportedEntries() []string { return []string{"documents", "slides"} }
func (s *StrategySkill) Dependencies() []Type    { return []Type{TypeResearch} }

func (s *StrategySkill) Execute(ctx context.Context, input Input) (*Output, error) {
	start := time.Now()

	prompt, err := s.buildPrompt(input)
	if err != nil {
		return nil, fmt.Errorf("strategy: build prompt: %w", err)
	}

	resp, err := s.llm.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: strategySystemPrompt,
		UserPrompt:   prompt,
		MaxTokens:    2500,
		Temperature:  0.4,
	})
	if err != nil {
		return nil, fmt.Errorf("strategy: llm complete: %w", err)
	}

	slog.InfoContext(ctx, "skill_executed",
		"skill_type", TypeStrategy,
		"project_id", input.ProjectID,
		"input_tokens", resp.Usage.InputTokens,
		"output_tokens", resp.Usage.OutputTokens,
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return &Output{
		SkillType: s.Type(),
		EntryType: "documents",
		Format:    FormatMarkdown,
		Content:   resp.Content,
		LLMUsage: &LLMUsage{
			Provider:     s.llm.Name(),
			Model:        resp.Model,
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
		},
	}, nil
}

func (s *StrategySkill) buildPrompt(input Input) (string, error) {
	brief := input.ProjectBrief
	researchSummary := ""
	if prev, ok := input.PreviousOutputs[TypeResearch]; ok && prev != nil {
		// 截取前 800 字作为摘要，避免 prompt 过长
		content := prev.Content
		if len([]rune(content)) > 800 {
			runes := []rune(content)
			content = string(runes[:800]) + "\n...(省略)"
		}
		researchSummary = content
	}

	data := struct {
		Industry        string
		Market          string
		Objective       string
		Audience        string
		ToneOfVoice     string
		VisualDirection string
		ResearchSummary string
	}{
		Industry:        brief.Industry,
		Market:          brief.Market,
		Objective:       brief.Objective,
		Audience:        brief.Audience,
		ToneOfVoice:     brief.ToneOfVoice,
		VisualDirection: brief.VisualDirection,
		ResearchSummary: researchSummary,
	}
	var buf bytes.Buffer
	if err := strategyPromptTmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
