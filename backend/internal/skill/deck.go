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

const deckSystemPrompt = `你是一位专业的商业提案策划师，擅长为客户创作逻辑清晰、有说服力的 PPT 提案大纲。

输出规范：
- 语言：中文
- 格式：Markdown，每页 PPT 对应一个二级标题（## Page X: 标题）
- 每页包含：页面标题、3-5 个核心要点、备注（演讲引导语）
- 总页数：10-15 页
- 结构：封面 → 背景与机会 → 用户洞察 → 品牌定位 → 营销策略 → 执行方案 → 预算 → 时间节点 → 合作模式 → 结语`

var deckPromptTmpl = template.Must(template.New("deck").Parse(`
项目信息：
- 项目名称：{{.Objective}}
- 行业：{{.Industry}}
- 目标用户：{{.Audience}}
- 核心价值主张：{{.ValueProposition}}
- 品牌定位：{{.Positioning}}

营销策略摘要：
{{.StrategySummary}}

请为以上项目创作一份客户提案 PPT 大纲（Markdown 格式）。`))

type DeckSkill struct {
	llm llm.Provider
}

func NewDeckSkill(provider llm.Provider) *DeckSkill {
	return &DeckSkill{llm: provider}
}

func (s *DeckSkill) Type() Type              { return TypeDeck }
func (s *DeckSkill) SupportedEntries() []string { return []string{"slides"} }
func (s *DeckSkill) Dependencies() []Type    { return []Type{TypeStrategy} }

func (s *DeckSkill) Execute(ctx context.Context, input Input) (*Output, error) {
	start := time.Now()

	prompt, err := s.buildPrompt(input)
	if err != nil {
		return nil, fmt.Errorf("deck: build prompt: %w", err)
	}

	resp, err := s.llm.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: deckSystemPrompt,
		UserPrompt:   prompt,
		MaxTokens:    3500,
		Temperature:  0.4,
	})
	if err != nil {
		return nil, fmt.Errorf("deck: llm complete: %w", err)
	}

	slog.InfoContext(ctx, "skill_executed",
		"skill_type", TypeDeck,
		"project_id", input.ProjectID,
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return &Output{
		SkillType: s.Type(),
		EntryType: "slides",
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

func (s *DeckSkill) buildPrompt(input Input) (string, error) {
	brief := input.ProjectBrief
	strategySummary := ""
	if prev, ok := input.PreviousOutputs[TypeStrategy]; ok && prev != nil {
		content := prev.Content
		if len([]rune(content)) > 600 {
			runes := []rune(content)
			content = string(runes[:600]) + "\n...(省略)"
		}
		strategySummary = content
	}

	data := struct {
		Objective        string
		Industry         string
		Audience         string
		ValueProposition string
		Positioning      string
		StrategySummary  string
	}{
		Objective:        brief.Objective,
		Industry:         brief.Industry,
		Audience:         brief.Audience,
		ValueProposition: brief.ValueProposition,
		Positioning:      brief.Positioning,
		StrategySummary:  strategySummary,
	}
	var buf bytes.Buffer
	if err := deckPromptTmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
