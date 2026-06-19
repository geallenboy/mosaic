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

const copySystemPrompt = `你是一位专业的营销文案策划，擅长为商业项目创作有传播力的宣传文案。

输出规范：
- 语言：中文
- 格式：Markdown，分模块输出不同场景的文案
- 包含：品牌 Slogan（3 个方向）、小红书笔记（3 篇）、朋友圈文案（5 条）、抖音视频文案（2 个）
- 文案风格需符合品牌定位，不浮夸，真实有力
- 每条文案后注明适用场景`

var copyPromptTmpl = template.Must(template.New("copy").Parse(`
品牌信息：
- 行业：{{.Industry}}
- 目标用户：{{.Audience}}
- 品牌定位：{{.Positioning}}
- 核心价值主张：{{.ValueProposition}}
- 品牌语气：{{.ToneOfVoice}}

营销策略方向：
{{.StrategySummary}}

请为以上品牌创作一套多平台营销文案包。`))

type CopySkill struct {
	llm llm.Provider
}

func NewCopySkill(provider llm.Provider) *CopySkill {
	return &CopySkill{llm: provider}
}

func (s *CopySkill) Type() Type              { return TypeCopy }
func (s *CopySkill) SupportedEntries() []string { return []string{"documents", "slides", "videos", "podcasts"} }
func (s *CopySkill) Dependencies() []Type    { return []Type{TypeStrategy} }

func (s *CopySkill) Execute(ctx context.Context, input Input) (*Output, error) {
	start := time.Now()

	prompt, err := s.buildPrompt(input)
	if err != nil {
		return nil, fmt.Errorf("copy: build prompt: %w", err)
	}

	resp, err := s.llm.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: copySystemPrompt,
		UserPrompt:   prompt,
		MaxTokens:    3000,
		Temperature:  0.6,
	})
	if err != nil {
		return nil, fmt.Errorf("copy: llm complete: %w", err)
	}

	slog.InfoContext(ctx, "skill_executed",
		"skill_type", TypeCopy,
		"project_id", input.ProjectID,
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

func (s *CopySkill) buildPrompt(input Input) (string, error) {
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
		Industry        string
		Audience        string
		Positioning     string
		ValueProposition string
		ToneOfVoice     string
		StrategySummary string
	}{
		Industry:        brief.Industry,
		Audience:        brief.Audience,
		Positioning:     brief.Positioning,
		ValueProposition: brief.ValueProposition,
		ToneOfVoice:     brief.ToneOfVoice,
		StrategySummary: strategySummary,
	}
	var buf bytes.Buffer
	if err := copyPromptTmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
