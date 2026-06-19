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
