package api

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/yourorg/hostctl/internal/config"
)

const (
	AppURLModePath   = "path"
	AppURLModeDomain = "domain"
	AppURLModeDual   = "dual"
)

type AppURLConfig struct {
	AppURLMode      string `json:"appURLMode"`
	AppDomainSuffix string `json:"appDomainSuffix,omitempty"`
	AppURLScheme    string `json:"appURLScheme"`
	AppURLPort      string `json:"appURLPort,omitempty"`
	AppPathBase     string `json:"appPathBase"`
	PathBaseURL     string `json:"-"`
}

func normalizeAppURLMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case AppURLModeDomain:
		return AppURLModeDomain
	case AppURLModeDual:
		return AppURLModeDual
	default:
		return AppURLModePath
	}
}

func NormalizeAppURLModeForConfig(mode string) string {
	return normalizeAppURLMode(mode)
}

func normalizeAppURLScheme(scheme string) string {
	switch strings.ToLower(strings.TrimSpace(scheme)) {
	case "http":
		return "http"
	default:
		return "https"
	}
}

func NormalizeAppURLSchemeForConfig(scheme string) string {
	return normalizeAppURLScheme(scheme)
}

func normalizeAppDomainSuffix(suffix string) string {
	suffix = strings.ToLower(strings.TrimSpace(suffix))
	suffix = strings.TrimPrefix(suffix, "*.")
	suffix = strings.TrimPrefix(suffix, ".")
	return strings.TrimSuffix(suffix, ".")
}

func NormalizeAppDomainSuffixForConfig(suffix string) string {
	return normalizeAppDomainSuffix(suffix)
}

func normalizeAppURLPort(port string) string {
	port = strings.TrimSpace(port)
	if port == "" {
		return ""
	}
	n, err := strconv.Atoi(port)
	if err != nil || n <= 0 || n > 65535 {
		return ""
	}
	return strconv.Itoa(n)
}

func NormalizeAppURLPortForConfig(port string) string {
	return normalizeAppURLPort(port)
}

func NewAppURLConfig(cfg config.Config) AppURLConfig {
	return AppURLConfig{
		AppURLMode:      normalizeAppURLMode(cfg.AppURLMode),
		AppDomainSuffix: normalizeAppDomainSuffix(cfg.AppDomainSuffix),
		AppURLScheme:    normalizeAppURLScheme(cfg.AppURLScheme),
		AppURLPort:      normalizeAppURLPort(cfg.AppURLPort),
		AppPathBase:     "/agent",
	}
}

func (c AppURLConfig) WithPathBaseURL(baseURL string) AppURLConfig {
	c.PathBaseURL = strings.TrimRight(baseURL, "/")
	return c
}

func (c AppURLConfig) PrimaryAppURL(code string, version *int64) string {
	if c.AppURLMode == AppURLModeDomain && c.AppDomainSuffix != "" {
		return c.DomainAppURL(code, version)
	}
	return c.PathAppURL(code, version)
}

func (c AppURLConfig) PathAppURL(code string, version *int64) string {
	base := strings.TrimRight(c.PathBaseURL, "/")
	pathBase := strings.TrimRight(c.AppPathBase, "/")
	if pathBase == "" {
		pathBase = "/agent"
	}
	if version != nil && *version > 0 {
		return base + pathBase + "/" + code + "/versions/" + strconv.FormatInt(*version, 10) + "/"
	}
	return base + pathBase + "/" + code + "/"
}

func (c AppURLConfig) DomainAppURL(code string, version *int64) string {
	if c.AppDomainSuffix == "" {
		return c.PathAppURL(code, version)
	}
	host := code + "." + c.AppDomainSuffix
	if c.AppURLPort != "" && !isDefaultPort(c.AppURLScheme, c.AppURLPort) {
		host = net.JoinHostPort(host, c.AppURLPort)
	}
	u := url.URL{Scheme: c.AppURLScheme, Host: host, Path: "/"}
	if version != nil && *version > 0 {
		u.Path = "/versions/" + strconv.FormatInt(*version, 10) + "/"
	}
	return u.String()
}

func (c AppURLConfig) AssetURL(code string, version int64, path string) string {
	if c.AppURLMode == AppURLModeDomain && c.AppDomainSuffix != "" {
		baseVersion := c.DomainAppURL(code, &version)
		return strings.TrimRight(baseVersion, "/") + "/" + strings.TrimLeft(path, "/")
	}
	return strings.TrimRight(c.PathAppURL(code, &version), "/") + "/" + strings.TrimLeft(path, "/")
}

func (c AppURLConfig) CodeFromRequestHost(r *http.Request) string {
	suffix := c.AppDomainSuffix
	if suffix == "" || c.AppURLMode == AppURLModePath {
		return ""
	}
	host := forwardedHost(r)
	host = stripHostPort(host)
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" || host == suffix {
		return ""
	}
	needle := "." + suffix
	if !strings.HasSuffix(host, needle) {
		return ""
	}
	code := strings.TrimSuffix(host, needle)
	if strings.Contains(code, ".") {
		return ""
	}
	return code
}

func forwardedHost(r *http.Request) string {
	if h := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); h != "" {
		return strings.TrimSpace(strings.Split(h, ",")[0])
	}
	return r.Host
}

func stripHostPort(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	if i := strings.LastIndex(host, ":"); i > -1 && strings.Count(host, ":") == 1 {
		if _, err := strconv.Atoi(host[i+1:]); err == nil {
			return host[:i]
		}
	}
	return host
}

func isDefaultPort(scheme, port string) bool {
	return (scheme == "https" && port == "443") || (scheme == "http" && port == "80")
}
