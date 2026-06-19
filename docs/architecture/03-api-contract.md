# 03 API 契约

## 1. 设计原则

**OpenAPI 3.1 YAML 是唯一事实源。** 任何接口变更必须先修改 `backend/api/openapi.yaml`，再由 CI 自动生成各端代码。禁止手工编写客户端类型或绕过 OpenAPI 直接对齐接口。

### 为什么选 OpenAPI 而不是 gRPC / GraphQL

| 方案 | 适合场景 | Mosaic 的判断 |
| --- | --- | --- |
| OpenAPI + REST | Web/移动端消费，HTTP 语义清晰 | 选用：多端客户端，HTTP 生态工具完善 |
| gRPC | 服务间通信，低延迟，强类型 | 不选：前端无原生 gRPC 支持，增加复杂度 |
| GraphQL | 复杂嵌套查询，前端按需取数据 | 不选：Mosaic 数据模型层次不深，REST 足够 |

## 2. 文件位置与代码生成链路

```
backend/api/openapi.yaml          ← 唯一事实源，手工维护
        │
        ├── Go 服务端     →  ogen 生成  →  backend/internal/handler/oas_gen/
        ├── TypeScript    →  openapi-typescript 生成  →  packages/api-client/src/
        └── Dart          →  openapi-generator 生成  →  apps/mobile/lib/api_client/
```

CI 在每次 PR 中自动运行生成，生成代码提交到仓库（允许 diff），保证与 openapi.yaml 始终一致。

### 生成命令（封装在 Makefile）

```makefile
.PHONY: gen-api

gen-api:
    # Go 服务端（handler 骨架 + 类型）
    ogen --target backend/internal/handler/oas_gen \
         --package oas_gen \
         backend/api/openapi.yaml

    # TypeScript 客户端（类型 + fetch 封装）
    npx openapi-typescript backend/api/openapi.yaml \
        --output packages/api-client/src/schema.ts

    # Dart 客户端（Flutter 用）
    openapi-generator generate \
        -i backend/api/openapi.yaml \
        -g dart-dio \
        -o apps/mobile/lib/api_client
```

## 3. URL 规范

系统有两套 API 前缀，分别服务于不同调用方：

| 前缀 | 服务对象 | 认证要求 |
| --- | --- | --- |
| `/api/v1/` | 主应用（Web/桌面/移动端用户侧） | JWT，任意已登录用户 |
| `/admin/api/v1/` | 管理后台 | JWT，需要 `system_admin` 角色 |

### 用户侧接口（/api/v1/）

```
GET    /api/v1/projects                    # 获取项目列表（只返回自己的）
POST   /api/v1/projects                    # 创建项目
GET    /api/v1/projects/{id}               # 获取项目详情
PATCH  /api/v1/projects/{id}               # 更新项目
DELETE /api/v1/projects/{id}               # 删除项目
POST   /api/v1/projects/{id}/start         # 启动项目执行
GET    /api/v1/projects/{id}/progress      # SSE：实时执行进度
GET    /api/v1/projects/{id}/deliverables  # 获取交付物列表
PATCH  /api/v1/deliverables/{id}           # 更新交付物
POST   /api/v1/deliverables/{id}/regenerate # 重新生成交付物
GET    /api/v1/skills                      # 获取 Skill 列表（Skill Hub）
```

### Admin 接口（/admin/api/v1/）

```
GET    /admin/api/v1/dashboard             # 系统总览指标
GET    /admin/api/v1/users                 # 全量用户列表（分页）
GET    /admin/api/v1/users/{id}            # 用户详情
PATCH  /admin/api/v1/users/{id}/role       # 修改用户角色
PATCH  /admin/api/v1/users/{id}/status     # 启用/禁用账户
POST   /admin/api/v1/users/{id}/logout     # 强制退出
GET    /admin/api/v1/projects              # 全量项目列表（跨用户）
GET    /admin/api/v1/projects/{id}         # 项目详情
POST   /admin/api/v1/projects/{id}/retry   # 手动重试失败项目
DELETE /admin/api/v1/projects/{id}         # 删除项目
GET    /admin/api/v1/skills                # Skill 列表（含统计）
PATCH  /admin/api/v1/skills/{id}/status    # 修改 Skill 状态
GET    /admin/api/v1/skills/{id}/stats     # Skill 执行统计
GET    /admin/api/v1/llm/costs             # LLM 成本统计
GET    /admin/api/v1/content/reviews       # 内容审核队列
PATCH  /admin/api/v1/content/reviews/{id}  # 处理审核
GET    /admin/api/v1/system/configs        # 系统配置列表
PATCH  /admin/api/v1/system/configs/{key}  # 修改系统配置
GET    /admin/api/v1/system/audit-logs     # 操作审计日志
```

Admin API 同样通过 OpenAPI Spec 定义，Admin 客户端独立生成：

```makefile
# Makefile 补充
gen-admin-api:
    npx openapi-typescript backend/api/admin-openapi.yaml \
        --output packages/api-client/src/admin-schema.ts
```

## 4. 统一响应信封

所有响应统一包裹在信封结构中，方便客户端统一处理。

### 成功响应

```json
{
  "data": { ... },
  "meta": {
    "request_id": "req_abc123",
    "timestamp": "2026-01-01T00:00:00Z"
  }
}
```

### 错误响应

```json
{
  "error": {
    "code": "PROJECT_NOT_FOUND",
    "message": "项目不存在或已被删除",
    "detail": { ... }
  },
  "meta": {
    "request_id": "req_abc123",
    "timestamp": "2026-01-01T00:00:00Z"
  }
}
```

### 分页响应

```json
{
  "data": [ ... ],
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total": 100,
    "has_next": true
  },
  "meta": { ... }
}
```

## 5. 核心接口定义

以下为 `openapi.yaml` 的关键片段，作为设计参考：

```yaml
openapi: "3.1.0"
info:
  title: Mosaic API
  version: "1.0.0"

paths:
  /api/v1/projects:
    post:
      operationId: createProject
      summary: 创建项目
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/CreateProjectRequest'
      responses:
        '201':
          description: 项目创建成功
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ProjectResponse'
        '422':
          $ref: '#/components/responses/ValidationError'

  /api/v1/projects/{id}/start:
    post:
      operationId: startProject
      summary: 启动项目 Skill 执行
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
            format: uuid
      responses:
        '202':
          description: 项目开始执行（异步）
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ProjectStatusResponse'

  /api/v1/projects/{id}/progress:
    get:
      operationId: getProjectProgress
      summary: SSE 实时进度流
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
            format: uuid
      responses:
        '200':
          description: SSE 事件流
          content:
            text/event-stream:
              schema:
                type: string

components:
  schemas:
    CreateProjectRequest:
      type: object
      required: [name, type, industry, goal, selected_entries]
      properties:
        name:
          type: string
        type:
          type: string
          enum: [store_opening, brand_launch, product_launch, custom]
        industry:
          type: string
        market:
          type: string
        target_audience:
          type: string
        goal:
          type: string
        budget_range:
          type: string
        style_keywords:
          type: array
          items:
            type: string
        selected_entries:
          type: array
          items:
            type: string
            enum: [documents, slides, sheets, images, videos, podcasts, skypage, ai_developer]

    DeliverableResponse:
      type: object
      properties:
        id:
          type: string
          format: uuid
        project_id:
          type: string
          format: uuid
        entry:
          type: string
          enum: [documents, slides, sheets, images, videos, podcasts, skypage, ai_developer]
        surface:
          type: string
        type:
          type: string
        title:
          type: string
        format:
          type: string
          enum: [markdown, html, json, image_prompt, code]
        content:
          type: string
        version:
          type: integer
        status:
          type: string
          enum: [pending, generating, done, failed]

  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT

security:
  - BearerAuth: []
```

## 6. 鉴权约定

所有需要认证的接口在 Header 中携带 JWT：

```
Authorization: Bearer <token>
```

以下接口无需认证：

- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`

JWT Payload 约定：

```json
{
  "sub": "user_uuid",
  "role": "user | service_provider | enterprise_admin",
  "tenant_id": "tenant_uuid",
  "exp": 1234567890
}
```

详见 [08-security.md](08-security.md)。

## 7. 版本策略

- 当前所有接口在 `/api/v1/` 下。
- 破坏性变更（删除字段、修改必填项）需要新增 `/api/v2/` 版本，旧版本维护不少于 3 个月。
- 新增字段（非破坏性）可在当前版本直接添加，客户端应忽略未知字段。
- 版本升级通过 `Deprecation` Header 提前通知客户端。

## 8. 错误码规范

错误码格式：`{RESOURCE}_{ERROR_TYPE}`，全大写，下划线分隔。

| 错误码 | HTTP 状态 | 含义 |
| --- | --- | --- |
| `PROJECT_NOT_FOUND` | 404 | 项目不存在 |
| `PROJECT_ALREADY_STARTED` | 409 | 项目已在执行中 |
| `DELIVERABLE_NOT_FOUND` | 404 | 交付物不存在 |
| `SKILL_EXECUTION_FAILED` | 500 | Skill 执行失败 |
| `VALIDATION_ERROR` | 422 | 请求参数校验失败 |
| `UNAUTHORIZED` | 401 | 未认证 |
| `FORBIDDEN` | 403 | 无权限 |
| `RATE_LIMITED` | 429 | 请求频率超限 |
| `INTERNAL_ERROR` | 500 | 服务器内部错误 |
