// hostctl-server 是控制平面入口。
package main

import (
	"context"
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
		WithVersion("0.2.0")

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
