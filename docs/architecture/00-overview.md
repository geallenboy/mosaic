# 00 总体架构概览

## 1. 愿景到系统的映射

Mosaic 万象集的产品目标是"云端 AI 团队"——用户说清楚目标，系统拆解任务、并行执行、多入口产出、保证风格统一。这个目标直接决定了系统最核心的工程挑战：

| 产品目标 | 工程挑战 |
| --- | --- |
| 多入口并行产出 | Skill 并发调度与结果汇聚 |
| 风格统一的跨入口内容 | 项目母版说明书作为单一上下文源 |
| 用户实时看到执行进度 | SSE 流式状态推送 |
| Web + 桌面 + 移动多端 | OpenAPI 契约驱动 + 平台适配抽象 |
| 从 0 到 1 快速验证 | 最小可运行架构，分阶段演进 |

## 2. 分层架构

```
┌─────────────────────────────────────────────────────┐
│                    客户端层                           │
│   React Web     Tauri 桌面壳      Flutter（预留）     │
│       └────────────┴─────────────────┘              │
│              共享层（Monorepo packages）               │
│   packages/ui  packages/api-client  packages/platform│
└─────────────────────┬───────────────────────────────┘
                      │ HTTPS / SSE
┌─────────────────────▼───────────────────────────────┐
│                    Go 后端                            │
│  HTTP Handler → 应用服务层 → 项目工作流编排器           │
│                              │                       │
│                     Skill 运行时（并发调度）            │
│                              │                       │
│              Repository 层 ──┤                       │
└──────────────┬───────────────┼───────────────────────┘
               │               │
        PostgreSQL         LLM Provider 抽象
        对象存储            （OpenAI / Anthropic / 本地）
        任务队列（中期）
```

## 3. 技术栈选型

### 3.1 后端

| 组件 | 技术选择 | 选型理由 |
| --- | --- | --- |
| 语言 | Go 1.22+ | goroutine 原生支持并发调度，标准库完善，部署产物单二进制 |
| HTTP 框架 | [chi](https://github.com/go-chi/chi) | 轻量、标准库兼容、中间件生态好，比 gin/echo 更易测试 |
| ORM / 查询 | [sqlc](https://sqlc.dev/) + 原生 SQL | 类型安全、无魔法、SQL 即文档，配合迁移工具使用 |
| 数据库迁移 | [goose](https://github.com/pressly/goose) | SQL 文件即迁移记录，版本管理清晰 |
| 配置管理 | [viper](https://github.com/spf13/viper) + 环境变量 | 支持多格式，12-Factor App 兼容 |
| 任务队列（中期） | [Asynq](https://github.com/hibiken/asynq) | Redis-backed，Go 原生，UI 监控面板开箱即用 |
| SSE | 标准库 `http.Flusher` | 无额外依赖，足够简单 |
| 测试 | `testing` + [testify](https://github.com/stretchr/testify) | 标准 + 断言增强 |

### 3.2 前端 / 多端

| 端 | 技术选择 | 选型理由 |
| --- | --- | --- |
| Web | React 18 + Vite + TypeScript | 生态最成熟，与 Tauri 共享代码无摩擦 |
| 组件库 | [shadcn/ui](https://ui.shadcn.com/) + Tailwind CSS | 无样式锁定，组件可完全自定义 |
| 状态管理 | [Zustand](https://zustand-demo.pmnd.rs/) | 轻量，无模板代码 |
| 数据请求 | [TanStack Query](https://tanstack.com/query) | 服务端状态缓存、乐观更新、后台刷新 |
| 桌面壳 | [Tauri 2.x](https://tauri.app/) | Rust 系统层 + WebView 复用 React 代码，比 Electron 包体积小 90% |
| 移动端 | [Flutter 3.x](https://flutter.dev/) | 单套代码覆盖 iOS + Android，V1+ 阶段实现 |

### 3.3 API 契约

| 组件 | 技术选择 | 选型理由 |
| --- | --- | --- |
| 规范格式 | OpenAPI 3.1 YAML | 单一事实源，驱动多端代码生成 |
| Go 服务端生成 | [ogen](https://ogen.dev/) | 类型安全的 Go 服务端代码生成，无反射 |
| TypeScript 客户端生成 | [openapi-typescript](https://github.com/drwpow/openapi-typescript) + openapi-fetch | 零运行时开销的类型生成 |
| Dart 客户端生成 | [openapi-generator](https://openapi-generator.tech/) (dart-dio) | 官方支持，与 Flutter 搭配成熟 |

### 3.4 基础设施

| 组件 | V0.1 | 中期 | 后期 |
| --- | --- | --- | --- |
| 数据库 | PostgreSQL 16（Docker） | PostgreSQL（托管，如 Supabase / RDS） | 同上 + 读副本 |
| 对象存储 | MinIO（Docker） | S3 兼容云存储 | 同上 + CDN |
| 任务队列 | — | Asynq + Redis | Asynq + Redis Cluster |
| 容器编排 | Docker Compose | Docker Compose / Fly.io / Railway | Kubernetes |
| 监控 | 结构化日志 | Prometheus + Grafana + Sentry | 同上 + 链路追踪 |
| LLM | OpenAI API | 多 Provider 路由 | 本地模型 + 云端混合 |

## 4. Monorepo 结构

```
mosaic/
├── apps/
│   ├── web/                  # React Web（Vite）
│   │   ├── src/
│   │   ├── package.json
│   │   └── vite.config.ts
│   ├── desktop/              # Tauri 桌面壳（Rust 层极薄）
│   │   ├── src-tauri/        # Rust 原生能力
│   │   │   ├── src/main.rs
│   │   │   └── tauri.conf.json
│   │   └── package.json      # 指向 apps/web 的 UI
│   ├── admin/                # 管理后台（React，复用 packages/ui）
│   │   ├── src/
│   │   │   ├── routes/       # dashboard/users/projects/skills/llm-cost/...
│   │   │   └── main.tsx
│   │   └── package.json
│   └── mobile/               # Flutter（预留目录，V1+ 实现）
│       └── lib/
├── packages/
│   ├── ui/                   # 共享 React 组件库（shadcn/ui 基础）
│   ├── api-client/           # 从 OpenAPI Spec 自动生成，Web + Tauri 共用
│   └── platform/             # PlatformAdapter 抽象层（屏蔽 Web/Tauri 差异）
├── backend/                  # Go 后端
│   ├── cmd/
│   │   └── server/main.go
│   ├── internal/
│   │   ├── handler/          # HTTP 处理层
│   │   ├── service/          # 应用服务层
│   │   ├── orchestrator/     # 项目工作流编排器
│   │   ├── skill/            # Skill 运行时与各 Skill 实现
│   │   ├── repository/       # 数据访问层
│   │   └── llm/              # LLM Provider 抽象
│   ├── api/
│   │   └── openapi.yaml      # OpenAPI 单一事实源
│   ├── db/
│   │   └── migrations/       # goose SQL 迁移文件
│   └── go.mod
├── docs/
│   ├── product-requirements.md
│   └── architecture/         # 本目录
├── .github/
│   └── workflows/            # CI/CD Pipeline
├── docker-compose.yml        # V0.1 本地开发与部署
├── docker-compose.prod.yml   # 生产环境 Compose
└── pnpm-workspace.yaml       # Monorepo 包管理（pnpm）
```

## 5. 三阶段演进路线

### 前期：V0.1（门店场景闭环）

目标：跑通从"用户输入" → "项目母版说明书" → "Skill 并行执行" → "交付中心展示"的完整闭环。

- 激活入口：Documents、Slides
- 激活 Skill：Research、Strategy、Copy、Deck
- Skill 调度：in-process goroutine + `errgroup`
- 部署：Docker Compose（单机）
- 数据库：PostgreSQL（本地 Docker）
- 对象存储：MinIO（本地 Docker）
- LLM：OpenAI API（直连）

### 中期：V0.2 - V0.3（全入口扩展）

目标：覆盖全部 8 个入口，引入任务持久化，支持长时间执行的 Skill，完善可观测性。

- 新增入口：Sheets、Images、Videos、Podcasts、Skypage、AI Developer
- 新增 Skill：Visual、Video、Web、Ops、Dev
- Skill 调度：迁移到 Asynq + Redis（任务可持久化、重试、监控）
- 部署：托管云平台（Fly.io / Railway）或容器编排
- LLM：抽象多 Provider，支持路由切换
- 监控：Prometheus + Grafana + Sentry 接入

### 后期：V1.0+（商业化与扩展）

目标：多租户、服务商白标、Skill Hub 开放、弹性伸缩。

- 多租户 RBAC（个人/服务商/企业）
- Skill Hub 自定义 Skill 上传与管理
- Flutter 移动端全量实现
- 弹性伸缩（Kubernetes 或 Serverless）
- 本地模型 + 云端混合路由（成本优化）

## 6. 环境策略

| 环境 | 用途 | 域名约定 | 数据库 |
| --- | --- | --- | --- |
| local | 本地开发，Docker Compose 启动 | localhost | 独立实例 |
| dev | CI 集成测试，自动部署每个 PR | dev.mosaic.app | 独立实例，每次重置 |
| staging | 预发布验证，与生产数据结构一致 | staging.mosaic.app | 定期从生产快照同步（脱敏后） |
| prod | 生产环境 | mosaic.app | 生产数据库（带备份） |

所有环境通过环境变量区分，不存在"通过代码判断环境"的逻辑。

## 7. 关键架构原则

1. **OpenAPI 优先**：接口变更先改 `backend/api/openapi.yaml`，再由 CI 生成多端代码，禁止手写客户端类型。
2. **Skill 无状态**：每个 Skill 执行时只依赖输入参数和 LLM，不持有内存状态，保证可重试和可横向扩展。
3. **项目母版说明书是单一上下文**：所有 Skill 输入必须来自项目母版说明书，不允许 Skill 之间直接传递状态（通过调度器协调）。
4. **前端零密钥**：任何 API Key、Token、LLM 密钥禁止出现在前端代码或浏览器可访问的位置。
5. **分阶段引入复杂度**：V0.1 用最简单的方案跑通，不提前引入任务队列、K8s、多 Provider 等中后期组件。
