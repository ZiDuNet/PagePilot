package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config is the hostctl-server runtime configuration.
type Config struct {
	HTTPAddr          string
	HostedDir         string
	DBPath            string
	AppURLMode        string
	AppDomainSuffix   string
	AppURLScheme      string
	AppURLPort        string
	CORSAllowOrigins  string
	EmbedPolicy       string
	EmbedAllowOrigins string

	MaxSingleFileBytes int64
	MaxSiteTotalBytes  int64
	MaxFilesPerSite    int

	CooldownSeconds      int
	AnonymousDeployLimit int

	BootstrapAdminUsername string
	BootstrapAdminPassword string

	EmailVerificationEnabled bool
	SMTPHost                 string
	SMTPPort                 string
	SMTPUsername             string
	SMTPPassword             string
	SMTPFrom                 string
	SMTPSecure               string

	StorageBackend     string
	OSSProvider        string
	OSSEndpoint        string
	OSSBucket          string
	OSSAccessKeyID     string
	OSSAccessKeySecret string
	OSSPrefix          string
	OSSPublicBaseURL   string
}

// Default returns runtime defaults, overridable by environment variables.
func Default() Config {
	c := Config{
		HTTPAddr:             "127.0.0.1:8787",
		HostedDir:            "/var/www/hosted",
		DBPath:               "/var/lib/hostctl/hostctl.db",
		AppURLMode:           "path",
		AppURLScheme:         "https",
		CORSAllowOrigins:     "",
		EmbedPolicy:          "any",
		MaxSingleFileBytes:   1 << 20,
		MaxSiteTotalBytes:    10 << 20,
		MaxFilesPerSite:      100,
		CooldownSeconds:      10,
		AnonymousDeployLimit: 5,
		StorageBackend:       "local",
		OSSProvider:          "aliyun",
		SMTPSecure:           "starttls",
	}

	if v := os.Getenv("HOSTCTL_HTTP_ADDR"); v != "" {
		c.HTTPAddr = v
	}
	if v := os.Getenv("HOSTCTL_HOSTED_DIR"); v != "" {
		c.HostedDir = v
	}
	if v := os.Getenv("HOSTCTL_DB_PATH"); v != "" {
		c.DBPath = v
	}
	if v := os.Getenv("HOSTCTL_APP_URL_MODE"); v != "" {
		c.AppURLMode = v
	}
	if v := os.Getenv("HOSTCTL_APP_DOMAIN_SUFFIX"); v != "" {
		c.AppDomainSuffix = v
	}
	if v := os.Getenv("HOSTCTL_APP_URL_SCHEME"); v != "" {
		c.AppURLScheme = v
	}
	if v := os.Getenv("HOSTCTL_APP_URL_PORT"); v != "" {
		c.AppURLPort = v
	}
	if v := os.Getenv("HOSTCTL_CORS_ALLOW_ORIGINS"); v != "" {
		c.CORSAllowOrigins = v
	}
	if v := os.Getenv("HOSTCTL_EMBED_POLICY"); v != "" {
		c.EmbedPolicy = NormalizeEmbedPolicy(v)
	}
	if v := os.Getenv("HOSTCTL_EMBED_ALLOW_ORIGINS"); v != "" {
		c.EmbedAllowOrigins = v
	}
	if v := os.Getenv("HOSTCTL_MAX_SINGLE_FILE_BYTES"); v != "" {
		var n int64
		_, _ = fmt.Sscanf(v, "%d", &n)
		if n > 0 {
			c.MaxSingleFileBytes = n
		}
	}
	if v := os.Getenv("HOSTCTL_MAX_SITE_TOTAL_BYTES"); v != "" {
		var n int64
		_, _ = fmt.Sscanf(v, "%d", &n)
		if n > 0 {
			c.MaxSiteTotalBytes = n
		}
	}
	if v := os.Getenv("HOSTCTL_MAX_FILES_PER_SITE"); v != "" {
		var n int
		_, _ = fmt.Sscanf(v, "%d", &n)
		if n > 0 {
			c.MaxFilesPerSite = n
		}
	}
	if v := os.Getenv("HOSTCTL_COOLDOWN_SECONDS"); v != "" {
		var n int
		_, _ = fmt.Sscanf(v, "%d", &n)
		if n >= 0 {
			c.CooldownSeconds = n
		}
	}
	if v := os.Getenv("HOSTCTL_ANONYMOUS_DEPLOY_LIMIT"); v != "" {
		var n int
		_, _ = fmt.Sscanf(v, "%d", &n)
		if n >= -1 {
			c.AnonymousDeployLimit = n
		}
	}
	if v := os.Getenv("HOSTCTL_ADMIN_USERNAME"); v != "" {
		c.BootstrapAdminUsername = v
	}
	if v := os.Getenv("HOSTCTL_ADMIN_PASSWORD"); v != "" {
		c.BootstrapAdminPassword = v
	}
	if v := os.Getenv("HOSTCTL_EMAIL_VERIFICATION_ENABLED"); v != "" {
		c.EmailVerificationEnabled = parseBoolEnv(v)
	}
	if v := os.Getenv("HOSTCTL_SMTP_HOST"); v != "" {
		c.SMTPHost = v
	}
	if v := os.Getenv("HOSTCTL_SMTP_PORT"); v != "" {
		c.SMTPPort = v
	}
	if v := os.Getenv("HOSTCTL_SMTP_USERNAME"); v != "" {
		c.SMTPUsername = v
	}
	if v := os.Getenv("HOSTCTL_SMTP_PASSWORD"); v != "" {
		c.SMTPPassword = v
	}
	if v := os.Getenv("HOSTCTL_SMTP_FROM"); v != "" {
		c.SMTPFrom = v
	}
	if v := os.Getenv("HOSTCTL_SMTP_SECURE"); v != "" {
		c.SMTPSecure = v
	}
	if v := os.Getenv("HOSTCTL_STORAGE_BACKEND"); v != "" {
		c.StorageBackend = normalizeStorageBackend(v)
	}
	if v := os.Getenv("HOSTCTL_OSS_PROVIDER"); v != "" {
		c.OSSProvider = strings.ToLower(strings.TrimSpace(v))
	}
	if v := os.Getenv("HOSTCTL_OSS_ENDPOINT"); v != "" {
		c.OSSEndpoint = v
	}
	if v := os.Getenv("HOSTCTL_OSS_BUCKET"); v != "" {
		c.OSSBucket = v
	}
	if v := os.Getenv("HOSTCTL_OSS_ACCESS_KEY_ID"); v != "" {
		c.OSSAccessKeyID = v
	}
	if v := os.Getenv("HOSTCTL_OSS_ACCESS_KEY_SECRET"); v != "" {
		c.OSSAccessKeySecret = v
	}
	if v := os.Getenv("HOSTCTL_OSS_PREFIX"); v != "" {
		c.OSSPrefix = v
	}
	if v := os.Getenv("HOSTCTL_OSS_PUBLIC_BASE_URL"); v != "" {
		c.OSSPublicBaseURL = v
	}

	if v := os.Getenv("APP_URL_MODE"); v != "" {
		c.AppURLMode = v
	}
	if v := os.Getenv("APP_DOMAIN_SUFFIX"); v != "" {
		c.AppDomainSuffix = v
	}
	if v := os.Getenv("APP_URL_SCHEME"); v != "" {
		c.AppURLScheme = v
	}
	if v := os.Getenv("APP_URL_PORT"); v != "" {
		c.AppURLPort = v
	}
	if v := os.Getenv("CORS_ALLOW_ORIGINS"); v != "" {
		c.CORSAllowOrigins = v
	}
	if v := os.Getenv("EMBED_POLICY"); v != "" {
		c.EmbedPolicy = NormalizeEmbedPolicy(v)
	}
	if v := os.Getenv("EMBED_ALLOW_ORIGINS"); v != "" {
		c.EmbedAllowOrigins = v
	}
	if v := os.Getenv("MAX_SINGLE_FILE_BYTES"); v != "" {
		var n int64
		_, _ = fmt.Sscanf(v, "%d", &n)
		if n > 0 {
			c.MaxSingleFileBytes = n
		}
	}
	if v := os.Getenv("MAX_SITE_TOTAL_BYTES"); v != "" {
		var n int64
		_, _ = fmt.Sscanf(v, "%d", &n)
		if n > 0 {
			c.MaxSiteTotalBytes = n
		}
	}
	if v := os.Getenv("MAX_FILES_PER_SITE"); v != "" {
		var n int
		_, _ = fmt.Sscanf(v, "%d", &n)
		if n > 0 {
			c.MaxFilesPerSite = n
		}
	}
	if v := os.Getenv("COOLDOWN_SECONDS"); v != "" {
		var n int
		_, _ = fmt.Sscanf(v, "%d", &n)
		if n >= 0 {
			c.CooldownSeconds = n
		}
	}
	if v := os.Getenv("ANONYMOUS_DEPLOY_LIMIT"); v != "" {
		var n int
		_, _ = fmt.Sscanf(v, "%d", &n)
		if n >= -1 {
			c.AnonymousDeployLimit = n
		}
	}
	if v := os.Getenv("ADMIN_USERNAME"); v != "" {
		c.BootstrapAdminUsername = v
	}
	if v := os.Getenv("ADMIN_PASSWORD"); v != "" {
		c.BootstrapAdminPassword = v
	}
	if v := os.Getenv("EMAIL_VERIFICATION_ENABLED"); v != "" {
		c.EmailVerificationEnabled = parseBoolEnv(v)
	}
	if v := os.Getenv("SMTP_HOST"); v != "" {
		c.SMTPHost = v
	}
	if v := os.Getenv("SMTP_PORT"); v != "" {
		c.SMTPPort = v
	}
	if v := os.Getenv("SMTP_USERNAME"); v != "" {
		c.SMTPUsername = v
	}
	if v := os.Getenv("SMTP_PASSWORD"); v != "" {
		c.SMTPPassword = v
	}
	if v := os.Getenv("SMTP_FROM"); v != "" {
		c.SMTPFrom = v
	}
	if v := os.Getenv("SMTP_SECURE"); v != "" {
		c.SMTPSecure = v
	}
	if v := os.Getenv("STORAGE_BACKEND"); v != "" {
		c.StorageBackend = normalizeStorageBackend(v)
	}
	if v := os.Getenv("OSS_PROVIDER"); v != "" {
		c.OSSProvider = strings.ToLower(strings.TrimSpace(v))
	}
	if v := os.Getenv("OSS_ENDPOINT"); v != "" {
		c.OSSEndpoint = v
	}
	if v := os.Getenv("OSS_BUCKET"); v != "" {
		c.OSSBucket = v
	}
	if v := os.Getenv("OSS_ACCESS_KEY_ID"); v != "" {
		c.OSSAccessKeyID = v
	}
	if v := os.Getenv("OSS_ACCESS_KEY_SECRET"); v != "" {
		c.OSSAccessKeySecret = v
	}
	if v := os.Getenv("OSS_PREFIX"); v != "" {
		c.OSSPrefix = v
	}
	if v := os.Getenv("OSS_PUBLIC_BASE_URL"); v != "" {
		c.OSSPublicBaseURL = v
	}

	c.CORSAllowOrigins = NormalizeCORSAllowOrigins(c.CORSAllowOrigins)
	c.EmbedPolicy = NormalizeEmbedPolicy(c.EmbedPolicy)
	c.EmbedAllowOrigins = NormalizeOriginList(c.EmbedAllowOrigins)
	c.StorageBackend = normalizeStorageBackend(c.StorageBackend)
	c.SMTPSecure = strings.ToLower(strings.TrimSpace(c.SMTPSecure))

	if os.Getenv("HOSTCTL_DEV") == "1" {
		if c.HostedDir == "/var/www/hosted" {
			c.HostedDir = filepath.Join("data", "hosted")
		}
		if c.DBPath == "/var/lib/hostctl/hostctl.db" {
			c.DBPath = filepath.Join("data", "hostctl.db")
		}
		if os.Getenv("HOSTCTL_COOLDOWN_SECONDS") == "" {
			c.CooldownSeconds = 1
		}
	}

	return c
}

// Validate checks required runtime configuration.
func (c Config) Validate() error {
	if c.HTTPAddr == "" {
		return fmt.Errorf("HTTPAddr is empty")
	}
	if c.HostedDir == "" {
		return fmt.Errorf("HostedDir is empty")
	}
	if c.DBPath == "" {
		return fmt.Errorf("DBPath is empty")
	}
	switch c.AppURLMode {
	case "", "path", "domain", "dual":
	default:
		return fmt.Errorf("AppURLMode must be path, domain, or dual")
	}
	switch c.AppURLScheme {
	case "", "http", "https":
	default:
		return fmt.Errorf("AppURLScheme must be http or https")
	}
	switch c.EmbedPolicy {
	case "", "any", "self", "allowlist", "deny":
	default:
		return fmt.Errorf("EmbedPolicy must be any, self, allowlist, or deny")
	}
	if c.MaxSingleFileBytes <= 0 {
		return fmt.Errorf("MaxSingleFileBytes must be positive")
	}
	if c.MaxSiteTotalBytes <= 0 {
		return fmt.Errorf("MaxSiteTotalBytes must be positive")
	}
	if c.MaxFilesPerSite <= 0 {
		return fmt.Errorf("MaxFilesPerSite must be positive")
	}
	if c.CooldownSeconds < 0 {
		return fmt.Errorf("CooldownSeconds must be non-negative")
	}
	if c.AnonymousDeployLimit < -1 {
		return fmt.Errorf("AnonymousDeployLimit must be -1 or greater")
	}
	switch c.StorageBackend {
	case "", "local", "oss":
	default:
		return fmt.Errorf("StorageBackend must be local or oss")
	}
	if c.StorageBackend == "oss" {
		if c.OSSEndpoint == "" || c.OSSBucket == "" {
			return fmt.Errorf("OSS endpoint and bucket are required when StorageBackend is oss")
		}
	}
	if c.EmailVerificationEnabled && (c.SMTPHost == "" || c.SMTPFrom == "") {
		return fmt.Errorf("SMTP host and from are required when email verification is enabled")
	}
	return nil
}

func parseBoolEnv(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}

func normalizeStorageBackend(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "oss", "aliyun", "aliyun-oss", "object":
		return "oss"
	default:
		return "local"
	}
}

// NormalizeEmbedPolicy 规范化应用 iframe 嵌入策略。
func NormalizeEmbedPolicy(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "deny", "none", "block", "disabled":
		return "deny"
	case "self", "same-origin":
		return "self"
	case "allowlist", "allow_list", "origins":
		return "allowlist"
	default:
		return "any"
	}
}

// NormalizeCORSAllowOrigins 规范化 CORS 白名单配置。
// 当前版本默认关闭 CORS；星号通配符会被视为关闭。
func NormalizeCORSAllowOrigins(origins string) string {
	origins = strings.TrimSpace(origins)
	if origins == "*" {
		return ""
	}
	return NormalizeOriginList(origins)
}

// NormalizeOriginList 规范化 http(s) origin 列表，保留逗号分隔形式。
func NormalizeOriginList(origins string) string {
	fields := strings.FieldsFunc(origins, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	out := make([]string, 0, len(fields))
	for _, item := range fields {
		item = strings.TrimRight(strings.TrimSpace(item), "/")
		if item != "" {
			out = append(out, item)
		}
	}
	return strings.Join(out, ", ")
}
