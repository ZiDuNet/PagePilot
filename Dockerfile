# PagePilot / hostctl multi-stage Dockerfile.
# Frontend stages build React assets, Go stage embeds them into static binaries.

ARG NODE_IMAGE=node:22-alpine
ARG GO_IMAGE=golang:1.25-alpine
ARG ALPINE_IMAGE=alpine:3.20

# ===== Frontend =====
FROM ${NODE_IMAGE} AS frontend-builder

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories

WORKDIR /src

COPY frontend/admin/package*.json ./frontend/admin/
RUN cd frontend/admin && npm ci

COPY frontend/user/package*.json ./frontend/user/
RUN cd frontend/user && npm ci

COPY frontend/admin ./frontend/admin
COPY frontend/user ./frontend/user
COPY internal/web ./internal/web

RUN cd frontend/admin && npm run build && \
    cd ../user && npm run build

# ===== Go Build =====
FROM ${GO_IMAGE} AS builder

# git is needed by go mod for VCS-backed modules; python3 rebuilds the embedded Skill ZIP.
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk add --no-cache git ca-certificates python3

WORKDIR /src

ARG GOPROXY=https://mirrors.aliyun.com/goproxy/,direct
ARG GOSUMDB=sum.golang.google.cn
ENV GOPROXY=${GOPROXY} \
    GOSUMDB=${GOSUMDB}

# Copy module files first to maximize layer cache reuse.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend-builder /src/internal/web/admin/app ./internal/web/admin/app
COPY --from=frontend-builder /src/internal/web/user/app ./internal/web/user/app

# modernc.org/sqlite is pure Go, so CGO can stay disabled.
ENV CGO_ENABLED=0 GOOS=linux
RUN python3 scripts/build_skill_zip.py && \
    go build -trimpath -ldflags="-s -w" -o /out/hostctl-server ./cmd/hostctl-server && \
    go build -trimpath -ldflags="-s -w" -o /out/pagep          ./cmd/hostctl && \
    go build -trimpath -ldflags="-s -w" -o /out/pagep-mcp      ./cmd/hostctl-mcp

# ===== Runtime =====
FROM ${ALPINE_IMAGE}

# Runtime deps: CA certs for HTTPS, tzdata for local time display, curl for healthcheck.
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk add --no-cache ca-certificates tzdata curl

RUN addgroup -S hostctl && adduser -S -G hostctl -h /var/lib/hostctl hostctl

RUN mkdir -p /var/lib/hostctl/sql /var/www/hosted /var/log/hostctl /opt/pagepilot/skill && \
    chown -R hostctl:hostctl /var/lib/hostctl /var/www/hosted /var/log/hostctl /opt/pagepilot

COPY --from=builder /out/hostctl-server /usr/local/bin/hostctl-server
COPY --from=builder /out/pagep          /usr/local/bin/pagep
COPY --from=builder /out/pagep-mcp      /usr/local/bin/pagep-mcp
RUN ln -s /usr/local/bin/pagep /usr/local/bin/hostctl && \
    ln -s /usr/local/bin/pagep-mcp /usr/local/bin/hostctl-mcp

COPY deploy/Caddyfile              /etc/hostctl/Caddyfile.example
COPY deploy/hostctl-server.service /etc/hostctl/hostctl-server.service.example
COPY --from=builder --chown=hostctl:hostctl /src/skill/hostctl-deploy /opt/pagepilot/skill/hostctl-deploy

USER hostctl
WORKDIR /var/lib/hostctl

# Defaults are intentionally environment-driven so compose/k8s can override them.
ENV HOSTCTL_HTTP_ADDR=0.0.0.0:8787 \
    HOSTCTL_HOSTED_DIR=/var/www/hosted \
    HOSTCTL_DB_PATH=/var/lib/hostctl/hostctl.db \
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
