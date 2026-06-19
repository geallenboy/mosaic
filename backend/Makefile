.PHONY: build run test lint migrate gen-api

# 构建
build:
	go build -o bin/server ./cmd/server

# 本地运行（需要先 cp .env.example .env 并填写值）
run:
	go run ./cmd/server

# 测试
test:
	go test ./... -count=1 -race

# 集成测试
test-integration:
	go test ./... -tags integration -count=1

# 覆盖率
coverage:
	go test ./internal/... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

# Lint
lint:
	golangci-lint run ./...

# 数据库迁移（需要设置 DATABASE_URL）
migrate-up:
	goose -dir db/migrations postgres "$(DATABASE_URL)" up

migrate-down:
	goose -dir db/migrations postgres "$(DATABASE_URL)" down

migrate-status:
	goose -dir db/migrations postgres "$(DATABASE_URL)" status

# 生成 API 客户端代码
gen-api:
	@echo "Generating Go server from openapi.yaml..."
	ogen --target internal/handler/oas_gen --package oas_gen api/openapi.yaml || true
	@echo "Generating TypeScript client from openapi.yaml..."
	cd .. && npx openapi-typescript backend/api/openapi.yaml --output packages/api-client/src/schema.ts || true
	@echo "Done."
