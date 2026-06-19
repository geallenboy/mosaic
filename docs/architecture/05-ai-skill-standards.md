# 05 AI/Skill 开发规范

## 1. Skill 接口契约

每个 Skill 必须严格遵循统一接口，不允许 Skill 之间直接通信（只通过编排器协调）。

### 1.1 Go 接口定义

```go
// internal/skill/runtime.go

type Skill interface {
    // Type 返回 Skill 唯一标识
    Type() Type
    // Execute 执行 Skill，幂等，可重试
    Execute(ctx context.Context, input Input) (*Output, error)
    // SupportedEntries 返回该 Skill 服务的入口列表
    SupportedEntries() []string
    // Dependencies 返回依赖的前置 Skill 类型（可为空）
    Dependencies() []Type
}
```

### 1.2 输入规范（Input）

```go
type Input struct {
    // 来自项目母版说明书的核心上下文（所有 Skill 共享）
    ProjectBrief *ProjectBrief

    // 依赖 Skill 的输出结果（按类型索引）
    // 例如 Strategy Skill 可以通过 PreviousOutputs[TypeResearch] 获取调研结果
    PreviousOutputs map[Type]*Output

    // 目标入口（Skill 可能服务多个入口，此字段指定本次目标）
    EntryType string

    // 用户额外传入的自定义参数（模板变量、语气调整等）
    UserContext map[string]any

    // 用于追踪的请求标识
    RequestID string
    ProjectID string
    TaskID    string
}
```

### 1.3 输出规范（Output）

```go
type OutputFormat string

const (
    FormatMarkdown    OutputFormat = "markdown"
    FormatHTML        OutputFormat = "html"
    FormatJSON        OutputFormat = "json"
    FormatImagePrompt OutputFormat = "image_prompt"
    FormatCode        OutputFormat = "code"
)

type Output struct {
    SkillType Type
    EntryType string
    Format    OutputFormat
    Content   string
    // 格式相关附加信息（如代码语言、图片尺寸规格等）
    Metadata  map[string]any
    // LLM 调用统计，用于成本追踪
    LLMUsage  *LLMUsage
}

type LLMUsage struct {
    Provider     string
    Model        string
    InputTokens  int
    OutputTokens int
    DurationMs   int64
    CostUSD      float64  // 估算，按 Provider 官方费率计算
}
```

### 1.4 Skill 实现模板

```go
// internal/skill/research.go

type ResearchSkill struct {
    llm llm.Provider
}

func NewResearchSkill(llm llm.Provider) *ResearchSkill {
    return &ResearchSkill{llm: llm}
}

func (s *ResearchSkill) Type() Type              { return TypeResearch }
func (s *ResearchSkill) SupportedEntries() []string { return []string{"documents", "sheets"} }
func (s *ResearchSkill) Dependencies() []Type    { return nil } // Research 无前置依赖

func (s *ResearchSkill) Execute(ctx context.Context, input Input) (*Output, error) {
    prompt, err := s.buildPrompt(input)
    if err != nil {
        return nil, fmt.Errorf("build prompt: %w", err)
    }

    resp, err := s.llm.Complete(ctx, llm.CompletionRequest{
        SystemPrompt: systemPrompt,
        UserPrompt:   prompt,
        Model:        "gpt-4o",
        MaxTokens:    4096,
        Temperature:  0.3,
    })
    if err != nil {
        return nil, fmt.Errorf("llm complete: %w", err)
    }

    content, err := s.parseResponse(resp.Content)
    if err != nil {
        // 解析失败时返回原始内容，不中断流程
        content = resp.Content
    }

    return &Output{
        SkillType: s.Type(),
        EntryType: input.EntryType,
        Format:    FormatMarkdown,
        Content:   content,
        LLMUsage:  toLLMUsage(resp.Usage),
    }, nil
}
```

## 2. LLM Provider 抽象

### 2.1 Provider 接口

```go
// internal/llm/provider.go

type Provider interface {
    // Complete 单轮补全
    Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
    // Stream 流式补全（用于实时输出场景）
    Stream(ctx context.Context, req CompletionRequest, handler StreamHandler) error
    // Name 返回 Provider 标识
    Name() string
}

type CompletionRequest struct {
    SystemPrompt string
    UserPrompt   string
    Model        string
    MaxTokens    int
    Temperature  float64
    // 结构化输出（JSON Schema），支持的 Provider 返回结构化 JSON
    ResponseSchema *json.RawMessage
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
```

### 2.2 多 Provider 路由（中期）

V0.1 直接使用 OpenAI，中期引入路由层，按 Skill 类型或成本策略选择 Provider：

```go
// internal/llm/router.go

type RoutedProvider struct {
    routes   map[string]Provider   // skill_type -> provider
    fallback Provider
}

func (r *RoutedProvider) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
    provider := r.selectProvider(req)
    return provider.Complete(ctx, req)
}

// 路由规则示例：
// - Research/Strategy：GPT-4o（高质量）
// - Copy/Deck：GPT-4o-mini（成本优化）
// - Visual（图片提示词）：Claude 3.5 Sonnet（创意性强）
// - Dev（代码）：GPT-4o 或 Claude 3.5 Sonnet
```

## 3. Prompt 管理规范

### 3.1 Prompt 存储位置

每个 Skill 的 Prompt 存放在对应 Skill 文件的同目录下，便于版本管理：

```
backend/internal/skill/
├── research.go
├── research_prompt.go   # Prompt 模板定义
├── strategy.go
├── strategy_prompt.go
└── ...
```

### 3.2 Prompt 结构规范

```go
// internal/skill/research_prompt.go

const researchSystemPrompt = `你是一位专业的商业市场研究分析师，
擅长通过有限信息对目标市场做出清晰、有依据的分析。

输出规范：
- 语言：中文
- 格式：Markdown，使用标题（##）和列表
- 长度：1500-2500 字
- 必须明确标注推断性内容："（基于用户输入推断）"或"（建议进一步验证）"
- 禁止编造具体数据（如市场规模数字、转化率等），可提供范围估算并说明来源假设`

func buildResearchPrompt(input Input) (string, error) {
    tmpl := template.Must(template.New("research").Parse(`
行业：{{.Brief.Industry}}
目标市场：{{.Brief.Market}}
目标用户：{{.Brief.Audience}}
项目目标：{{.Brief.Objective}}
核心卖点：{{.Brief.ValueProposition}}

请基于以上信息，完成市场调研报告，包含：
1. 目标用户画像与核心需求
2. 竞争格局与差异化机会
3. 市场切入建议
4. 主要风险与假设`))

    var buf bytes.Buffer
    if err := tmpl.Execute(&buf, input); err != nil {
        return "", err
    }
    return buf.String(), nil
}
```

### 3.3 Prompt 版本化

- Prompt 变更通过 Git 历史追踪版本。
- 生产环境的 Prompt 变更需要先在 staging 运行 eval 测试通过后才能合并。
- 重大 Prompt 重写（System Prompt 结构变化）需要新增 Skill 版本，并在 `skill_registry` 中更新 `version` 字段。

## 4. 可观测性

### 4.1 结构化日志

每次 Skill 执行记录结构化日志，便于排查和成本分析：

```go
slog.InfoContext(ctx, "skill_executed",
    "skill_type", s.Type(),
    "project_id", input.ProjectID,
    "task_id", input.TaskID,
    "entry_type", input.EntryType,
    "llm_provider", resp.Usage.Provider,
    "llm_model", resp.Usage.Model,
    "input_tokens", resp.Usage.InputTokens,
    "output_tokens", resp.Usage.OutputTokens,
    "duration_ms", durationMs,
    "cost_usd", estimatedCost,
    "status", "success",
)
```

### 4.2 成本追踪

每次 LLM 调用的 token 用量和估算成本记录到 `tasks.input_snapshot`（JSONB），便于后期统计每个项目的 AI 成本：

```json
{
  "llm_calls": [
    {
      "skill_type": "research",
      "provider": "openai",
      "model": "gpt-4o",
      "input_tokens": 800,
      "output_tokens": 2000,
      "duration_ms": 12500,
      "cost_usd": 0.042
    }
  ]
}
```

### 4.3 中期：Prometheus 指标

中期接入 Prometheus，暴露以下指标：

```
mosaic_skill_executions_total{skill_type, status}          # Skill 执行次数
mosaic_skill_duration_seconds{skill_type}                  # Skill 执行耗时直方图
mosaic_llm_tokens_total{provider, model, type}             # Token 用量
mosaic_llm_cost_usd_total{provider, model}                 # 累计 LLM 成本
mosaic_project_completion_rate{status}                     # 项目完成率
```

## 5. 幂等性与重试

### 5.1 Skill 幂等设计

每个 Skill 的 Execute 必须幂等：相同 `task_id` 重复调用返回相同结果（或重新生成并覆盖）。

实现方式：执行前检查 `tasks` 表，若 `status = done` 且 `input_snapshot` 未变化，直接返回已存在的 `deliverable` 内容，不重复调用 LLM。

### 5.2 LLM API 重试策略

```go
// internal/llm/openai.go

func (c *OpenAIClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
    var lastErr error
    for attempt := 0; attempt < 3; attempt++ {
        if attempt > 0 {
            // 指数退避：1s, 2s, 4s
            select {
            case <-time.After(time.Duration(1<<attempt) * time.Second):
            case <-ctx.Done():
                return nil, ctx.Err()
            }
        }

        resp, err := c.client.CreateChatCompletion(ctx, toOpenAIRequest(req))
        if err == nil {
            return toCompletionResponse(resp), nil
        }

        // 仅对限流和服务器错误重试
        if isRetryable(err) {
            lastErr = err
            continue
        }
        return nil, err  // 非可重试错误立即返回
    }
    return nil, fmt.Errorf("after 3 attempts: %w", lastErr)
}

func isRetryable(err error) bool {
    var apiErr *openai.APIError
    if errors.As(err, &apiErr) {
        return apiErr.HTTPStatusCode == 429 || apiErr.HTTPStatusCode >= 500
    }
    return false
}
```

## 6. 内容安全与事实标注

### 6.1 输出后处理

每个 Skill 在返回内容前，运行统一的后处理管道：

```go
func PostProcess(content string, format OutputFormat) string {
    content = sanitizeContent(content)         // 移除敏感信息（API Key、手机号等）
    content = addFactualDisclaimer(content)    // 添加事实标注
    return content
}
```

### 6.2 事实标注规则

Skill 输出的以下类型内容必须自动标注：

| 内容类型 | 标注方式 |
| --- | --- |
| 市场规模、增长率等数据 | 添加"（数据为估算，建议参考权威来源验证）" |
| 竞品分析结论 | 添加"（基于公开信息推断，建议实地调研）" |
| 用户画像描述 | 添加"（基于行业通用认知，建议用户调研验证）" |
| 收益/ROI 预测 | 添加"（仅为参考，非真实经营数据）" |

## 7. Skill 评测（Eval）流程

### 7.1 评测目的

确保 Prompt 变更、模型升级、Skill 重构不导致输出质量下降。

### 7.2 评测套件结构

```
backend/internal/skill/testdata/
├── research/
│   ├── case_01_store_opening.json    # 输入
│   ├── case_01_expected.md           # 期望输出（关键特征）
│   └── case_02_brand_launch.json
├── strategy/
└── copy/
```

### 7.3 评测运行

```bash
# 运行所有 Skill 的评测（需要真实 LLM API Key）
go test ./internal/skill/... -run TestSkillEval -v -eval

# 只运行特定 Skill
go test ./internal/skill/... -run TestSkillEval/research -v -eval
```

评测使用 LLM-as-Judge 模式：用 GPT-4o 对输出内容评分（相关性、格式合规性、事实标注覆盖率），分数低于阈值时 CI 失败。

## 8. AI 辅助编码规范

在用 AI（Cursor / Claude / Copilot）辅助开发 Mosaic 时，遵循以下规范：

### 8.1 Skill 开发

- AI 生成新 Skill 时，必须包含：`Type()`、`Dependencies()`、`SupportedEntries()`、`Execute()` 四个方法。
- AI 生成 Prompt 时，必须在 System Prompt 中包含语言（中文）、格式、长度、事实标注规则四项。
- AI 不得直接硬编码 API Key 或任何敏感值。

### 8.2 数据库操作

- AI 生成 SQL 时，必须包含对应的 `-- +goose Down` 回滚语句。
- AI 生成的迁移文件需要人工 review 后才能提交。

### 8.3 API 接口

- AI 生成新接口时，必须先更新 `openapi.yaml`，再生成实现代码。
- AI 不得手写客户端类型，必须通过代码生成。

### 8.4 测试

- AI 辅助生成的代码，必须同时生成对应的单元测试。
- AI 生成的测试不得 mock 数据库（使用测试数据库代替）。
