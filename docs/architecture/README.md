# Mosaic 技术架构文档

本目录是 Mosaic 万象集从 0 到 1 的完整技术架构文档套件。所有文档通过相对链接互相引用，本文件作为唯一入口。

产品需求文档见 [../product-requirements.md](../product-requirements.md)。

## 文档目录

| 文档 | 主要内容 | 主要读者 |
| --- | --- | --- |
| [00-overview.md](00-overview.md) | 总体架构、技术栈选型、Monorepo 结构、环境策略、三阶段演进路线 | 全体工程师、Tech Lead |
| [01-backend.md](01-backend.md) | Go 分层设计、项目工作流编排器、Skill 并发调度引擎、SSE、配置管理 | 后端工程师 |
| [02-clients.md](02-clients.md) | React Web、平台抽象层、Tauri 桌面壳、Flutter 预留设计 | 前端/客户端工程师 |
| [03-api-contract.md](03-api-contract.md) | OpenAPI 规范约定、多端代码生成链路、错误信封、版本与鉴权 | 全体工程师 |
| [04-data-model.md](04-data-model.md) | 核心实体 ER 图、PostgreSQL 表设计、数据库迁移、对象存储 | 后端工程师 |
| [05-ai-skill-standards.md](05-ai-skill-standards.md) | Skill 接口契约、LLM Provider 抽象、Prompt 管理、可观测性、AI 开发规范 | AI/后端工程师 |
| [06-testing.md](06-testing.md) | 测试金字塔、各端测试策略、契约测试、Skill 评测、覆盖率门禁 | 全体工程师 |
| [07-deployment-release.md](07-deployment-release.md) | 容器化、CI/CD、环境分层、云原生演进、发布流程、监控告警 | DevOps/全体工程师 |
| [08-security.md](08-security.md) | 密钥管理、认证授权、多租户 RBAC、数据保护、供应链安全 | 全体工程师、安全负责人 |
| [09-admin.md](09-admin.md) | 管理后台架构、Admin API、用户/项目/Skill/成本/内容审核/系统配置管理 | 后端工程师、Admin 前端工程师 |

## 核心设计决策速查

| 决策 | 选择 | 所在文档 |
| --- | --- | --- |
| 后端语言 | Go | [01-backend.md](01-backend.md) |
| Web 前端 | React + Vite | [02-clients.md](02-clients.md) |
| 桌面客户端 | Tauri（复用 React Web） | [02-clients.md](02-clients.md) |
| 移动端 | Flutter（V1+ 阶段） | [02-clients.md](02-clients.md) |
| API 契约 | OpenAPI 3.1 单一事实源 | [03-api-contract.md](03-api-contract.md) |
| 主数据库 | PostgreSQL | [04-data-model.md](04-data-model.md) |
| 对象存储 | S3 兼容（本地 MinIO / 云端） | [04-data-model.md](04-data-model.md) |
| Skill 调度（V0.1） | in-process goroutine + errgroup | [01-backend.md](01-backend.md) |
| Skill 调度（中期） | Asynq（Redis-backed 任务队列） | [01-backend.md](01-backend.md) |
| LLM 接入 | Provider 抽象层，支持多模型路由 | [05-ai-skill-standards.md](05-ai-skill-standards.md) |
| 实时进度推送 | SSE（Server-Sent Events） | [01-backend.md](01-backend.md) |
| CI/CD | GitHub Actions | [07-deployment-release.md](07-deployment-release.md) |
| 认证 | JWT + RBAC | [08-security.md](08-security.md) |
| V0.1 部署 | Docker Compose | [07-deployment-release.md](07-deployment-release.md) |
| Admin 管理后台 | React（apps/admin）+ Go /admin/api/v1/ | [09-admin.md](09-admin.md) |

## 阅读建议

- **新成员入门**：先读 00-overview → 03-api-contract → 对应端的文档。
- **后端开发**：00-overview → 01-backend → 04-data-model → 05-ai-skill-standards。
- **前端/客户端开发**：00-overview → 02-clients → 03-api-contract。
- **部署上线**：07-deployment-release → 08-security。
- **编写新 Skill**：05-ai-skill-standards → 01-backend（Skill Runtime 章节）。
- **Admin 后台开发**：09-admin → 03-api-contract（Admin 路由约定）→ 04-data-model（新增表）。
