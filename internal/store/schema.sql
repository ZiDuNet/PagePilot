-- hostctl 元数据库 schema

-- 部署者 token（鉴权用）
CREATE TABLE IF NOT EXISTS tokens (
    id              TEXT PRIMARY KEY,           -- UUID
    token_hash      TEXT NOT NULL UNIQUE,       -- bcrypt(token)
    label           TEXT,                       -- 可选标签，如 "claude-code-mac"
    is_admin        BOOLEAN NOT NULL DEFAULT 0,
    is_revoked      BOOLEAN NOT NULL DEFAULT 0,
    owner_user_id   TEXT,                       -- 归属用户；为空表示兼容旧 token
    created_at      DATETIME NOT NULL,
    last_used_at    DATETIME
);

-- 一个 code 对应一个 site
CREATE TABLE IF NOT EXISTS sites (
    code                       TEXT PRIMARY KEY,
    public_id                  TEXT NOT NULL UNIQUE,             -- 对外暴露的 UUID
    owner_token_id             TEXT NOT NULL,
    current_version            INTEGER,                          -- 当前对外服务的版本号；NULL = 已下线
    primary_version_strategy   TEXT NOT NULL DEFAULT 'likes',    -- 'likes' | 'latest'
    view_count                 INTEGER NOT NULL DEFAULT 0,       -- 访问数（页面 GET）
    like_count                 INTEGER NOT NULL DEFAULT 0,       -- 点赞数
    status                     TEXT NOT NULL DEFAULT 'active',   -- 'active' | 'inactive'
    access_password_hash       TEXT NOT NULL DEFAULT '',         -- 为空表示公开访问
    expires_at                 DATETIME,                         -- 可选过期时间；NULL = 永久
    created_at                 DATETIME NOT NULL,
    updated_at                 DATETIME NOT NULL,                -- 最后修改时间
    source                     TEXT NOT NULL                     -- 'api' | 'cli' | 'mcp'
);

CREATE INDEX IF NOT EXISTS idx_sites_status ON sites(status);
CREATE INDEX IF NOT EXISTS idx_sites_public_id ON sites(public_id);

-- 一个 site 的一个版本
CREATE TABLE IF NOT EXISTS versions (
    id                TEXT PRIMARY KEY,         -- UUID
    site_code         TEXT NOT NULL REFERENCES sites(code),
    version_number    INTEGER NOT NULL,
    title             TEXT,
    description       TEXT NOT NULL,
    main_entry        TEXT NOT NULL DEFAULT 'index.html',
    total_size        INTEGER NOT NULL,
    file_count        INTEGER NOT NULL,
    content_sha256    TEXT NOT NULL,            -- 全部文件的聚合 hash（按 path 排序拼接 hash）
    is_locked         BOOLEAN NOT NULL DEFAULT 0,
    status            TEXT NOT NULL DEFAULT 'active',  -- 'active' | 'inactive'
    created_at        DATETIME NOT NULL,
    UNIQUE(site_code, version_number)
);

CREATE INDEX IF NOT EXISTS idx_versions_site ON versions(site_code, version_number);

-- 版本下的文件清单（实际文件存在磁盘上，这里只放元数据）
CREATE TABLE IF NOT EXISTS files (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    site_code         TEXT NOT NULL,
    version_number    INTEGER NOT NULL,
    file_path         TEXT NOT NULL,            -- 相对路径 e.g. "images/logo.png"
    size              INTEGER NOT NULL,
    sha256            TEXT NOT NULL,
    is_binary         BOOLEAN NOT NULL,
    UNIQUE(site_code, version_number, file_path)
);

CREATE INDEX IF NOT EXISTS idx_files_version ON files(site_code, version_number);

-- Day 7: 运行时设置（管理后台可写的 baseURL 等）
CREATE TABLE IF NOT EXISTS settings (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    updated_at  DATETIME NOT NULL
);

-- 应用商城点赞记录（防重复）
CREATE TABLE IF NOT EXISTS likes (
    site_code        TEXT NOT NULL,
    user_fingerprint TEXT NOT NULL,             -- IP 或 cookie hash，限制每用户/site 一次
    created_at       DATETIME NOT NULL,
    PRIMARY KEY (site_code, user_fingerprint)
);

CREATE INDEX IF NOT EXISTS idx_likes_site ON likes(site_code);

CREATE TABLE IF NOT EXISTS anonymous_sessions (
    id            TEXT PRIMARY KEY,
    agent_id      TEXT,
    agent_label   TEXT,
    device_ip     TEXT,
    user_agent    TEXT,
    deploy_count  INTEGER NOT NULL DEFAULT 0,
    created_at    DATETIME NOT NULL,
    last_used_at  DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_anonymous_sessions_last_used ON anonymous_sessions(last_used_at);

CREATE TABLE IF NOT EXISTS admin_users (
    id              TEXT PRIMARY KEY,
    username        TEXT NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,
    is_admin        BOOLEAN NOT NULL DEFAULT 0,
    is_active       BOOLEAN NOT NULL DEFAULT 1,
    can_like        BOOLEAN NOT NULL DEFAULT 1,
    deploy_limit    INTEGER NOT NULL DEFAULT 20,
    deploy_count    INTEGER NOT NULL DEFAULT 0,
    created_at      DATETIME NOT NULL,
    last_login_at   DATETIME
);

CREATE TABLE IF NOT EXISTS admin_sessions (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES admin_users(id),
    session_hash    TEXT NOT NULL UNIQUE,
    created_at      DATETIME NOT NULL,
    last_used_at    DATETIME NOT NULL,
    expires_at      DATETIME NOT NULL,
    revoked_at      DATETIME
);

CREATE INDEX IF NOT EXISTS idx_admin_sessions_hash ON admin_sessions(session_hash);
CREATE INDEX IF NOT EXISTS idx_admin_sessions_expires ON admin_sessions(expires_at);

CREATE TABLE IF NOT EXISTS agent_binding_codes (
    code          TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES admin_users(id),
    label         TEXT,
    created_at    DATETIME NOT NULL,
    expires_at    DATETIME NOT NULL,
    consumed_at   DATETIME,
    consumed_by   TEXT
);

CREATE INDEX IF NOT EXISTS idx_agent_binding_codes_user ON agent_binding_codes(user_id, created_at);
