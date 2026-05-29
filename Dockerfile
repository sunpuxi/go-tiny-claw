# ============================================================
# Stage 1: 编译 Go 二进制
# ============================================================
FROM golang:1.25-alpine AS builder

WORKDIR /build

# 先拷贝依赖文件，利用 Docker 层缓存加速
COPY go.mod go.sum ./
RUN go mod download

# 拷贝源码并编译
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o claw cmd/claw/main.go

# ============================================================
# Stage 2: 最小运行镜像
# ============================================================
FROM alpine:3.20

# ca-certificates: HTTPS 请求所必需
# tzdata: 时区支持
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# 复制编译好的二进制
COPY --from=builder /build/claw .

# 复制默认配置文件（运行时可通过 volume 覆盖以实现热加载）
COPY config/config.yaml ./config/config.yaml

# 复制 workspace 目录
COPY workspace ./workspace

EXPOSE 48080

CMD ["./claw"]
