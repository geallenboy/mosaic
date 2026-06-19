-- +goose Up
-- +goose StatementBegin

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE users (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email        TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role         TEXT NOT NULL DEFAULT 'user',
    tenant_id    UUID,
    avatar_url   TEXT,
    status       TEXT NOT NULL DEFAULT 'active',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_email ON users(email);

CREATE TABLE projects (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    type             TEXT NOT NULL,
    industry         TEXT,
    market           TEXT,
    target_audience  TEXT,
    goal             TEXT NOT NULL,
    budget_range     TEXT,
    style_keywords   TEXT[],
    selected_entries TEXT[] NOT NULL DEFAULT '{}',
    status           TEXT NOT NULL DEFAULT 'draft',
    error_message    TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_projects_user_id ON projects(user_id);
CREATE INDEX idx_projects_status ON projects(status);

CREATE TABLE project_briefs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id        UUID NOT NULL UNIQUE REFERENCES projects(id) ON DELETE CASCADE,
    objective         TEXT NOT NULL,
    audience          TEXT NOT NULL,
    positioning       TEXT NOT NULL DEFAULT '',
    value_proposition TEXT NOT NULL DEFAULT '',
    tone_of_voice     TEXT NOT NULL DEFAULT '',
    visual_direction  TEXT NOT NULL DEFAULT '',
    key_messages      TEXT[],
    assumptions       TEXT[],
    constraints       TEXT[],
    raw_json          JSONB,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE tasks (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id     UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    skill_type     TEXT NOT NULL,
    title          TEXT NOT NULL,
    description    TEXT,
    status         TEXT NOT NULL DEFAULT 'pending',
    input_snapshot JSONB,
    error_message  TEXT,
    retry_count    INT NOT NULL DEFAULT 0,
    started_at     TIMESTAMPTZ,
    completed_at   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tasks_project_id ON tasks(project_id);
CREATE INDEX idx_tasks_status ON tasks(status);

CREATE TABLE deliverables (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    task_id     UUID REFERENCES tasks(id) ON DELETE SET NULL,
    entry       TEXT NOT NULL,
    surface     TEXT,
    type        TEXT NOT NULL,
    title       TEXT NOT NULL,
    format      TEXT NOT NULL DEFAULT 'markdown',
    content     TEXT,
    storage_key TEXT,
    version     INT NOT NULL DEFAULT 1,
    status      TEXT NOT NULL DEFAULT 'pending',
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_deliverables_project_id ON deliverables(project_id);
CREATE INDEX idx_deliverables_entry ON deliverables(entry);
CREATE INDEX idx_deliverables_status ON deliverables(status);

CREATE TABLE skill_registry (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name             TEXT NOT NULL,
    type             TEXT NOT NULL UNIQUE,
    description      TEXT,
    source           TEXT NOT NULL DEFAULT 'built_in',
    input_schema     JSONB NOT NULL DEFAULT '{}',
    output_type      TEXT NOT NULL DEFAULT 'markdown',
    supported_entries TEXT[] NOT NULL DEFAULT '{}',
    status           TEXT NOT NULL DEFAULT 'available',
    owner_id         UUID REFERENCES users(id),
    version          TEXT NOT NULL DEFAULT '1.0.0',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS skill_registry;
DROP TABLE IF EXISTS deliverables;
DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS project_briefs;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
