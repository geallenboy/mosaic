# 08 安全设计

## 1. 核心安全原则

1. **前端零密钥**：任何 API Key、LLM 密钥、数据库连接串禁止出现在前端代码或浏览器可访问的位置。
2. **最小权限**：每个服务、每个角色只拥有完成自身职责所需的最小权限。
3. **深度防御**：不依赖单一安全机制，多层防护。
4. **默认安全**：新功能默认需要认证，需要主动声明"公开"。
5. **安全左移**：安全检查在 CI 中执行，不等到上线后才发现。

## 2. 密钥管理

### 2.1 密钥分类与存储规则

| 密钥类型 | 存储位置 | 禁止位置 |
| --- | --- | --- |
| LLM API Key（OpenAI 等） | 服务端环境变量 / 密钥管理服务 | 前端代码、Git 仓库、日志 |
| 数据库连接串 | 服务端环境变量 / 密钥管理服务 | 前端代码、Git 仓库、日志 |
| JWT 签名密钥 | 服务端环境变量 / 密钥管理服务 | 前端代码、Git 仓库 |
| 对象存储 Access Key | 服务端环境变量 / 密钥管理服务 | 前端代码、Git 仓库 |
| Tauri 桌面端用户凭证 | 系统密钥链（Keychain / Windows Credential Manager） | 明文文件、localStorage |

### 2.2 本地开发

使用 `.env` 文件注入，`.gitignore` 中必须包含：

```gitignore
# 安全相关
.env
.env.local
.env.*.local
*.pem
*.key
```

仓库中提供 `.env.example` 作为模板，不包含任何真实值：

```bash
# .env.example
DATABASE_URL=postgres://user:password@localhost:5432/mosaic
OPENAI_API_KEY=sk-your-api-key-here
JWT_SECRET=your-jwt-secret-here-min-32-chars
STORAGE_ACCESS_KEY=your-minio-access-key
STORAGE_SECRET_KEY=your-minio-secret-key
```

### 2.3 生产环境

生产环境密钥通过以下方式之一注入（不硬编码在 Compose 或代码中）：

- **云平台 Secrets**：Fly.io `fly secrets set`、Railway Environment Variables（加密存储）
- **GitHub Actions Secrets**：CI/CD 流水线中使用 `${{ secrets.XXX }}`
- **密钥管理服务（中期）**：HashiCorp Vault 或云厂商 KMS（AWS Secrets Manager、阿里云 KMS）

### 2.4 密钥泄露响应流程

1. 立即在 Provider 端吊销泄露的密钥
2. 生成新密钥并更新所有环境
3. 审查 Git 历史，若密钥已提交，使用 `git filter-repo` 清理历史
4. 评估泄露期间的 API 用量，判断是否有异常调用

## 3. 认证与授权

### 3.1 认证流程（JWT）

```
用户登录
    │
    ▼
POST /api/v1/auth/login
    │  验证 email + password（bcrypt hash）
    ▼
返回 Access Token（15 分钟）+ Refresh Token（7 天）
    │
    ▼
客户端存储：
    Web：Access Token 存 memory，Refresh Token 存 httpOnly Cookie
    Tauri：Access Token 存 memory，Refresh Token 存系统密钥链
    │
    ▼
请求携带：Authorization: Bearer <access_token>
    │
    ▼
Access Token 过期 → 自动用 Refresh Token 换新的 Access Token
```

**为什么 Access Token 存 memory 而不是 localStorage**：防止 XSS 攻击读取 Token。页面刷新后通过 Refresh Token 自动续期。

### 3.2 JWT Payload 设计

```json
{
  "sub": "user_uuid",
  "email": "user@example.com",
  "role": "user",
  "tenant_id": null,
  "iat": 1704067200,
  "exp": 1704068100
}
```

JWT 只存放不敏感的身份标识，不存放用户详细信息（避免 Token 过大和信息泄露）。

### 3.3 RBAC 角色权限矩阵

| 资源 / 操作 | 普通用户 | 服务商 | 企业管理员 | 系统管理员 |
| --- | --- | --- | --- | --- |
| 查看自己的项目 | ✓ | ✓ | ✓ | ✓ |
| 创建项目 | ✓ | ✓ | ✓ | ✓ |
| 查看客户项目 | ✗ | ✓（自己的客户） | ✓（本租户） | ✓ |
| 管理自定义 Skill | ✗ | ✓ | ✓ | ✓ |
| 查看 Skill Hub 全部 Skill | ✗ | ✓ | ✓ | ✓ |
| 用户管理 | ✗ | ✗ | ✓（本租户） | ✓ |

### 3.4 Go 中间件实现

```go
// internal/handler/middleware/auth.go

func RequireAuth(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := extractBearerToken(r)
        if token == "" {
            writeUnauthorized(w, "missing token")
            return
        }

        claims, err := jwt.Parse(token, signingKey)
        if err != nil {
            writeUnauthorized(w, "invalid token")
            return
        }

        // 将用户信息注入 context
        ctx := context.WithValue(r.Context(), ctxKeyUser, claims)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

func RequireRole(roles ...string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            claims := r.Context().Value(ctxKeyUser).(*Claims)
            for _, role := range roles {
                if claims.Role == role {
                    next.ServeHTTP(w, r)
                    return
                }
            }
            writeForbidden(w)
        })
    }
}
```

## 4. 数据保护

### 4.1 传输加密

- 所有生产环境接口强制 HTTPS（TLS 1.2+），HTTP 自动重定向到 HTTPS。
- 本地开发允许 HTTP（localhost 不强制）。
- SSE 连接同样走 HTTPS，不允许降级。

### 4.2 数据库加密

- PostgreSQL 连接强制 `sslmode=require`（生产环境）。
- 数据库密码使用随机生成的强密码（> 32 位），不使用默认密码。
- 备份文件加密存储。

### 4.3 敏感信息脱敏

以下内容禁止出现在日志、错误信息、API 响应和导出文件中：

- LLM API Key / Token
- 数据库连接串
- 用户密码（任何形式）
- JWT 签名密钥
- 用户私有内容（非本人请求不可见）

Go 结构体中的敏感字段使用 `json:"-"` 禁止序列化：

```go
type Config struct {
    DatabaseURL string `json:"-"`  // 禁止在任何 JSON 响应中出现
    LLMAPIKey   string `json:"-"`
    JWTSecret   string `json:"-"`
}
```

### 4.4 用户数据隔离

数据库查询必须携带 `user_id`（或 `tenant_id`）过滤，禁止全表查询用户数据：

```go
// 正确：始终过滤 user_id
func (r *ProjectRepo) GetByID(ctx context.Context, projectID, userID string) (*Project, error) {
    return r.db.QueryRow(ctx,
        "SELECT * FROM projects WHERE id = $1 AND user_id = $2",
        projectID, userID,
    )
}

// 错误：缺少 user_id 过滤（IDOR 漏洞）
// SELECT * FROM projects WHERE id = $1
```

## 5. 输入校验与注入防护

### 5.1 SQL 注入防护

所有数据库查询使用参数化查询（sqlc 生成代码默认遵守此规则）：

```go
// 正确（参数化查询）
db.QueryRow(ctx, "SELECT * FROM projects WHERE id = $1", projectID)

// 错误（字符串拼接，SQL 注入风险）
// db.QueryRow(ctx, "SELECT * FROM projects WHERE id = '" + projectID + "'")
```

### 5.2 XSS 防护

- React 默认转义 JSX 中的输出，不使用 `dangerouslySetInnerHTML`（Skypage 入口的 HTML 预览除外，使用 iframe sandbox 隔离）。
- 用户生成内容在存储前经过 HTML sanitizer 处理。
- Content Security Policy（CSP）Header 限制脚本来源。

### 5.3 请求校验

所有 API 入参在 handler 层使用 ogen 生成的校验逻辑（基于 OpenAPI Spec 中的 required/format/enum 约束），校验失败返回 422。

## 6. 依赖与供应链安全

### 6.1 Go 依赖

```yaml
# .github/workflows/security.yml 片段
- name: Go vulnerability check
  run: |
    go install golang.org/x/vuln/cmd/govulncheck@latest
    govulncheck ./...
```

### 6.2 Node.js 依赖

```yaml
- name: npm audit
  run: pnpm audit --audit-level=high
```

### 6.3 Docker 镜像扫描

```yaml
- name: Scan Docker image
  uses: aquasecurity/trivy-action@master
  with:
    image-ref: ghcr.io/mosaic-app/backend:${{ github.sha }}
    severity: HIGH,CRITICAL
    exit-code: '1'
```

以上安全扫描在 CI 中自动运行，发现高危漏洞则阻断合并。

## 7. 速率限制

防止 LLM API 被滥用和暴力攻击。

```go
// internal/handler/middleware/rate_limit.go

// 每用户每分钟最多创建 3 个项目（防止 LLM 费用被刷）
var projectCreationLimiter = rate.NewLimiter(rate.Every(time.Minute/3), 3)

// 登录接口每 IP 每分钟最多 10 次（防暴力破解）
var loginLimiter = rate.NewLimiter(rate.Every(time.Minute/10), 10)
```

中期引入 Redis-backed 分布式限流，支持多实例部署场景。

## 8. 安全 Checklist（发布前必查）

- [ ] 所有生产密钥通过环境变量或密钥管理服务注入，未硬编码
- [ ] `.env` 和 `*.key` 在 `.gitignore` 中
- [ ] 生产 HTTPS 证书有效期 > 30 天
- [ ] 数据库连接使用 `sslmode=require`
- [ ] 所有 API 接口有 RequireAuth 中间件（公开接口除外）
- [ ] 用户数据查询携带 user_id 过滤
- [ ] CI 安全扫描（govulncheck、npm audit、trivy）通过
- [ ] 速率限制已启用
- [ ] JWT 密钥长度 > 32 字符，随机生成
