package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config is the hostctl-server runtime configuration.
type Config struct {
	HTTPAddr         string
	HostedDir        string
	DBPath           string
	PublicBaseURL    string
	AppURLMode       string
	AppDomainSuffix  string
	AppURLScheme     string
	AppURLPort       string
	CORSAllowOrigins string

	MaxSingleFileBytes int64
	MaxSiteTotalBytes  int64
	MaxFilesPerSite    int

	CooldownSeconds      int
	AnonymousDeployLimit int

	BootstrapAdminUsername string
	BootstrapAdminPassword string
}

// Default returns runtime defaults, overridable by environment variables.
func Default() Config {
	c := Config{
		HTTPAddr:             "127.0.0.1:8787",
		HostedDir:            "/var/www/hosted",
		DBPath:               "/var/lib/hostctl/hostctl.db",
		PublicBaseURL:        "http://localhost:8787",
		AppURLMode:           "path",
		AppURLScheme:         "https",
		CORSAllowOrigins:     "",
		MaxSingleFileBytes:   1 << 20,
		MaxSiteTotalBytes:    10 << 20,
		MaxFilesPerSite:      100,
		CooldownSeconds:      10,
		AnonymousDeployLimit: 5,
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
	if v := os.Getenv("HOSTCTL_PUBLIC_BASE_URL"); v != "" {
		c.PublicBaseURL = v
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

	if v := os.Getenv("PUBLIC_BASE_URL"); v != "" {
		c.PublicBaseURL = v
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

	c.CORSAllowOrigins = NormalizeCORSAllowOrigins(c.CORSAllowOrigins)

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
	if c.PublicBaseURL == "" {
		return fmt.Errorf("PublicBaseURL is empty")
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
	return nil
}

// NormalizeCORSAllowOrigins 规范化 CORS 白名单配置。
// 当前版本默认关闭 CORS；星号通配符会被视为关闭。
func NormalizeCORSAllowOrigins(origins string) string {
	origins = strings.TrimSpace(origins)
	if origins == "*" {
		return ""
	}
	return origins
}
