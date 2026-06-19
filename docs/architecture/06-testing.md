# 06 测试策略

## 1. 测试金字塔

```
          ┌─────────────────┐
          │   E2E 测试       │  少量，覆盖关键用户旅程
          │  (Playwright)   │  慢，有真实 LLM 调用
          ├─────────────────┤
          │  集成测试         │  中量，测试跨层交互
          │  (Go / TS)      │  使用真实数据库，mock LLM
          ├─────────────────┤
          │  单元测试         │  大量，测试单个函数/组件
          │  (Go / React)   │  快，完全隔离
          └─────────────────┘
```

**目标覆盖率**：
- Go 后端核心业务逻辑（service、orchestrator、skill）：> 80%
- Go Repository 层：> 70%（集成测试覆盖）
- React 组件：> 60%（关键交互逻辑）
- E2E：覆盖 3 个核心用户旅程（创建项目、查看交付中心、导出）

## 2. Go 后端测试

### 2.1 单元测试

测试 service、orchestrator、skill 的业务逻辑，依赖通过接口 mock。

**命名规范**：`{被测函数}_{场景}_{期望结果}`

```go
// internal/orchestrator/orchestrator_test.go

func TestOrchestrator_StartProject_Success(t *testing.T) {
    // Arrange
    mockSkillRegistry := skill.NewMockRegistry()
    mockTaskRepo := repository.NewMockTaskRepo()
    mockDeliverableRepo := repository.NewMockDeliverableRepo()
    orch := orchestrator.New(mockSkillRegistry, mockTaskRepo, mockDeliverableRepo, nil)

    req := orchestrator.StartProjectRequest{
        ProjectID: "test-project-id",
        Brief:     testBrief(),
        SelectedEntries: []string{"documents", "slides"},
        UserID: "test-user-id",
    }

    // Act
    projectID, err := orch.StartProject(context.Background(), req)

    // Assert
    require.NoError(t, err)
    assert.NotEmpty(t, projectID)
    assert.Equal(t, 1, mockTaskRepo.CreateCallCount())  // 至少创建了任务
}
```

### 2.2 Skill 单元测试（Mock LLM）

```go
// internal/skill/research_test.go

func TestResearchSkill_Execute_WithValidInput(t *testing.T) {
    mockLLM := llm.NewMockProvider()
    mockLLM.On("Complete", mock.Anything, mock.Anything).Return(&llm.CompletionResponse{
        Content: "## 市场分析\n\n目标用户：...",
        Usage: llm.TokenUsage{InputTokens: 500, OutputTokens: 1500},
    }, nil)

    s := skill.NewResearchSkill(mockLLM)
    output, err := s.Execute(context.Background(), skill.Input{
        ProjectBrief: testBrief(),
        EntryType:    "documents",
    })

    require.NoError(t, err)
    assert.Equal(t, skill.FormatMarkdown, output.Format)
    assert.Contains(t, output.Content, "##")
    assert.NotEmpty(t, output.LLMUsage)
}

func TestResearchSkill_Execute_LLMError_ReturnsError(t *testing.T) {
    mockLLM := llm.NewMockProvider()
    mockLLM.On("Complete", mock.Anything, mock.Anything).Return(nil, errors.New("rate limited"))

    s := skill.NewResearchSkill(mockLLM)
    _, err := s.Execute(context.Background(), skill.Input{ProjectBrief: testBrief()})

    require.Error(t, err)
}
```

### 2.3 集成测试（真实数据库）

集成测试使用 [testcontainers-go](https://github.com/testcontainers/testcontainers-go) 启动真实 PostgreSQL 实例，测试 repository 层和数据库交互：

```go
// internal/repository/project_repo_integration_test.go
// +build integration

func TestProjectRepo_CreateAndGet(t *testing.T) {
    ctx := context.Background()
    db := setupTestDB(t)  // testcontainers 启动 Postgres，测试结束自动清理

    repo := repository.NewProjectRepo(db)

    // 创建项目
    project, err := repo.Create(ctx, repository.CreateProjectParams{
        UserID: testUserID,
        Name:   "测试项目",
        Type:   "store_opening",
        Goal:   "测试目标",
        SelectedEntries: []string{"documents"},
    })
    require.NoError(t, err)
    assert.NotEmpty(t, project.ID)

    // 查询项目
    found, err := repo.GetByID(ctx, project.ID)
    require.NoError(t, err)
    assert.Equal(t, project.ID, found.ID)
    assert.Equal(t, "测试项目", found.Name)
}

// 复用的数据库启动辅助函数
func setupTestDB(t *testing.T) *sql.DB {
    t.Helper()
    container, err := postgres.RunContainer(context.Background(),
        testcontainers.WithImage("postgres:16"),
        postgres.WithDatabase("mosaic_test"),
    )
    require.NoError(t, err)
    t.Cleanup(func() { container.Terminate(context.Background()) })

    dsn, _ := container.ConnectionString(context.Background(), "sslmode=disable")
    db, err := sql.Open("pgx", dsn)
    require.NoError(t, err)

    // 运行迁移
    goose.Up(db, "../../db/migrations")
    return db
}
```

运行集成测试：

```bash
go test ./... -tags integration -count=1
```

### 2.4 HTTP Handler 测试

Handler 测试使用 `httptest`，mock service 层：

```go
// internal/handler/project_handler_test.go

func TestProjectHandler_CreateProject_ValidRequest(t *testing.T) {
    mockSvc := service.NewMockProjectService()
    mockSvc.On("CreateProject", mock.Anything, mock.Anything).Return(&service.ProjectResult{
        ID: "proj-123",
    }, nil)

    h := handler.NewProjectHandler(mockSvc)
    r := chi.NewRouter()
    r.Post("/api/v1/projects", h.CreateProject)

    body := `{"name":"测试","type":"store_opening","goal":"目标","selected_entries":["documents"]}`
    req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()

    r.ServeHTTP(w, req)

    assert.Equal(t, http.StatusCreated, w.Code)
    var resp map[string]any
    json.Unmarshal(w.Body.Bytes(), &resp)
    assert.Equal(t, "proj-123", resp["data"].(map[string]any)["id"])
}
```

## 3. 前端测试

### 3.1 组件单元测试（Vitest + React Testing Library）

```typescript
// apps/web/src/components/entries/DocumentEntry.test.tsx

describe('DocumentEntry', () => {
  it('renders deliverable content in markdown', () => {
    const deliverable = { title: '市场调研报告', content: '## 分析\n\n内容', format: 'markdown' }
    render(<DocumentEntry deliverable={deliverable} />)
    expect(screen.getByText('市场调研报告')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '分析' })).toBeInTheDocument()
  })

  it('shows regenerate button when status is failed', () => {
    const deliverable = { ...base, status: 'failed' }
    render(<DocumentEntry deliverable={deliverable} />)
    expect(screen.getByRole('button', { name: /重新生成/ })).toBeInTheDocument()
  })
})
```

### 3.2 Hook 测试

```typescript
// hooks/useProjectSSE.test.ts

describe('useProjectSSE', () => {
  it('updates project status on SSE progress event', async () => {
    server.use(
      http.get('/api/v1/projects/:id/progress', ({ request }) => {
        return new Response(
          'event: progress\ndata: {"phase":"skill_execution","progress":50}\n\n',
          { headers: { 'Content-Type': 'text/event-stream' } }
        )
      })
    )

    const { result } = renderHook(() => useProjectSSE('proj-123'))
    await waitFor(() => expect(result.current.progress).toBe(50))
  })
})
```

### 3.3 API 契约测试

验证前端 API 调用与 OpenAPI Spec 一致：

```typescript
// packages/api-client/src/contract.test.ts

// 使用 openapi-fetch 的类型系统，TypeScript 编译期即可发现契约不匹配
// 以下测试确保运行时也符合 Spec
it('createProject request matches OpenAPI schema', async () => {
  const req: CreateProjectRequest = {
    name: '测试',
    type: 'store_opening',
    goal: '目标',
    selected_entries: ['documents'],
  }
  // TypeScript 类型检查：如果 openapi.yaml 改了，这里编译就会报错
  const result = await client.POST('/api/v1/projects', { body: req })
  expect(result.response.status).toBe(201)
})
```

## 4. E2E 测试（Playwright）

覆盖 3 个核心用户旅程，每次 PR 在 staging 环境运行。

### 4.1 测试套件

```
e2e/
├── tests/
│   ├── create-project.spec.ts    # 创建项目完整流程
│   ├── delivery-center.spec.ts   # 查看交付中心
│   └── export-deliverable.spec.ts # 导出交付物
├── fixtures/
│   └── project-inputs.json       # 测试用项目输入数据
└── playwright.config.ts
```

### 4.2 创建项目 E2E 示例

```typescript
// e2e/tests/create-project.spec.ts

test('用户可以创建门店开业营销项目并看到交付中心', async ({ page }) => {
  // 登录
  await page.goto('/login')
  await page.fill('[name=email]', 'test@mosaic.app')
  await page.fill('[name=password]', 'test_password')
  await page.click('[type=submit]')

  // 创建项目
  await page.goto('/workspace')
  await page.click('text=创建项目')
  await page.selectOption('[name=type]', 'store_opening')
  await page.fill('[name=name]', '深圳轻食餐厅开业')
  await page.fill('[name=goal]', '在深圳开一家轻食餐厅，目标客户是附近白领')
  await page.check('input[value=documents]')
  await page.check('input[value=slides]')
  await page.click('text=开始生成')

  // 等待执行完成（最长 120 秒）
  await page.waitForSelector('[data-testid=delivery-center]', { timeout: 120_000 })

  // 验证交付物
  await expect(page.locator('[data-entry=documents]')).toBeVisible()
  await expect(page.locator('[data-entry=slides]')).toBeVisible()
  await expect(page.locator('text=市场调研')).toBeVisible()
})
```

**E2E 测试策略**：
- E2E 测试使用专用测试账户，不污染生产数据。
- 测试中调用真实 LLM（用量小，可控），确保端到端流程真实有效。
- 每次 PR 不运行 E2E，只在合并到 main 后的 staging 环境运行。

## 5. Skill 评测测试

详见 [05-ai-skill-standards.md](05-ai-skill-standards.md) 第 7 节。

评测测试独立于普通测试套件，通过 `-eval` 构建标签触发，需要真实 LLM API Key：

```bash
# 只在 CI 的特定 job 中运行，不阻塞普通 PR
go test ./internal/skill/... -tags eval -run TestSkillEval
```

## 6. 覆盖率门禁

CI 中强制执行覆盖率检查，低于阈值则 PR 不可合并：

```yaml
# .github/workflows/test.yml
- name: Check coverage
  run: |
    go test ./internal/... -coverprofile=coverage.out
    go tool cover -func=coverage.out | grep total | awk '{print $3}' | \
      awk '{if ($1+0 < 70) {print "Coverage below 70%: "$1; exit 1}}'
```

| 层 | 最低覆盖率 |
| --- | --- |
| service/ | 80% |
| orchestrator/ | 80% |
| skill/ | 75% |
| repository/ | 70%（集成测试覆盖） |
| handler/ | 70% |

## 7. 测试数据管理

- 单元测试：使用代码内 `testBrief()`、`testProject()` 等 helper 函数构造测试数据，不依赖外部文件。
- 集成测试：使用 testcontainers 启动隔离数据库，测试结束自动清理，不留残余数据。
- E2E 测试：使用固定测试账户，项目数据在每次测试后通过 API 清理。
- 禁止在测试中硬编码真实用户数据、真实 API Key 或生产环境 URL。
