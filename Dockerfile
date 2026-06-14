# PagePilot / hostctl multi-stage Dockerfile.
# Build stage compiles static Go binaries; runtime stage keeps only binaries and small OS deps.

# ===== Build =====
FROM golang:1.22-alpine AS builder

# git is needed by go mod for VCS-backed modules.
RUN apk add --no-cache git ca-certificates

WORKDIR /src

ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn
ENV GOPROXY=${GOPROXY} \
    GOSUMDB=${GOSUMDB}

# Copy module files first to maximize layer cache reuse.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# modernc.org/sqlite is pure Go, so CGO can stay disabled.
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/hostctl-server ./cmd/hostctl-server && \
    go build -trimpath -ldflags="-s -w" -o /out/hostctl        ./cmd/hostctl

# ===== Runtime =====
FROM alpine:3.20

# Runtime deps: CA certs for HTTPS, tzdata for local time display, curl for healthcheck.
RUN apk add --no-cache ca-certificates tzdata curl

RUN addgroup -S hostctl && adduser -S -G hostctl -h /var/lib/hostctl hostctl

RUN mkdir -p /var/lib/hostctl/sql /var/www/hosted /var/log/hostctl /opt/pagepilot/skill && \
    chown -R hostctl:hostctl /var/lib/hostctl /var/www/hosted /var/log/hostctl /opt/pagepilot

COPY --from=builder /out/hostctl-server /usr/local/bin/hostctl-server
COPY --from=builder /out/hostctl        /usr/local/bin/hostctl

COPY deploy/Caddyfile              /etc/hostctl/Caddyfile.example
COPY deploy/hostctl-server.service /etc/hostctl/hostctl-server.service.example
COPY --from=builder --chown=hostctl:hostctl /src/skill/hostctl-deploy /opt/pagepilot/skill/hostctl-deploy

USER hostctl
WORKDIR /var/lib/hostctl

# Defaults are intentionally environment-driven so compose/k8s can override them.
ENV HOSTCTL_HTTP_ADDR=0.0.0.0:8787 \
    HOSTCTL_HOSTED_DIR=/var/www/hosted \
    HOSTCTL_DB_PATH=/var/lib/hostctl/hostctl.db \
    HOSTCTL_PUBLIC_BASE_URL=http://localhost:8787 \
    HOSTCTL_SKILL_DIR=/opt/pagepilot/skill/hostctl-deploy \
    HOSTCTL_COOLDOWN_SECONDS=10 \
    REQUIRE_AUTH=true \
    HOSTCTL_ADMIN_USERNAME=admin \
    HOSTCTL_ADMIN_PASSWORD=123456

VOLUME ["/var/lib/hostctl", "/var/lib/hostctl/sql", "/var/www/hosted", "/var/log/hostctl"]

EXPOSE 8787

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -fsS http://127.0.0.1:8787/api/health || exit 1

# Docker starts with admin / 123456 only when the database has no users.
# Change the password immediately after first login.
ENTRYPOINT ["hostctl-server"]
CMD ["--addr", "0.0.0.0:8787", \
     "--hosted-dir", "/var/www/hosted", \
     "--db", "/var/lib/hostctl/hostctl.db"]
