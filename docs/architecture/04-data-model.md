# 04 数据模型

## 1. 核心实体关系图

```
┌──────────┐       ┌──────────────┐
│  users   │──1:N──│   projects   │
└──────────┘       └──────┬───────┘
                           │ 1:1
                   ┌───────▼───────┐
                   │ project_briefs│  项目母版说明书
                   └───────────────┘
                           │ 1:N（来自同一项目）
                   ┌───────▼───────┐
                   │    tasks      │  Skill 执行任务
                   └───────┬───────┘
                           │ 1:1
                   ┌───────▼───────┐
                   │ deliverables  │  交付物
                   └───────────────┘

┌──────────────────┐
│  skill_registry  │  内置/自定义 Skill 元数据
└──────────────────┘
```

## 2. 表设计

### 2.1 users

```sql
CREATE TABLE users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email       TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    role        TEXT NOT NULL DEFAULT 'user',  -- user | service_provider | enterprise_admin
    tenant_id   UUID,                           -- 企业用户所属租户（V1+）
    avatar_url  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_tenant_id ON users(tenant_id);
```

### 2.2 projects

```sql
CREATE TABLE projects (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    type             TEXT NOT NULL,             -- store_opening | brand_launch | custom | ...
    industry         TEXT,
    market           TEXT,
    target_audience  TEXT,
    goal             TEXT NOT NULL,
    budget_range     TEXT,
    style_keywords   TEXT[],                    -- PostgreSQL 数组
    selected_entries TEXT[] NOT NULL,           -- [documents, slides, ...]
    status           TEXT NOT NULL DEFAULT 'draft',
                                                -- draft | running | done | failed | archived
    error_message    TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_projects_user_id ON projects(user_id);
CREATE INDEX idx_projects_status ON projects(status);
```

### 2.3 project_briefs（项目母版说明书）

```sql
CREATE TABLE project_briefs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id        UUID NOT NULL UNIQUE REFERENCES projects(id) ON DELETE CASCADE,
    objective         TEXT NOT NULL,
    audience          TEXT NOT NULL,
    positioning       TEXT NOT NULL,
    value_proposition TEXT NOT NULL,
    tone_of_voice     TEXT NOT NULL,
    visual_direction  TEXT NOT NULL,
    key_messages      TEXT[],
    assumptions       TEXT[],
    constraints       TEXT[],
    raw_json          JSONB,                    -- 完整 LLM 输出，便于调试和重新解析
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### 2.4 tasks（Skill 执行任务）

```sql
CREATE TABLE tasks (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id    UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    skill_type    TEXT NOT NULL,               -- research | strategy | copy | deck | ...
    title         TEXT NOT NULL,
    description   TEXT,
    status        TEXT NOT NULL DEFAULT 'pending',
                                               -- pending | running | done | failed
    input_snapshot JSONB,                      -- 执行时的输入快照，便于重试和审计
    error_message  TEXT,
    retry_count    INT NOT NULL DEFAULT 0,
    started_at    TIMESTAMPTZ,
    completed_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tasks_project_id ON tasks(project_id);
CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_skill_type ON tasks(skill_type);
```

### 2.5 deliverables（交付物）

```sql
CREATE TABLE deliverables (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    task_id     UUID REFERENCES tasks(id) ON DELETE SET NULL,
    entry       TEXT NOT NULL,
                -- documents | slides | sheets | images | videos | podcasts | skypage | ai_developer
    surface     TEXT,                          -- 入口内的子类型，如 documents 下的 market_research
    type        TEXT NOT NULL,                 -- 交付物业务类型
    title       TEXT NOT NULL,
    format      TEXT NOT NULL,                 -- markdown | html | json | image_prompt | code
    content     TEXT,                          -- 文本内容（小内容直接存库）
    storage_key TEXT,                          -- 对象存储路径（大内容存对象存储）
    version     INT NOT NULL DEFAULT 1,
    status      TEXT NOT NULL DEFAULT 'pending',
                -- pending | generating | done | failed
    metadata    JSONB,                         -- 格式相关的附加信息
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_deliverables_project_id ON deliverables(project_id);
CREATE INDEX idx_deliverables_entry ON deliverables(entry);
CREATE INDEX idx_deliverables_status ON deliverables(status);
```

**content vs storage_key 判断规则**：
- 内容 < 64KB：直接存 `content` 字段。
- 内容 >= 64KB（长文档、HTML 页面、代码文件）：存对象存储，`storage_key` 记录路径，`content` 为 NULL。

### 2.6 deliverable_versions（版本历史）

```sql
CREATE TABLE deliverable_versions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    deliverable_id UUID NOT NULL REFERENCES deliverables(id) ON DELETE CASCADE,
    version        INT NOT NULL,
    content        TEXT,
    storage_key    TEXT,
    change_reason  TEXT,                       -- user_edit | regenerate | brief_update
    created_by     UUID REFERENCES users(id),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(deliverable_id, version)
);
```

### 2.7 skill_registry（Skill 注册表）

```sql
CREATE TABLE skill_registry (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name             TEXT NOT NULL,
    type             TEXT NOT NULL UNIQUE,     -- research | strategy | ... 内置类型唯一
    description      TEXT,
    source           TEXT NOT NULL DEFAULT 'built_in',  -- built_in | custom
    input_schema     JSONB NOT NULL,           -- JSON Schema
    output_type      TEXT NOT NULL,            -- markdown | html | json | image_prompt | code
    supported_entries TEXT[] NOT NULL,
    status           TEXT NOT NULL DEFAULT 'available',
                                               -- available | beta | deprecated
    owner_id         UUID REFERENCES users(id), -- 自定义 Skill 的创建者（V1+）
    version          TEXT NOT NULL DEFAULT '1.0.0',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

## 3. 数据库迁移

使用 [goose](https://github.com/pressly/goose) 管理迁移，迁移文件按序号命名，存放在 `backend/db/migrations/`。

```
backend/db/migrations/
├── 001_init_users.sql
├── 002_init_projects.sql
├── 003_init_tasks_deliverables.sql
├── 004_init_skill_registry.sql
└── 005_seed_builtin_skills.sql
```

### 迁移文件格式

```sql
-- 001_init_users.sql
-- +goose Up
CREATE TABLE users ( ... );

-- +goose Down
DROP TABLE users;
```

### 常用命令

```bash
# 执行所有待迁移
goose -dir backend/db/migrations postgres "$DATABASE_URL" up

# 回滚最近一次迁移
goose -dir backend/db/migrations postgres "$DATABASE_URL" down

# 查看当前版本
goose -dir backend/db/migrations postgres "$DATABASE_URL" status
```

### 迁移原则

- 每个迁移文件只做一件事，粒度细。
- 生产环境迁移必须先在 staging 环境验证。
- 破坏性迁移（删列、改类型）分两步：先添加新列 + 迁移数据，再删旧列。
- 禁止在迁移文件中写业务数据（种子数据除外，用 `_seed.sql` 后缀标注）。

## 4. 对象存储

大内容（文档、HTML 页面、图片提示词集合）和用户上传的素材存放在对象存储。

### Bucket 结构

```
mosaic-{env}/
├── deliverables/
│   └── {project_id}/{deliverable_id}/v{version}.{ext}
│       例：deliverables/abc123/def456/v1.md
│           deliverables/abc123/def456/v2.html
└── uploads/
    └── {user_id}/{filename}
        例：uploads/user123/logo.png
```

### 本地开发

使用 MinIO（Docker Compose 内），与 AWS S3 API 完全兼容：

```yaml
# docker-compose.yml 片段
minio:
  image: minio/minio
  command: server /data --console-address ":9001"
  environment:
    MINIO_ROOT_USER: mosaic
    MINIO_ROOT_PASSWORD: mosaic_secret
  ports:
    - "9000:9000"   # S3 API
    - "9001:9001"   # 管理控制台
  volumes:
    - minio_data:/data
```

生产环境替换为 AWS S3、阿里云 OSS 或 Cloudflare R2（均兼容 S3 API），只需修改环境变量，代码不变。

## 5. 查询优化注意事项

- 项目详情页一次性加载 `tasks` 和 `deliverables`，避免 N+1，使用 JOIN 或批量查询。
- 交付中心按 `entry` 分组展示，`deliverables` 表已建立 `entry` 索引。
- 版本历史按需加载，默认只返回最新版本。
- `raw_json` 和 `input_snapshot` 等 JSONB 字段不参与列表查询，只在详情和调试场景使用。
