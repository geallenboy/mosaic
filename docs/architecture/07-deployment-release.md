# 07 部署与发布

## 1. 容器化

### 1.1 Go 后端 Dockerfile

采用多阶段构建，最终镜像只包含二进制文件和必要的 CA 证书。

```dockerfile
# backend/Dockerfile

# 构建阶段
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /mosaic-server ./cmd/server

# 运行阶段（极小镜像）
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /mosaic-server /mosaic-server

EXPOSE 8080
ENTRYPOINT ["/mosaic-server"]
```

### 1.2 React Web Dockerfile

```dockerfile
# apps/web/Dockerfile

FROM node:20-alpine AS builder

WORKDIR /app
COPY package.json pnpm-lock.yaml ./
RUN npm install -g pnpm && pnpm install --frozen-lockfile

COPY . .
RUN pnpm build

FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
```

```nginx
# apps/web/nginx.conf
server {
    listen 80;
    root /usr/share/nginx/html;
    index index.html;

    # React Router 支持
    location / {
        try_files $uri $uri/ /index.html;
    }

    # API 反向代理（Nginx 转发到后端）
    location /api/ {
        proxy_pass http://backend:8080;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

## 2. V0.1：Docker Compose 部署

V0.1 阶段单机部署，所有服务通过 Docker Compose 编排。

```yaml
# docker-compose.yml（本地开发）
version: '3.9'

services:
  backend:
    build: ./backend
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=postgres://mosaic:mosaic@db:5432/mosaic?sslmode=disable
      - STORAGE_ENDPOINT=http://minio:9000
      - STORAGE_ACCESS_KEY=mosaic
      - STORAGE_SECRET_KEY=mosaic_secret
      - STORAGE_BUCKET=mosaic-dev
      - LLM_PROVIDER=openai
      - LLM_API_KEY=${OPENAI_API_KEY}   # 从本地 .env 注入，禁止硬编码
      - LLM_MODEL=gpt-4o
      - AUTH_JWT_SECRET=${JWT_SECRET}
      - ENV=development
    depends_on:
      db:
        condition: service_healthy
      minio:
        condition: service_started
    restart: unless-stopped

  web:
    build: ./apps/web
    ports:
      - "3000:80"
    depends_on:
      - backend

  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: mosaic
      POSTGRES_USER: mosaic
      POSTGRES_PASSWORD: mosaic
    volumes:
      - postgres_data:/var/lib/postgresql/data
    ports:
      - "${POSTGRES_HOST_PORT:-15432}:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U mosaic"]
      interval: 5s
      timeout: 5s
      retries: 5

  minio:
    image: minio/minio
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: mosaic
      MINIO_ROOT_PASSWORD: mosaic_secret
    volumes:
      - minio_data:/data
    ports:
      - "9000:9000"
      - "9001:9001"

volumes:
  postgres_data:
  minio_data:
```

```yaml
# docker-compose.prod.yml（生产环境覆盖）
version: '3.9'

services:
  backend:
    image: ghcr.io/mosaic-app/backend:${VERSION}
    environment:
      - ENV=production
      - DATABASE_URL=${DATABASE_URL}           # 从密钥管理服务注入
      - LLM_API_KEY=${LLM_API_KEY}
    restart: always
    deploy:
      replicas: 2
      update_config:
        parallelism: 1
        delay: 10s

  web:
    image: ghcr.io/mosaic-app/web:${VERSION}
    restart: always
```

## 3. CI/CD Pipeline（GitHub Actions）

### 3.1 PR 检查流水线

每个 PR 触发，必须全部通过才能合并。

```yaml
# .github/workflows/ci.yml

name: CI

on:
  pull_request:
    branches: [main, develop]

jobs:
  backend-lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }
      - name: golangci-lint
        uses: golangci/golangci-lint-action@v4

  backend-test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16
        env: { POSTGRES_DB: mosaic_test, POSTGRES_USER: test, POSTGRES_PASSWORD: test }
        options: --health-cmd pg_isready
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - name: Run unit tests
        run: go test ./internal/... -count=1 -race -coverprofile=coverage.out
      - name: Run integration tests
        run: go test ./internal/... -tags integration -count=1
        env: { DATABASE_URL: postgres://test:test@localhost/mosaic_test }
      - name: Check coverage
        run: |
          COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | tr -d '%')
          echo "Coverage: $COVERAGE%"
          awk "BEGIN {if ($COVERAGE < 70) {exit 1}}"

  frontend-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with: { node-version: '20' }
      - run: npm install -g pnpm && pnpm install --frozen-lockfile
      - run: pnpm --filter @mosaic/web test --run
      - run: pnpm --filter @mosaic/ui test --run

  api-gen-check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Regenerate API clients
        run: make gen-api
      - name: Check no diff
        run: |
          git diff --exit-code packages/api-client/src/schema.ts || \
          (echo "API client is out of sync with openapi.yaml. Run 'make gen-api' and commit." && exit 1)
```

### 3.2 发布流水线

合并到 `main` 分支后自动构建镜像并部署到 staging，需要手动批准才能部署到生产。

```yaml
# .github/workflows/release.yml

name: Release

on:
  push:
    branches: [main]

jobs:
  build-and-push:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Login to GitHub Container Registry
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Build and push backend image
        uses: docker/build-push-action@v5
        with:
          context: ./backend
          push: true
          tags: |
            ghcr.io/mosaic-app/backend:${{ github.sha }}
            ghcr.io/mosaic-app/backend:latest

      - name: Build and push web image
        uses: docker/build-push-action@v5
        with:
          context: ./apps/web
          push: true
          tags: |
            ghcr.io/mosaic-app/web:${{ github.sha }}
            ghcr.io/mosaic-app/web:latest

  deploy-staging:
    needs: build-and-push
    runs-on: ubuntu-latest
    environment: staging
    steps:
      - name: Deploy to staging
        run: |
          VERSION=${{ github.sha }} docker-compose -f docker-compose.prod.yml pull
          VERSION=${{ github.sha }} docker-compose -f docker-compose.prod.yml up -d
          # 等待健康检查通过
          sleep 15
          curl -f https://staging.mosaic.app/health || exit 1

  deploy-production:
    needs: deploy-staging
    runs-on: ubuntu-latest
    environment:
      name: production        # GitHub Environments 配置手动审批
      url: https://mosaic.app
    steps:
      - name: Deploy to production
        run: |
          VERSION=${{ github.sha }} docker-compose -f docker-compose.prod.yml pull
          VERSION=${{ github.sha }} docker-compose -f docker-compose.prod.yml up -d --no-downtime
```

## 4. 环境分层

| 环境 | 触发方式 | 数据库 | LLM | 部署目标 |
| --- | --- | --- | --- | --- |
| local | 手动 `docker-compose up` | 本地 Docker | OpenAI（开发 Key） | 开发者本机 |
| dev | PR 创建/更新 | 独立实例，每次迁移重置 | Mock LLM | CI 临时环境 |
| staging | 合并到 main 后自动 | 独立实例，定期脱敏快照 | OpenAI（测试 Key） | 托管云平台 |
| prod | 手动审批后发布 | 生产数据库 + 备份 | OpenAI（生产 Key） | 托管云平台 |

## 5. 云原生演进路径

### 前期（V0.1）：Docker Compose 单机

- 单台云服务器（4 核 8GB 起步）
- Docker Compose 编排所有服务
- PostgreSQL 和 MinIO 也运行在同一台机器
- 适合早期验证，成本低，维护简单

### 中期（V0.2-V0.3）：托管云平台

迁移到 [Fly.io](https://fly.io/) 或 [Railway](https://railway.app/)：

- 后端：Fly.io Machines（按需自动扩缩，支持 Go 二进制直接部署）
- 数据库：Supabase PostgreSQL（托管，自动备份，连接池内置）
- 对象存储：Cloudflare R2（S3 兼容，免费出流量）
- Redis（Asynq 队列）：Upstash Redis（Serverless Redis，按用量计费）
- CDN：Cloudflare（静态资源 + Web）

### 后期（V1.0+）：Kubernetes

大规模用户量后迁移到 K8s（自建或托管 EKS/GKE）：

- HPA（Horizontal Pod Autoscaler）根据 CPU/请求量自动扩缩后端
- 独立的 Worker 部署（处理 Asynq 任务）
- PostgreSQL 读写分离
- 多区域部署（中国用户 + 海外用户）

## 6. 桌面应用发布

Tauri 桌面应用通过 GitHub Actions 构建多平台安装包：

```yaml
# .github/workflows/desktop-release.yml

jobs:
  build-desktop:
    strategy:
      matrix:
        platform: [macos-latest, windows-latest, ubuntu-22.04]
    runs-on: ${{ matrix.platform }}
    steps:
      - uses: actions/checkout@v4
      - uses: tauri-apps/tauri-action@v0
        with:
          tagName: desktop-v${{ github.ref_name }}
          releaseName: 'Mosaic Desktop v${{ github.ref_name }}'
          releaseBody: |
            See the assets below to download this release.
```

发布产物上传到 GitHub Releases，同时触发 Tauri 内置更新服务，已安装用户自动收到更新通知。

## 7. 数据库迁移发布流程

生产环境数据库迁移需要特别谨慎：

1. 迁移文件合并到 main 后，先在 staging 执行并观察 15 分钟。
2. 生产发布前，先执行 `goose status` 确认迁移文件未被跳过。
3. 破坏性迁移（删除列/表）必须在下线旧代码 72 小时后才能执行。
4. 所有迁移必须有 `-- +goose Down` 回滚语句，且在 staging 验证过回滚可用。

## 8. 监控与告警

### V0.1（结构化日志）

所有日志使用 `log/slog`（Go 标准库）输出 JSON 格式，部署到有日志聚合能力的平台（Fly.io 内置日志、Sentry）。

关键告警：
- 后端 5xx 错误率 > 1%：立即告警
- Skill 失败率 > 20%：立即告警
- 健康检查端点 `/health` 不可达：立即告警

### 中期（Prometheus + Grafana）

在后端暴露 `/metrics` 端点，Prometheus 抓取，Grafana 展示核心指标面板：

- 请求量与延迟分布
- Skill 执行耗时与成功率
- LLM Token 用量与成本趋势
- 活跃项目数与每日交付物生成量

### 健康检查端点

```go
// internal/handler/health.go

func (h *HealthHandler) Check(w http.ResponseWriter, r *http.Request) {
    checks := map[string]string{
        "database": h.checkDB(),
        "storage":  h.checkStorage(),
        "llm":      "ok",  // LLM 不做实时检查，避免产生费用
    }

    allOK := true
    for _, status := range checks {
        if status != "ok" {
            allOK = false
        }
    }

    code := http.StatusOK
    if !allOK {
        code = http.StatusServiceUnavailable
    }

    w.WriteHeader(code)
    json.NewEncoder(w).Encode(checks)
}
```
