-- +goose Up
-- +goose StatementBegin

CREATE TABLE admin_audit_logs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_user_id UUID NOT NULL REFERENCES users(id),
    action        TEXT NOT NULL,
    resource      TEXT NOT NULL,
    resource_id   TEXT,
    detail        JSONB,
    ip            TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_admin_audit_logs_admin_user ON admin_audit_logs(admin_user_id);
CREATE INDEX idx_admin_audit_logs_created_at ON admin_audit_logs(created_at);

CREATE TABLE system_configs (
    key         TEXT PRIMARY KEY,
    value       JSONB NOT NULL,
    description TEXT,
    updated_by  UUID REFERENCES users(id),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO system_configs (key, value, description) VALUES
('llm.routing', '{"research":"gpt-4o","strategy":"gpt-4o","copy":"gpt-4o-mini","deck":"gpt-4o-mini","visual":"gpt-4o","video":"gpt-4o-mini","web":"gpt-4o","ops":"gpt-4o-mini","dev":"gpt-4o"}', 'Skill -> LLM 模型路由规则'),
('rate_limit.project_creation', '{"per_user_per_minute": 3}', '项目创建速率限制'),
('llm.cost_alert_threshold_usd', '"100"', '日费用告警阈值（USD）');

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM system_configs;
DROP TABLE IF EXISTS system_configs;
DROP TABLE IF EXISTS admin_audit_logs;
-- +goose StatementEnd
