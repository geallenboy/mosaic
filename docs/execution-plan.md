# Mosaic 执行计划

> 更新于：2026-06-19
> 当前阶段：V0.1 基础搭建完成，进入 MVP 补全阶段

---

## 当前基线（已完成）

| 模块 | 状态 | 说明 |
|------|------|------|
| 产品需求文档（PRD） | ✅ | AI Workspace Agents，8大入口 + Skill Hub |
| 技术架构文档（10份） | ✅ | 覆盖后端、客户端、API、数据模型、AI标准、测试、部署、安全、管理后台 |
| Monorepo 脚手架 | ✅ | apps/web、apps/admin、packages/ui/api-client/platform |
| Go 后端 V0.1 核心 | ✅ | LLM抽象、Skill运行时、并发编排器、SSE进度推送 |
| OpenAPI 契约 V0.1 | ✅ | 用户侧接口（认证、项目、交付物、Skill） |
| 数据库迁移文件 | ✅ | users、projects、tasks、deliverables、skill_registry 等核心表 |
| Docker Compose 本地环境 | ✅ | backend + web + admin + postgres + minio |
| GitHub 仓库 + 工作流规范 | ✅ | master + dev，分支/提交/PR规范 |

---

## 里程碑总览

```
V0.1 MVP（当前）─── V0.2 Web 可用 ─── V0.3 Admin 上线 ─── V1.0 公测
    ↓                    ↓                   ↓                  ↓
后端能跑通          用户能用页面         运营能管理          全功能上线
```

---

## 阶段一：V0.1 MVP 补全（优先级最高）

**目标**：后端能端到端跑通一个真实项目，可 curl 验证，无 UI 也可演示。

**预计规模**：小，1~2 人/天可完成

### 任务清单

#### 1.1 数据库层接入
- [ ] 在 Go 后端集成 `pgx` 或 `database/sql`，连接 PostgreSQL
- [ ] 实现 `ProjectRepository`：`Create`、`UpdateStatus`、`GetByID`
- [ ] 实现 `TaskRepository`：批量写入任务状态
- [ ] 实现 `DeliverableRepository`：保存 Skill 输出内容
- [ ] 替换当前 Orchestrator 中的内存状态为数据库持久化

**验收标准**：重启服务后，历史项目进度可查询；不再丢失进度状态。

#### 1.2 认证模块
- [ ] 实现 `POST /api/v1/auth/register`（邮箱+密码）
- [ ] 实现 `POST /api/v1/auth/login`，返回 JWT access + refresh token
- [ ] 实现 JWT 中间件，保护项目类接口
- [ ] 实现 `POST /api/v1/auth/refresh`

**验收标准**：curl 注册、登录、带 token 创建项目全链路通过。

#### 1.3 项目接口完善
- [ ] `POST /api/v1/projects/start` 关联当前登录用户
- [ ] `GET /api/v1/projects` 返回用户项目列表（分页）
- [ ] `GET /api/v1/projects/{id}` 返回项目详情 + 当前状态
- [ ] `GET /api/v1/projects/{id}/progress` SSE 实时进度（现有，需补 auth）
- [ ] `GET /api/v1/deliverables/{id}` 返回单个交付物内容

**验收标准**：Postman 能走完注册→登录→创建项目→轮询进度→查看交付物完整流程。

#### 1.4 LLM Key 配置
- [ ] 确保 `.env` 里 `LLM_API_KEY` 生效，`config.go` 正确读取
- [ ] 至少走通一次真实 LLM 调用（不用 mock），输出有意义的内容

**验收标准**：`curl POST /api/v1/projects/start`，5分钟内 SSE 推送4个 Skill 完成，数据库有记录。

---

## 阶段二：V0.2 Web 端可用

**目标**：用户可以通过网页完成完整的项目创建→进度跟踪→查看交付物流程。

**预计规模**：中，前端工作为主

### 任务清单

#### 2.1 前端基础设施
- [ ] `apps/web` 集成 React Router（路由配置：登录、工作台、项目详情）
- [ ] 集成 Tailwind CSS + `@mosaic/ui` 共享组件库起步（Button、Input、Card）
- [ ] 集成 `@tanstack/react-query` 管理 API 请求状态
- [ ] 集成 `@mosaic/api-client`（由 openapi.yaml 生成类型）

#### 2.2 认证页面
- [ ] 登录页（邮箱+密码，错误提示）
- [ ] 注册页
- [ ] 路由守卫（未登录跳转到登录页）
- [ ] Token 存储（localStorage）+ 自动 refresh 逻辑

#### 2.3 工作台首页
- [ ] 项目列表展示（分页/无限滚动）
- [ ] 「新建项目」入口：填写项目名 + Brief 表单
- [ ] 项目卡片：显示名称、状态、创建时间

#### 2.4 项目详情页
- [ ] 项目基本信息展示
- [ ] 4个 Skill 任务进度卡片（Research、Strategy、Copy、Deck）
- [ ] 实时进度条（SSE 接入）
- [ ] 交付物展示区（Markdown 渲染）

**验收标准**：完整用户流程可在浏览器跑通，无控制台报错。

---

## 阶段三：V0.3 Admin 管理后台

**目标**：运营/管理员能通过后台管理用户、查看系统状态、管理 Skill 配置。

### 任务清单

#### 3.1 Admin API（后端）
- [ ] `GET /admin/api/v1/users`：用户列表（分页、搜索）
- [ ] `PATCH /admin/api/v1/users/{id}`：启用/禁用账号
- [ ] `GET /admin/api/v1/projects`：全量项目监控
- [ ] `GET /admin/api/v1/skills`：Skill 注册列表
- [ ] `GET /admin/api/v1/stats/llm-cost`：LLM token 消耗统计

#### 3.2 Admin 前端（`apps/admin`）
- [ ] Admin 独立登录页（系统管理员账号）
- [ ] 用户管理页面
- [ ] 项目监控页面
- [ ] Skill 注册/禁用管理
- [ ] LLM 费用看板（简版）

**验收标准**：管理员能登录后台，查看用户列表，禁用账号，查看项目状态。

---

## 阶段四：V1.0 公测准备

**目标**：产品可对外展示，核心体验闭环，支持更多 Skill 类型。

### 任务清单

#### 4.1 更多 Skill 接入
- [ ] Visual Skill（调用图像生成 API，输出品牌视觉素材）
- [ ] Web Skill（调用 Skypage 能力，生成落地页 HTML）
- [ ] 扩展「门店开业」场景的完整 Skill 链路（6~8个 Skill）

#### 4.2 产品体验优化
- [ ] 项目模板（选择场景 → 预填 Brief）
- [ ] 交付物下载（PDF / HTML 导出）
- [ ] 错误重试入口（单个 Skill 失败可重试）

#### 4.3 基础设施
- [ ] 接入云数据库（PlanetScale / Supabase / RDS）
- [ ] 静态资产上传到 S3/R2
- [ ] 配置生产环境 CI/CD（GitHub Actions → 云服务器部署）
- [ ] 基础监控（错误日志 + LLM 费用告警）

#### 4.4 内容与品牌
- [ ] 产品官网（单页，介绍核心价值）
- [ ] 注册 waitlist 页面
- [ ] 录制产品 Demo 视频

---

## 优先级矩阵

| 任务 | 影响 | 难度 | 优先级 |
|------|------|------|--------|
| 数据库层接入（1.1） | 高 | 中 | 🔴 最高 |
| 认证模块（1.2） | 高 | 低 | 🔴 最高 |
| LLM 真实调用验证（1.4） | 高 | 低 | 🔴 最高 |
| 项目接口完善（1.3） | 高 | 低 | 🔴 最高 |
| 前端基础设施（2.1） | 高 | 中 | 🟠 高 |
| 项目详情 + SSE（2.4） | 高 | 中 | 🟠 高 |
| Admin API（3.1） | 中 | 中 | 🟡 中 |
| 更多 Skill（4.1） | 中 | 中 | 🟡 中 |
| 产品官网（4.4） | 中 | 低 | 🟡 中 |

---

## 分支开发规划

按阶段对应分支：

| 分支 | 对应阶段 |
|------|---------|
| `feat/db-repository` | 1.1 数据库层 |
| `feat/auth-jwt` | 1.2 认证模块 |
| `feat/project-api-v1` | 1.3 项目接口 |
| `feat/web-foundation` | 2.1 前端基础设施 |
| `feat/web-auth-pages` | 2.2 认证页面 |
| `feat/web-workspace` | 2.3 + 2.4 工作台 |
| `feat/admin-api` | 3.1 Admin API |
| `feat/admin-web` | 3.2 Admin 前端 |

每个分支独立开发，PR 合入 `dev`，里程碑节点 PR 合入 `master`。

---

## 当前建议下一步

> **立即执行**：`feat/db-repository` + `feat/auth-jwt`（可并行）

这两个任务完成后，V0.1 才算真正"能跑"——有持久化、有认证、有完整接口，可以开始前端对接。
