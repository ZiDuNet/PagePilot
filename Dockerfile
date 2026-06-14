# hostctl 多阶段 Dockerfile
# 阶段 1：构建 Go 二进制
# 阶段 2：运行时（极简镜像，仅含二进制 + 必要 CA 证书）

# ===== Build =====
FROM golang:1.22-alpine AS builder

# 安装 git（go mod 需要）+ ca-certificates
RUN apk add --no-cache git ca-certificates

WORKDIR /src

# 先单独复制 mod 文件，利用层缓存
COPY go.mod go.sum ./
RUN go mod download

# 复制源码
COPY . .

# 编译 server 和 CLI（关闭 CGO，因为 modernc.org/sqlite 是纯 Go）
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/hostctl-server ./cmd/hostctl-server && \
    go build -trimpath -ldflags="-s -w" -o /out/hostctl        ./cmd/hostctl

# ===== Runtime =====
FROM alpine:3.20

# 必要的运行时依赖：ca-certificates（HTTPS 抓取）、tzdata（时区显示）、curl（健康检查）
RUN apk add --no-cache ca-certificates tzdata curl

# 创建非 root 用户
RUN addgroup -S hostctl && adduser -S -G hostctl -h /var/lib/hostctl hostctl

# 创建数据目录
RUN mkdir -p /var/lib/hostctl /var/www/hosted /var/log/hostctl && \
    chown -R hostctl:hostctl /var/lib/hostctl /var/www/hosted /var/log/hostctl

# 拷贝二进制
COPY --from=builder /out/hostctl-server /usr/local/bin/hostctl-server
COPY --from=builder /out/hostctl        /usr/local/bin/hostctl

# 拷贝部署模板（Caddyfile / systemd unit）方便宿主使用
COPY deploy/Caddyfile              /etc/hostctl/Caddyfile.example
COPY deploy/hostctl-server.service /etc/hostctl/hostctl-server.service.example

USER hostctl
WORKDIR /var/lib/hostctl

# 默认配置：监听 0.0.0.0:8787，数据卷落到 /var/lib/hostctl 与 /var/www/hosted
# 运行时请通过环境变量或参数覆盖 PublicBaseURL。
ENV HOSTCTL_HTTP_ADDR=0.0.0.0:8787 \
    HOSTCTL_HOSTED_DIR=/var/www/hosted \
    HOSTCTL_DB_PATH=/var/lib/hostctl/hostctl.db \
    HOSTCTL_PUBLIC_BASE_URL=http://localhost:8787 \
    HOSTCTL_COOLDOWN_SECONDS=10

VOLUME ["/var/lib/hostctl", "/var/www/hosted"]

EXPOSE 8787

# 健康检查：dev 模式 /api/health 是公开的；prod 模式也允许这个端点（只读）
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -fsS http://127.0.0.1:8787/api/health || exit 1

# 默认 dev 模式启动（无需 token 即可访问 /admin）。
# 生产部署请加 --require-auth，并在 admin UI 中创建 admin token。
ENTRYPOINT ["hostctl-server"]
CMD ["--addr", "0.0.0.0:8787", \
     "--hosted-dir", "/var/www/hosted", \
     "--db", "/var/lib/hostctl/hostctl.db", \
     "--public-url", "http://localhost:8787"]
