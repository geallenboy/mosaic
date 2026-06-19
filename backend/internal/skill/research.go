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

const researchSystemPrompt = `你是一位专业的商业市场研究分析师，擅长通过有限信息对目标市场做出清晰、有依据的分析。

输出规范：
- 语言：中文
- 格式：Markdown，使用标题（##）和列表
- 长度：1500-2500 字
- 必须明确标注推断性内容：在句末加注"（基于用户输入推断）"或"（建议进一步验证）"
- 禁止编造具体数据（如市场规模数字、转化率等），可提供范围估算并说明来源假设
- 结构：目标用户画像 → 竞争格局 → 差异化机会 → 市场切入建议 → 主要风险`

var researchPromptTmpl = template.Must(template.New("research").Parse(`
行业：{{.Industry}}
目标市场：{{.Market}}
目标用户：{{.Audience}}
项目目标：{{.Objective}}
核心价值主张：{{.ValueProposition}}

请基于以上信息，完成一份市场调研分析报告，帮助项目团队理解市场机会与竞争环境。`))

type ResearchSkill struct {
	llm llm.Provider
}

func NewResearchSkill(provider llm.Provider) *ResearchSkill {
	return &ResearchSkill{llm: provider}
}

func (s *ResearchSkill) Type() Type              { return TypeResearch }
func (s *ResearchSkill) SupportedEntries() []string { return []string{"documents", "sheets"} }
func (s *ResearchSkill) Dependencies() []Type    { return nil }

func (s *ResearchSkill) Execute(ctx context.Context, input Input) (*Output, error) {
	start := time.Now()

	prompt, err := s.buildPrompt(input)
	if err != nil {
		return nil, fmt.Errorf("research: build prompt: %w", err)
	}

	resp, err := s.llm.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: researchSystemPrompt,
		UserPrompt:   prompt,
		MaxTokens:    3000,
		Temperature:  0.3,
	})
	if err != nil {
		return nil, fmt.Errorf("research: llm complete: %w", err)
	}

	slog.InfoContext(ctx, "skill_executed",
		"skill_type", TypeResearch,
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

func (s *ResearchSkill) buildPrompt(input Input) (string, error) {
	brief := input.ProjectBrief
	data := struct {
		Industry         string
		Market           string
		Audience         string
		Objective        string
		ValueProposition string
	}{
		Industry:         brief.Industry,
		Market:           brief.Market,
		Audience:         brief.Audience,
		Objective:        brief.Objective,
		ValueProposition: brief.ValueProposition,
	}
	var buf bytes.Buffer
	if err := researchPromptTmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
