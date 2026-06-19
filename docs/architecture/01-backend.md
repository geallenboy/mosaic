# 01 Go 后端架构

## 1. 分层设计

Go 后端采用严格的单向依赖分层，每层只依赖其下方的层。

```
HTTP Request
     │
┌────▼────────────────────────────────┐
│  handler/          HTTP 处理层        │  路由、参数解析、请求验证、响应格式化
└────┬────────────────────────────────┘
     │
┌────▼────────────────────────────────┐
│  service/          应用服务层          │  业务逻辑、事务协调、权限检查
└────┬────────────────────────────────┘
     │              │
┌────▼──────┐  ┌────▼────────────────────┐
│repository/│  │orchestrator/  编排器层    │  项目工作流管理、Skill 调度
│ 数据访问层 │  └────┬────────────────────┘
└────┬──────┘       │
     │         ┌────▼────────────────────┐
  PostgreSQL   │  skill/    Skill 运行时   │  LLM 调用、输出解析、结果写入
               └────┬────────────────────┘
                    │
               ┌────▼────┐
               │  llm/   │  LLM Provider 抽象
               └─────────┘
```

### 目录结构

```
backend/
├── cmd/
│   └── server/
│       └── main.go           # 启动入口：注册路由、依赖注入、启动 HTTP Server
├── internal/
│   ├── handler/              # HTTP 处理层
│   │   ├── project.go
│   │   ├── deliverable.go
│   │   ├── skill.go
│   │   └── sse.go            # SSE 流式进度端点
│   ├── service/              # 应用服务层
│   │   ├── project_svc.go
│   │   └── deliverable_svc.go
│   ├── orchestrator/         # 工作流编排器
│   │   ├── orchestrator.go   # 核心编排逻辑
│   │   ├── graph.go          # Skill 依赖图构建
│   │   └── executor.go       # 并发执行器
│   ├── skill/                # Skill 运行时
│   │   ├── runtime.go        # Skill 接口定义与注册
│   │   ├── research.go       # Research Skill 实现
│   │   ├── strategy.go
│   │   ├── copy.go
│   │   ├── deck.go
│   │   ├── visual.go
│   │   ├── web.go
│   │   ├── video.go
│   │   ├── ops.go
│   │   └── dev.go
│   ├── repository/           # 数据访问层（sqlc 生成 + 手写查询）
│   │   ├── project_repo.go
│   │   ├── task_repo.go
│   │   └── deliverable_repo.go
│   ├── llm/                  # LLM Provider 抽象
│   │   ├── provider.go       # Provider 接口定义
│   │   ├── openai.go         # OpenAI 实现
│   │   └── anthropic.go      # Anthropic 实现（中期）
│   └── config/
│       └── config.go         # 配置结构体，viper 加载
├── api/
│   └── openapi.yaml          # OpenAPI 3.1 单一事实源
├── db/
│   └── migrations/           # goose SQL 迁移文件
│       ├── 001_init.sql
│       └── 002_skills.sql
└── go.mod
```

## 2. 项目工作流编排器

编排器是后端最核心的组件，负责把"用户提交项目"转化为"一批 Skill 任务并发执行"。

### 2.1 编排流程

```
用户提交项目
     │
     ▼
[1] 生成项目母版说明书（BriefGenerator）
     │  调用 LLM 生成结构化 Brief
     ▼
[2] 确定激活入口（EntryResolver）
     │  根据用户选择的交付物，确定激活哪些 Entry
     ▼
[3] 构建 Skill 依赖图（DependencyGraph）
     │  Research → Strategy → Copy/Deck/Visual/...
     ▼
[4] 并发执行（Executor）
     │  拓扑排序，无依赖的 Skill 同时执行
     ▼
[5] 汇聚结果（ResultAggregator）
     │  每个 Skill 完成后写入 Deliverable
     ▼
[6] 一致性检查（ConsistencyChecker）
     │  品牌名/语气/目标用户跨入口对齐
     ▼
[7] 发布交付中心（DeliveryCenterPublisher）
```

### 2.2 核心接口定义

```go
// internal/orchestrator/orchestrator.go

type Orchestrator interface {
    // StartProject 启动项目执行，异步，立即返回 projectID
    StartProject(ctx context.Context, req StartProjectRequest) (projectID string, err error)
    // GetStatus 查询当前执行状态（供 SSE 拉取）
    GetStatus(ctx context.Context, projectID string) (*ProjectStatus, error)
}

type StartProjectRequest struct {
    ProjectID      string
    Brief          *ProjectBrief
    SelectedEntries []EntryType
    UserID         string
}

type ProjectStatus struct {
    ProjectID string
    Phase     Phase           // brief_generation | skill_execution | consistency_check | done | failed
    Tasks     []TaskStatus
    Progress  int             // 0-100
}
```

### 2.3 Skill 依赖图

不同 Skill 之间存在依赖关系：Strategy Skill 需要 Research Skill 的输出结果才能执行，其他 Skill 可以并行。

```
Research ──┐
           ▼
        Strategy ──┬──> Copy
                   ├──> Deck
                   ├──> Visual
                   ├──> Video
                   ├──> Web
                   ├──> Ops
                   └──> Dev
```

依赖图通过拓扑排序确定执行顺序：

```go
// internal/orchestrator/graph.go

type SkillNode struct {
    SkillType    skill.Type
    Dependencies []skill.Type
}

// DefaultDependencies 定义默认依赖关系
var DefaultDependencies = map[skill.Type][]skill.Type{
    skill.TypeStrategy: {skill.TypeResearch},
    skill.TypeCopy:     {skill.TypeStrategy},
    skill.TypeDeck:     {skill.TypeStrategy},
    skill.TypeVisual:   {skill.TypeStrategy},
    skill.TypeVideo:    {skill.TypeCopy},
    skill.TypeWeb:      {skill.TypeStrategy, skill.TypeCopy},
    skill.TypeOps:      {skill.TypeStrategy},
    skill.TypeDev:      {skill.TypeResearch},
}
```

## 3. Skill 并发调度

### 3.1 V0.1：in-process goroutine + errgroup

V0.1 阶段，所有 Skill 在同一 Go 进程内并发执行，使用 `golang.org/x/sync/errgroup` 管理。

```go
// internal/orchestrator/executor.go

func (e *Executor) ExecuteLayer(ctx context.Context, skills []skill.Skill) error {
    g, ctx := errgroup.WithContext(ctx)

    for _, s := range skills {
        s := s // 捕获循环变量
        g.Go(func() error {
            result, err := s.Execute(ctx, e.input)
            if err != nil {
                // 写入失败状态，不中断其他 Skill
                e.updateTaskStatus(s.Type(), TaskStatusFailed, err.Error())
                return nil // 非致命错误不传播
            }
            e.saveDeliverable(s.Type(), result)
            e.updateTaskStatus(s.Type(), TaskStatusDone, "")
            return nil
        })
    }

    return g.Wait()
}
```

**V0.1 的局限**：进程重启会丢失正在执行的任务。对 V0.1（执行时间 < 60s 的场景）可接受。

### 3.2 中期：Asynq 持久化任务队列

中期引入 Asynq（Redis-backed），解决任务持久化和长时执行问题。

迁移策略：
1. Orchestrator 接口不变（调用方无感知）。
2. 在 `executor.go` 中把 goroutine 替换为 Asynq task enqueue。
3. 增加独立的 Worker 进程消费任务。
4. SSE 进度推送改为订阅 Redis Pub/Sub。

```go
// 中期：Executor 改为 Asynq 入队
func (e *AsynqExecutor) ExecuteLayer(ctx context.Context, skills []skill.Skill) error {
    for _, s := range skills {
        payload, _ := json.Marshal(SkillTaskPayload{
            ProjectID: e.projectID,
            SkillType: s.Type(),
            Input:     e.input,
        })
        task := asynq.NewTask(TaskTypeSkillExecute, payload)
        _, err := e.client.Enqueue(task, asynq.MaxRetry(3), asynq.Timeout(5*time.Minute))
        if err != nil {
            return err
        }
    }
    return nil
}
```

## 4. Skill 接口规范

每个 Skill 实现以下接口：

```go
// internal/skill/runtime.go

type Type string

const (
    TypeResearch Type = "research"
    TypeStrategy Type = "strategy"
    TypeCopy     Type = "copy"
    TypeDeck     Type = "deck"
    TypeVisual   Type = "visual"
    TypeWeb      Type = "web"
    TypeVideo    Type = "video"
    TypeOps      Type = "ops"
    TypeDev      Type = "dev"
)

type Input struct {
    ProjectBrief   *ProjectBrief
    PreviousOutputs map[Type]*Output  // 依赖 Skill 的输出
    EntryType      string
    UserContext    map[string]any    // 用户自定义参数
}

type Output struct {
    SkillType   Type
    EntryType   string
    Format      OutputFormat        // markdown | html | json | image_prompt | code
    Content     string
    Metadata    map[string]any
}

type Skill interface {
    Type() Type
    Execute(ctx context.Context, input Input) (*Output, error)
    // SupportedEntries 返回该 Skill 服务的入口列表
    SupportedEntries() []string
}
```

## 5. SSE 流式进度推送

客户端通过 SSE 连接实时获取 Skill 执行进度，无需轮询。

```go
// internal/handler/sse.go

func (h *SSEHandler) ProjectProgress(w http.ResponseWriter, r *http.Request) {
    projectID := chi.URLParam(r, "projectID")

    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "SSE not supported", http.StatusInternalServerError)
        return
    }

    ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-r.Context().Done():
            return
        case <-ticker.C:
            status, err := h.orchestrator.GetStatus(r.Context(), projectID)
            if err != nil {
                fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
                flusher.Flush()
                return
            }
            data, _ := json.Marshal(status)
            fmt.Fprintf(w, "event: progress\ndata: %s\n\n", data)
            flusher.Flush()

            if status.Phase == PhaseDone || status.Phase == PhaseFailed {
                fmt.Fprintf(w, "event: close\ndata: {}\n\n")
                flusher.Flush()
                return
            }
        }
    }
}
```

## 6. 错误处理与重试策略

### 错误分类

| 错误类型 | 处理策略 |
| --- | --- |
| LLM API 限流（429） | 指数退避重试，最多 3 次 |
| LLM API 超时 | 重试一次，超过后标记 Skill 失败，不中断其他 Skill |
| LLM 输出解析失败 | 重新生成一次，记录日志，返回原始内容 |
| 数据库写入失败 | 事务回滚，任务标记为失败 |
| 网络中断 | 中期迁移到 Asynq 后自动重试 |

### Skill 失败不级联

单个 Skill 失败不终止整个项目执行。失败的 Skill 标记为 `failed`，其他 Skill 继续执行。最终交付中心展示已成功的交付物，失败的入口显示"重新生成"按钮。

## 7. 配置管理

所有配置通过环境变量注入，不存在硬编码配置。

```go
// internal/config/config.go

type Config struct {
    Server   ServerConfig
    Database DatabaseConfig
    Storage  StorageConfig
    LLM      LLMConfig
    Auth     AuthConfig
}

type LLMConfig struct {
    Provider   string  // openai | anthropic
    APIKey     string  // 从环境变量读取，禁止硬编码
    Model      string
    MaxTokens  int
    TimeoutSec int
}
```

`.env` 文件用于本地开发，不提交到代码仓库（`.gitignore` 覆盖）。生产环境通过容器环境变量或密钥管理服务注入（见 [08-security.md](08-security.md)）。

## 8. 依赖注入

使用构造函数手动注入，不引入 DI 框架，保持代码可读性与可测试性。

```go
// cmd/server/main.go

func main() {
    cfg := config.Load()
    db := database.Connect(cfg.Database)
    objStore := storage.NewS3Client(cfg.Storage)
    llmProvider := llm.NewOpenAI(cfg.LLM)

    projectRepo := repository.NewProjectRepo(db)
    taskRepo := repository.NewTaskRepo(db)
    deliverableRepo := repository.NewDeliverableRepo(db)

    skillRegistry := skill.NewRegistry(llmProvider)
    orchestrator := orchestrator.New(skillRegistry, taskRepo, deliverableRepo, objStore)
    projectSvc := service.NewProjectService(projectRepo, orchestrator)

    r := chi.NewRouter()
    handler.RegisterRoutes(r, projectSvc, orchestrator)

    http.ListenAndServe(cfg.Server.Addr, r)
}
```
