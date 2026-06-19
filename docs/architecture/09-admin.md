# 09 管理后台（Admin）

## 1. 定位与边界

Admin 管理后台是 Mosaic 的系统级控制台，供**系统管理员**使用，与面向终端用户的主应用（`apps/web`）完全独立部署。

**核心边界**：

| 维度 | 主应用（apps/web） | 管理后台（apps/admin） |
| --- | --- | --- |
| 使用者 | 普通用户、服务商、企业用户 | 系统管理员（system_admin 角色） |
| 数据视角 | 只看自己的数据 | 看全部租户的数据 |
| 核心操作 | 创建项目、查看交付物 | 管理用户、审核内容、配置系统、监控成本 |
| 认证入口 | `/login` | `/admin/login`（独立，同 JWT 体系） |
| API 前缀 | `/api/v1/` | `/admin/api/v1/` |

**共享的内容**：
- 共用 `packages/ui` 组件库（表格、表单、按钮等基础组件）
- 共用 `packages/api-client` 的认证逻辑（JWT 获取/刷新）
- Admin 专属 API 类型通过 OpenAPI Spec 的 `x-admin: true` 标注，单独生成 Admin 客户端

## 2. Monorepo 位置

```
apps/
├── web/          # 主应用（用户侧）
├── desktop/      # Tauri 桌面壳
├── mobile/       # Flutter（预留）
└── admin/        # 管理后台（新增）
    ├── src/
    │   ├── routes/
    │   │   ├── dashboard/        # 系统概览
    │   │   ├── users/            # 用户管理
    │   │   ├── projects/         # 项目监控
    │   │   ├── skills/           # Skill 注册表管理
    │   │   ├── llm-cost/         # LLM 成本仪表盘
    │   │   ├── content-review/   # 内容审核
    │   │   └── system/           # 系统配置
    │   ├── components/           # Admin 专属组件
    │   ├── stores/               # Zustand 状态
    │   └── main.tsx
    ├── package.json
    └── vite.config.ts
```

Admin 应用依赖：

```json
{
  "dependencies": {
    "@mosaic/ui": "workspace:*",
    "@mosaic/api-client": "workspace:*",
    "@tanstack/react-table": "^8.x",
    "@tanstack/react-query": "^5.x",
    "recharts": "^2.x"
  }
}
```

## 3. Go 后端 Admin 路由组

Admin API 在 Go 后端中独立挂载，使用更严格的中间件链。

```
/admin/api/v1/
    ├── /auth/login          # Admin 登录（同 JWT 体系，验证 system_admin 角色）
    ├── /dashboard           # 系统总览数据
    ├── /users               # 用户管理
    ├── /projects            # 全量项目查询与管理
    ├── /skills              # Skill 注册表 CRUD
    ├── /llm/costs           # LLM 成本统计
    ├── /content/reviews     # 内容审核队列
    └── /system/config       # 系统配置
```

### 路由注册

```go
// cmd/server/main.go

// 主应用路由（用户侧）
r.Group(func(r chi.Router) {
    r.Use(middleware.RequireAuth)
    handler.RegisterUserRoutes(r, ...)
})

// Admin 路由（独立中间件链）
r.Route("/admin/api/v1", func(r chi.Router) {
    r.Use(middleware.RequireAuth)
    r.Use(middleware.RequireRole("system_admin"))  // 严格限制角色
    r.Use(middleware.AdminAuditLog)                // 记录所有 Admin 操作
    handler.RegisterAdminRoutes(r, adminSvc)
})
```

### Admin 专属中间件：操作审计日志

Admin 的所有写操作（创建、修改、删除）必须记录审计日志：

```go
// internal/handler/middleware/admin_audit.go

func AdminAuditLog(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodGet {
            claims := getUserClaims(r.Context())
            slog.InfoContext(r.Context(), "admin_action",
                "admin_user_id", claims.Sub,
                "method",        r.Method,
                "path",          r.URL.Path,
                "ip",            r.RemoteAddr,
                "user_agent",    r.Header.Get("User-Agent"),
            )
        }
        next.ServeHTTP(w, r)
    })
}
```

## 4. Admin 功能模块

### 4.1 系统总览仪表盘

实时展示系统核心指标，用于日常巡检。

**数据指标**：

| 指标 | 说明 |
| --- | --- |
| 今日新增用户数 | 按注册时间统计 |
| 今日创建项目数 | 按创建时间统计 |
| 今日生成交付物数 | 按交付物完成时间统计 |
| Skill 执行成功率（近 24h） | `done / (done + failed)` |
| LLM 今日费用（USD） | 累加 `tasks.input_snapshot` 中的 cost_usd |
| 系统健康状态 | 后端 / 数据库 / 对象存储 / LLM 连通性 |

**API**：

```
GET /admin/api/v1/dashboard
```

```json
{
  "data": {
    "users_today": 12,
    "projects_today": 45,
    "deliverables_today": 312,
    "skill_success_rate_24h": 0.94,
    "llm_cost_today_usd": 8.42,
    "system_health": {
      "database": "ok",
      "storage": "ok",
      "llm": "ok"
    }
  }
}
```

### 4.2 用户管理

**功能列表**：
- 查询所有用户（分页、按角色/注册时间过滤、搜索 email）
- 查看用户详情（项目数、交付物数、消费金额）
- 修改用户角色（user / service_provider / enterprise_admin / system_admin）
- 禁用 / 启用账户
- 强制退出（吊销所有 Refresh Token）

**核心接口**：

```
GET    /admin/api/v1/users                  # 用户列表（分页）
GET    /admin/api/v1/users/{id}             # 用户详情
PATCH  /admin/api/v1/users/{id}/role        # 修改角色
PATCH  /admin/api/v1/users/{id}/status      # 启用/禁用
POST   /admin/api/v1/users/{id}/logout      # 强制退出
```

### 4.3 项目监控

**功能列表**：
- 查询全量项目（跨用户、跨租户）
- 按状态过滤（running / done / failed）
- 查看失败项目详情（失败原因、失败的 Skill、错误日志）
- 手动触发失败项目重试
- 查看某个项目的全部任务和交付物

**核心接口**：

```
GET    /admin/api/v1/projects               # 全量项目列表
GET    /admin/api/v1/projects/{id}          # 项目详情（含任务和交付物）
POST   /admin/api/v1/projects/{id}/retry    # 手动重试失败项目
DELETE /admin/api/v1/projects/{id}          # 删除项目（含交付物和对象存储文件）
```

### 4.4 Skill 注册表管理

**功能列表**：
- 查看全部内置和自定义 Skill
- 修改 Skill 状态（available / beta / deprecated）
- 更新 Skill 的 `supported_entries` 配置
- 查看每个 Skill 的执行统计（调用次数、平均耗时、成功率、平均 Token 消耗）
- V1+ 阶段：审核自定义 Skill 上传申请

**核心接口**：

```
GET    /admin/api/v1/skills                 # Skill 列表（含统计）
PATCH  /admin/api/v1/skills/{id}/status     # 修改 Skill 状态
GET    /admin/api/v1/skills/{id}/stats      # Skill 执行统计
```

**Skill 统计数据结构**（聚合自 `tasks` 表）：

```sql
SELECT
    skill_type,
    COUNT(*) as total_executions,
    COUNT(*) FILTER (WHERE status = 'done') as success_count,
    AVG(EXTRACT(EPOCH FROM (completed_at - started_at)) * 1000) as avg_duration_ms,
    SUM((input_snapshot->'llm_calls'->0->>'input_tokens')::int) as total_input_tokens,
    SUM((input_snapshot->'llm_calls'->0->>'cost_usd')::float) as total_cost_usd
FROM tasks
WHERE created_at > NOW() - INTERVAL '30 days'
GROUP BY skill_type;
```

### 4.5 LLM 成本仪表盘

**功能列表**：
- 按日/周/月查看 LLM 总费用趋势图
- 按 Provider / Model / Skill 类型分解费用
- 查看费用最高的 Top 10 项目
- 设置日费用告警阈值（超出后邮件/Webhook 通知）

**前端图表**：使用 `recharts` 库渲染趋势折线图和费用分解饼图。

```
GET /admin/api/v1/llm/costs?period=7d&group_by=skill_type
```

```json
{
  "data": {
    "period": "7d",
    "total_usd": 58.42,
    "breakdown": [
      { "dimension": "research", "cost_usd": 18.2, "executions": 420 },
      { "dimension": "strategy", "cost_usd": 15.1, "executions": 380 },
      { "dimension": "copy",     "cost_usd": 12.8, "executions": 510 }
    ],
    "daily_trend": [
      { "date": "2026-06-13", "cost_usd": 7.2 },
      { "date": "2026-06-14", "cost_usd": 9.1 }
    ]
  }
}
```

### 4.6 内容审核

**功能列表**：
- 查看被系统标记为疑似违规的交付物（V1+ 接入内容安全 API 后）
- 人工标记：通过 / 删除 / 屏蔽
- 查看用户举报记录

**核心接口**：

```
GET    /admin/api/v1/content/reviews        # 待审核队列
PATCH  /admin/api/v1/content/reviews/{id}   # 处理审核（approve/reject）
```

### 4.7 系统配置

**功能列表**：
- 查看和修改 LLM Provider 路由规则（哪个 Skill 用哪个模型）
- 修改各接口的速率限制阈值
- 查看当前环境变量（敏感值脱敏展示，不可修改）
- 管理公告 / 维护窗口

**设计原则**：系统配置存入数据库（而不是只靠环境变量），支持运行时热更新，不需要重启服务。

## 5. 数据库补充

Admin 功能需要在现有表基础上新增 2 张表：

### admin_audit_logs（审计日志）

```sql
CREATE TABLE admin_audit_logs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_user_id UUID NOT NULL REFERENCES users(id),
    action       TEXT NOT NULL,    -- 操作描述：update_user_role, disable_user, ...
    resource     TEXT NOT NULL,    -- 操作对象类型：user, project, skill
    resource_id  TEXT,             -- 操作对象 ID
    detail       JSONB,            -- 操作详情（修改前/后的值）
    ip           TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_admin_audit_logs_admin_user ON admin_audit_logs(admin_user_id);
CREATE INDEX idx_admin_audit_logs_created_at ON admin_audit_logs(created_at);
```

### system_configs（系统配置）

```sql
CREATE TABLE system_configs (
    key         TEXT PRIMARY KEY,
    value       JSONB NOT NULL,
    description TEXT,
    updated_by  UUID REFERENCES users(id),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 初始化默认配置
INSERT INTO system_configs (key, value, description) VALUES
('llm.routing', '{"research":"gpt-4o","strategy":"gpt-4o","copy":"gpt-4o-mini"}', 'Skill -> LLM 模型路由规则'),
('rate_limit.project_creation', '{"per_user_per_minute": 3}', '项目创建速率限制'),
('llm.cost_alert_threshold_usd', '100', '日费用告警阈值（USD）');
```

## 6. Admin 前端页面结构

```
apps/admin/src/routes/
├── dashboard/
│   └── page.tsx              # 系统总览（指标卡 + 健康状态）
├── users/
│   ├── page.tsx              # 用户列表（可搜索分页表格）
│   └── [id]/page.tsx         # 用户详情
├── projects/
│   ├── page.tsx              # 项目列表（状态过滤 + 分页）
│   └── [id]/page.tsx         # 项目详情（任务树 + 交付物列表）
├── skills/
│   ├── page.tsx              # Skill 列表（含统计）
│   └── [id]/page.tsx         # Skill 详情与状态管理
├── llm-cost/
│   └── page.tsx              # 成本仪表盘（趋势图 + 分解图）
├── content-review/
│   └── page.tsx              # 审核队列
└── system/
    ├── config/page.tsx       # 系统配置
    └── audit-logs/page.tsx   # 审计日志
```

### 核心组件

```typescript
// Admin 专属 DataTable（基于 @tanstack/react-table）
// 支持：分页、排序、过滤、行选中批量操作
<DataTable
  columns={userColumns}
  data={users}
  pagination={pagination}
  onPaginationChange={setPagination}
  filters={[
    { key: 'role', label: '角色', options: ['user', 'service_provider', 'enterprise_admin'] },
    { key: 'status', label: '状态', options: ['active', 'disabled'] },
  ]}
/>
```

## 7. Admin 部署

Admin 应用作为独立的 Docker 容器部署，与主应用的 Nginx 隔离：

```yaml
# docker-compose.prod.yml 补充

admin:
  image: ghcr.io/mosaic-app/admin:${VERSION}
  restart: always
  # Admin 不对外暴露，通过内网访问或 VPN
  # 生产环境建议只允许公司 IP 或 VPN 访问
```

**访问控制**：生产环境的 Admin 界面通过 IP 白名单或 VPN 限制访问，不对公网开放。

## 8. 与其他文档的关联

| 文档 | 关联内容 |
| --- | --- |
| [00-overview.md](00-overview.md) | Monorepo 结构已更新包含 apps/admin |
| [03-api-contract.md](03-api-contract.md) | /admin/api/v1/ 路由约定 |
| [04-data-model.md](04-data-model.md) | admin_audit_logs、system_configs 两张新表 |
| [08-security.md](08-security.md) | system_admin 角色、Admin 操作审计日志 |
| [07-deployment-release.md](07-deployment-release.md) | Admin 容器独立部署、IP 白名单 |
