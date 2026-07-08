// hostctl-server 是控制平面入口。
package main

import (
	"context"
	"encoding/base64"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/yourorg/hostctl/internal/api"
	"github.com/yourorg/hostctl/internal/auth"
	"github.com/yourorg/hostctl/internal/config"
	"github.com/yourorg/hostctl/internal/deploy"
	"github.com/yourorg/hostctl/internal/store"
)

// loadMasterKey 从环境变量加载 AES-256 主密钥。
// 优先级：HOSTCTL_MASTER_KEY > dev 默认密钥。
// 生产环境未设置时启动失败。
func loadMasterKey() ([32]byte, error) {
	var key [32]byte
	raw := strings.TrimSpace(os.Getenv("HOSTCTL_MASTER_KEY"))
	if raw == "" {
		if isDev() {
			copy(key[:], "pagepilot-dev-master-key-0000000")
			return key, nil
		}
		return key, errMasterKeyMissing
	}

	// 接受 base64 或原始 32 字节字符串
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil && len(decoded) == 32 {
		copy(key[:], decoded)
		return key, nil
	}
	if len(raw) >= 32 {
		copy(key[:], []byte(raw)[:32])
		return key, nil
	}
	return key, errMasterKeyLength
}

var (
	errMasterKeyMissing = &configError{msg: "HOSTCTL_MASTER_KEY is required in production (set HOSTCTL_DEV=1 to use dev fallback)"}
	errMasterKeyLength  = &configError{msg: "HOSTCTL_MASTER_KEY must decode to exactly 32 bytes"}
)

type configError struct{ msg string }

func (e *configError) Error() string { return e.msg }

func isDev() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("HOSTCTL_DEV"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func main() {
	cfg := config.Default()

	// CLI flag 覆盖（环境变量已由 config.Default 处理）
	flag.StringVar(&cfg.HTTPAddr, "addr", cfg.HTTPAddr, "HTTP listen address")
	flag.StringVar(&cfg.HostedDir, "hosted-dir", cfg.HostedDir, "static files root directory")
	flag.StringVar(&cfg.DBPath, "db", cfg.DBPath, "SQLite database path")
	dev := flag.Bool("dev", false, "enable dev mode (uses ./data/ for paths)")
	requireAuth := flag.Bool("require-auth", false, "require Bearer token on all write operations")
	flag.Parse()

	if *dev {
		os.Setenv("HOSTCTL_DEV", "1")
		cfg = config.Default()
		// flag 已经覆盖过的，重新覆盖一次
		flag.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "addr":
				cfg.HTTPAddr = f.Value.String()
			case "hosted-dir":
				cfg.HostedDir = f.Value.String()
			case "db":
				cfg.DBPath = f.Value.String()
			}
		})
	}

	// REQUIRE_AUTH 环境变量（便于 docker compose / systemd 配置）
	if !*requireAuth {
		if v := strings.ToLower(strings.TrimSpace(os.Getenv("REQUIRE_AUTH"))); v == "1" || v == "true" || v == "yes" || v == "on" {
			*requireAuth = true
		}
	}

	if err := cfg.Validate(); err != nil {
		log.Fatalf("config invalid: %v", err)
	}

	// 加载 AES 主密钥（SEC-01：生产环境强制配置）
	masterKey, err := loadMasterKey()
	if err != nil {
		if *dev || isDev() {
			log.Printf("WARNING: using insecure dev master key (only safe in dev mode)")
		} else {
			log.Fatalf("master key: %v", err)
		}
	}
	auth.SetMasterKey(masterKey)

	// 确保静态根目录存在
	if err := os.MkdirAll(cfg.HostedDir, 0o755); err != nil {
		log.Fatalf("mkdir hosted dir %s: %v", cfg.HostedDir, err)
	}

	// 打开 SQLite
	st, err := store.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	// 构造 auth + deployer
	authSvc := auth.New(st)
	if err := authSvc.EnsureBootstrapAdmin(context.Background(), cfg.BootstrapAdminUsername, cfg.BootstrapAdminPassword); err != nil {
		log.Fatalf("bootstrap admin: %v", err)
	}
	deployer := deploy.New(cfg, st)

	// 启动时从数据库恢复持久化设置。
	cfg = deployer.LoadPersistedSettings(context.Background())

	// 构造 server
	srv := api.New(cfg, deployer, authSvc, *requireAuth, log.Default()).
		WithVersion("0.3.0")

	// 信号处理：优雅退出
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down...")
		_ = st.Close()
		os.Exit(0)
	}()

	// 启动
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
