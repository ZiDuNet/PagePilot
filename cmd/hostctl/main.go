// hostctl 是命令行客户端，封装 hostctl HTTP API。
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/yourorg/hostctl/internal/api"
	"github.com/yourorg/hostctl/internal/client"
)

var htmlTitleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

// 全局 flag
var (
	flagServer  string
	flagToken   string
	flagJSON    bool
	flagNoColor bool
)

// 配置文件路径
func configPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".hostctl", "config.json")
}

// loadConfig 从 ~/.hostctl/config.json 读取默认 server / token。
func loadConfig() map[string]string {
	cfg := map[string]string{}
	p := configPath()
	if p == "" {
		return cfg
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(b, &cfg)
	return cfg
}

// saveConfig 写配置。
func saveConfig(cfg map[string]string) error {
	p := configPath()
	if p == "" {
		return fmt.Errorf("no home directory")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}

// buildClient 用 flag + config 创建 client。
func buildClient() *client.Client {
	cfg := loadConfig()
	if flagServer == "" {
		flagServer = cfg["server"]
	}
	if flagServer == "" {
		flagServer = "http://localhost:8787"
	}
	if flagToken == "" {
		flagToken = cfg["token"]
	}
	if flagToken == "" {
		flagToken = os.Getenv("HOSTCTL_TOKEN")
	}
	return client.New(flagServer, flagToken)
}

// withSignalCancel 创建可被 Ctrl-C 取消的 context。
func withSignalCancel() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()
	return ctx, cancel
}

func main() {
	root := &cobra.Command{
		Use:   "hostctl",
		Short: "Static site hosting CLI for Agent-driven deploys",
		// 持久 flag（所有子命令可用）
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// cobra 默认会把 -h 处理掉；这里什么都不做
		},
	}
	root.PersistentFlags().StringVar(&flagServer, "server", "", "hostctl server URL (default: from ~/.hostctl/config.json or $HOSTCTL_SERVER)")
	root.PersistentFlags().StringVar(&flagToken, "token", "", "bearer token (default: from ~/.hostctl/config.json or $HOSTCTL_TOKEN)")
	root.PersistentFlags().BoolVar(&flagJSON, "json", false, "output structured JSON (Agent mode)")
	root.PersistentFlags().BoolVar(&flagNoColor, "no-color", false, "disable ANSI color output")

	root.AddCommand(
		cmdDeploy(),
		cmdAppend(),
		cmdVersions(),
		cmdGet(),
		cmdOverwrite(),
		cmdLock(),
		cmdUnlock(),
		cmdStatus(),
		cmdCurrent(),
		cmdDeleteVersion(),
		cmdMarket(),
		cmdLike(),
		cmdStrategy(),
		cmdAccess(),
		cmdAdmin(),
		cmdClaimSession(),
		cmdToken(),
		cmdConfig(),
	)

	if err := root.Execute(); err != nil {
		// cobra 已经打印了错误
		os.Exit(1)
	}
}

// ===== 输出工具 =====

func printErr(err error) {
	if apiErr, ok := err.(*client.APIError); ok && apiErr.Body != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", apiErr.Body.ErrorCode)
		fmt.Fprintf(os.Stderr, "  detail: %s\n", apiErr.Body.Detail)
		if apiErr.Body.Hint != "" {
			fmt.Fprintf(os.Stderr, "  hint:   %s\n", apiErr.Body.Hint)
		}
		if apiErr.Body.RetryAfterSeconds != nil {
			fmt.Fprintf(os.Stderr, "  retry:  %ds\n", *apiErr.Body.RetryAfterSeconds)
		}
		return
	}
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
}

// printDeployResult 打印 deploy 响应（人类可读 + JSON 两种）。
func printDeployResult(r *api.DeployResponse) {
	if flagJSON {
		_ = json.NewEncoder(os.Stdout).Encode(r)
		return
	}
	green, reset := color()
	fmt.Printf("%sDeployed successfully%s\n", green, reset)
	fmt.Printf("  URL:         %s\n", r.URL)
	fmt.Printf("  Detail URL:  %s\n", r.DetailURL)
	fmt.Printf("  Version URL: %s\n", r.VersionURL)
	fmt.Printf("  Code:        %s\n", r.Code)
	fmt.Printf("  Version:     %d\n", r.VersionNumber)
	fmt.Printf("  Size:        %d bytes\n", r.Size)
	fmt.Printf("  Created at:  %s\n", r.CreatedAt)
	if r.PreserveHint != "" {
		fmt.Printf("  Hint:        %s\n", r.PreserveHint)
	}
}

// color 返回 ANSI 颜色串（在 --no-color 或 stdout 非 TTY 时为空）。
func color() (string, string) {
	if flagNoColor {
		return "", ""
	}
	fi, _ := os.Stdout.Stat()
	if (fi.Mode() & os.ModeCharDevice) == 0 {
		return "", ""
	}
	return "\033[32m", "\033[0m"
}

// readSource 读取源（文件 / 目录），构造 DeployRequest 的 Content 或 Files。
func readSource(source string) (files []api.DeployFile, mainEntry string, err error) {
	info, err := os.Stat(source)
	if err != nil {
		return nil, "", fmt.Errorf("stat source: %w", err)
	}

	if !info.IsDir() {
		// 单文件
		b, err := os.ReadFile(source)
		if err != nil {
			return nil, "", err
		}
		// 推断是否二进制：检查前 512 字节是否含 \x00
		isBinary := looksBinary(b)
		if isBinary {
			files = []api.DeployFile{{
				Path:          filepath.Base(source),
				ContentBase64: base64.StdEncoding.EncodeToString(b),
			}}
		} else {
			files = []api.DeployFile{{
				Path:    filepath.Base(source),
				Content: string(b),
			}}
		}
		mainEntry = filepath.Base(source)
		return files, mainEntry, nil
	}

	// 目录：递归读取所有文件
	mainEntry = ""
	entries := []string{}
	err = filepath.Walk(source, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		entries = append(entries, p)
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	sort.Strings(entries)

	for _, p := range entries {
		rel, _ := filepath.Rel(source, p)
		rel = filepath.ToSlash(rel)
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, "", fmt.Errorf("read %s: %w", p, err)
		}
		if looksBinary(b) {
			files = append(files, api.DeployFile{
				Path:          rel,
				ContentBase64: base64.StdEncoding.EncodeToString(b),
			})
		} else {
			files = append(files, api.DeployFile{
				Path:    rel,
				Content: string(b),
			})
		}
		// 默认主入口优先级：index.html > 第一个 .html 文件 > 第一个文件
		if mainEntry == "" {
			mainEntry = rel
		}
		if rel == "index.html" {
			mainEntry = rel
		}
	}
	if mainEntry == "" {
		mainEntry = "index.html"
	}
	return files, mainEntry, nil
}

func deriveSiteTitle(files []api.DeployFile, mainEntry string) string {
	title := strings.TrimSpace(extractHTMLTitle(files, mainEntry))
	if title == "" {
		return ""
	}
	if strings.EqualFold(title, "index.html") || strings.EqualFold(title, "index.htm") {
		return ""
	}
	return title
}

func extractHTMLTitle(files []api.DeployFile, mainEntry string) string {
	mainEntry = strings.TrimSpace(mainEntry)
	if mainEntry == "" {
		return ""
	}
	for _, f := range files {
		if !strings.EqualFold(f.Path, mainEntry) || f.ContentBase64 != "" {
			continue
		}
		match := htmlTitleRe.FindStringSubmatch(f.Content)
		if len(match) < 2 {
			return ""
		}
		return strings.TrimSpace(htmlUnescape(match[1]))
	}
	return ""
}

func htmlUnescape(s string) string {
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#39;", "'")
	return s
}

// looksBinary 简单判断：含 \x00 或大量非可打印字节。
func looksBinary(b []byte) bool {
	n := len(b)
	if n == 0 {
		return false
	}
	checkLen := n
	if checkLen > 512 {
		checkLen = 512
	}
	nonPrintable := 0
	for i := 0; i < checkLen; i++ {
		c := b[i]
		if c == 0 {
			return true
		}
		if c < 0x09 || (c > 0x0d && c < 0x20) {
			nonPrintable++
		}
	}
	return nonPrintable*8 > checkLen // > 12.5%
}

// ===== 子命令：deploy =====

func cmdDeploy() *cobra.Command {
	var (
		description string
		title       string
		customCode  string
		filename    string
		accessPass  string
	)
	c := &cobra.Command{
		Use:   "deploy <source>",
		Short: "Deploy a static site from a file or directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if description == "" {
				return fmt.Errorf("--description is required (max 240 chars)")
			}
			files, mainEntry, err := readSource(args[0])
			if err != nil {
				return err
			}
			if filename != "" {
				mainEntry = filename
			}
			if title == "" {
				title = deriveSiteTitle(files, mainEntry)
			}
			req := api.DeployRequest{
				Description:    description,
				Title:          title,
				Filename:       mainEntry,
				Files:          files,
				Source:         "cli",
				AccessPassword: accessPass,
			}
			if customCode != "" {
				req.EnableCustomCode = true
				req.CustomCode = customCode
			}
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().Deploy(ctx, req)
			if err != nil {
				printErr(err)
				return errSilent
			}
			printDeployResult(resp)
			return nil
		},
	}
	c.Flags().StringVarP(&description, "description", "d", "", "deployment description (required, max 240 chars)")
	c.Flags().StringVarP(&title, "title", "t", "", "site title (optional metadata)")
	c.Flags().StringVarP(&customCode, "code", "c", "", "custom short code (^[a-z0-9-]{3,32}$)")
	c.Flags().StringVarP(&filename, "filename", "f", "", "main entry filename (default: index.html)")
	c.Flags().StringVar(&accessPass, "access-password", "", "optional visit password for a new user-owned site")
	_ = c.MarkFlagRequired("description")
	return c
}

// ===== 子命令：append =====

func cmdAppend() *cobra.Command {
	var (
		description string
		title       string
	)
	c := &cobra.Command{
		Use:   "append <code> <source>",
		Short: "Append a new version to an existing site",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if description == "" {
				return fmt.Errorf("--description is required")
			}
			files, mainEntry, err := readSource(args[1])
			if err != nil {
				return err
			}
			if title == "" {
				title = deriveSiteTitle(files, mainEntry)
			}
			req := api.DeployRequest{
				Description:      description,
				Title:            title,
				Filename:         mainEntry,
				Files:            files,
				EnableCustomCode: true,
				CustomCode:       args[0],
				CreateVersion:    true,
				Source:           "cli",
			}
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().Deploy(ctx, req)
			if err != nil {
				printErr(err)
				return errSilent
			}
			printDeployResult(resp)
			return nil
		},
	}
	c.Flags().StringVarP(&description, "description", "d", "", "version description (required, max 240 chars)")
	c.Flags().StringVarP(&title, "title", "t", "", "version title (optional metadata)")
	_ = c.MarkFlagRequired("description")
	return c
}

// ===== 子命令：versions =====

func cmdVersions() *cobra.Command {
	c := &cobra.Command{
		Use:   "versions <code>",
		Short: "List all versions of a site",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().ListVersions(ctx, args[0])
			if err != nil {
				printErr(err)
				return errSilent
			}
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(resp)
				return nil
			}
			if resp.CurrentVersion != nil {
				fmt.Printf("current: v%d\n", *resp.CurrentVersion)
			} else {
				fmt.Printf("current: (none)\n")
			}
			fmt.Println()
			fmt.Printf("%-7s %-8s %-9s %-9s %-30s %s\n", "VER", "LOCKED", "STATUS", "FILES", "DESCRIPTION", "CREATED")
			for _, v := range resp.Versions {
				locked := "no"
				if v.IsLocked {
					locked = "YES"
				}
				marker := "  "
				if v.IsCurrent {
					marker = "* "
				}
				desc := v.Description
				if len(desc) > 28 {
					desc = desc[:25] + "..."
				}
				fmt.Printf("%s%-6d %-8s %-9s %-9d %-30s %s\n",
					marker,
					v.VersionNumber, locked, v.Status, v.FileCount, desc,
					v.CreatedAt.Format("2006-01-02 15:04"))
			}
			return nil
		},
	}
	return c
}

// ===== 子命令：get =====

func cmdGet() *cobra.Command {
	var (
		version  int64
		output   string
		download bool
	)
	c := &cobra.Command{
		Use:   "get <code>",
		Short: "Get content metadata or download a version",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := withSignalCancel()
			defer cancel()
			var versionPtr *int64
			if cmd.Flags().Changed("version") {
				versionPtr = &version
			}
			cl := buildClient()
			if download {
				// 下载到 output 目录
				if output == "" {
					output = args[0]
				}
				if err := os.MkdirAll(output, 0o755); err != nil {
					return err
				}
				r, ct, err := cl.Download(ctx, args[0], versionPtr)
				if err != nil {
					printErr(err)
					return errSilent
				}
				defer r.Close()
				if strings.Contains(ct, "zip") {
					outFile := filepath.Join(output, fmt.Sprintf("%s.zip", args[0]))
					if versionPtr != nil {
						outFile = filepath.Join(output, fmt.Sprintf("%s-v%d.zip", args[0], *versionPtr))
					}
					f, err := os.Create(outFile)
					if err != nil {
						return err
					}
					defer f.Close()
					if _, err := io.Copy(f, r); err != nil {
						return err
					}
					if !flagJSON {
						fmt.Printf("Downloaded %s\n", outFile)
					}
				} else {
					// 单 HTML，直接保存
					outFile := filepath.Join(output, "index.html")
					f, err := os.Create(outFile)
					if err != nil {
						return err
					}
					defer f.Close()
					if _, err := io.Copy(f, r); err != nil {
						return err
					}
					if !flagJSON {
						fmt.Printf("Downloaded %s\n", outFile)
					}
				}
				return nil
			}
			resp, err := cl.GetContent(ctx, args[0], versionPtr)
			if err != nil {
				printErr(err)
				return errSilent
			}
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(resp)
				return nil
			}
			fmt.Printf("code:        %s\n", resp.Code)
			fmt.Printf("version:     %d\n", resp.Version)
			if resp.Title != "" {
				fmt.Printf("title:       %s\n", resp.Title)
			}
			fmt.Printf("description: %s\n", resp.Description)
			fmt.Printf("mainEntry:   %s\n", resp.MainEntry)
			fmt.Printf("totalSize:   %d bytes\n", resp.TotalSize)
			fmt.Printf("locked:      %t\n", resp.IsLocked)
			fmt.Printf("created:     %s\n", resp.CreatedAt.Format("2006-01-02 15:04"))
			fmt.Println()
			fmt.Printf("%-30s %-10s %s\n", "PATH", "SIZE", "SHA256")
			for _, f := range resp.Files {
				sha := f.Sha256
				if len(sha) > 16 {
					sha = sha[:16] + "..."
				}
				fmt.Printf("%-30s %-10d %s\n", f.Path, f.Size, sha)
			}
			return nil
		},
	}
	c.Flags().Int64Var(&version, "version", 0, "version number (default: current)")
	c.Flags().StringVarP(&output, "output", "o", "", "output directory for --download")
	c.Flags().BoolVar(&download, "download", false, "download files instead of showing metadata")
	return c
}

// ===== 子命令：overwrite =====

func cmdOverwrite() *cobra.Command {
	var description string
	c := &cobra.Command{
		Use:   "overwrite <code> <version> <source>",
		Short: "Overwrite content of an unlocked version",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			if description == "" {
				return fmt.Errorf("--description is required")
			}
			var version int64
			if _, err := fmt.Sscanf(args[1], "%d", &version); err != nil {
				return fmt.Errorf("invalid version %q: %v", args[1], err)
			}
			files, mainEntry, err := readSource(args[2])
			if err != nil {
				return err
			}
			req := api.OverwriteRequest{
				Description: description,
				Filename:    mainEntry,
				Files:       files,
			}
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().Overwrite(ctx, args[0], version, req)
			if err != nil {
				printErr(err)
				return errSilent
			}
			printDeployResult(resp)
			return nil
		},
	}
	c.Flags().StringVarP(&description, "description", "d", "", "version description (required)")
	_ = c.MarkFlagRequired("description")
	return c
}

// ===== 子命令：lock / unlock =====

func cmdLock() *cobra.Command {
	return &cobra.Command{
		Use:   "lock <code> <version>",
		Short: "Lock a version (no further modifications or deletions)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var version int64
			if _, err := fmt.Sscanf(args[1], "%d", &version); err != nil {
				return err
			}
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().Lock(ctx, args[0], version, true)
			if err != nil {
				printErr(err)
				return errSilent
			}
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(resp)
				return nil
			}
			fmt.Printf("Locked %s v%d\n", resp.Code, resp.VersionNumber)
			return nil
		},
	}
}

func cmdUnlock() *cobra.Command {
	return &cobra.Command{
		Use:   "unlock <code> <version>",
		Short: "Unlock a previously locked version",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var version int64
			if _, err := fmt.Sscanf(args[1], "%d", &version); err != nil {
				return err
			}
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().Lock(ctx, args[0], version, false)
			if err != nil {
				printErr(err)
				return errSilent
			}
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(resp)
				return nil
			}
			fmt.Printf("Unlocked %s v%d\n", resp.Code, resp.VersionNumber)
			return nil
		},
	}
}

// ===== 子命令：status =====

func cmdStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status <code> <version> <active|inactive>",
		Short: "Set version status (active/inactive)",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			var version int64
			if _, err := fmt.Sscanf(args[1], "%d", &version); err != nil {
				return err
			}
			status := args[2]
			if status != "active" && status != "inactive" {
				return fmt.Errorf("status must be 'active' or 'inactive'")
			}
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().SetStatus(ctx, args[0], version, status)
			if err != nil {
				printErr(err)
				return errSilent
			}
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(resp)
				return nil
			}
			fmt.Printf("Set %s v%d status=%s\n", resp.Code, resp.VersionNumber, status)
			return nil
		},
	}
}

// ===== 子命令：current =====

func cmdCurrent() *cobra.Command {
	return &cobra.Command{
		Use:   "current <code> <version>",
		Short: "Switch the current live version",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var version int64
			if _, err := fmt.Sscanf(args[1], "%d", &version); err != nil {
				return err
			}
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().SetCurrent(ctx, args[0], version)
			if err != nil {
				printErr(err)
				return errSilent
			}
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(resp)
				return nil
			}
			fmt.Printf("Switched %s to v%d\n", resp.Code, resp.CurrentVersion)
			return nil
		},
	}
}

// ===== 子命令：delete-version =====

func cmdDeleteVersion() *cobra.Command {
	var confirm bool
	c := &cobra.Command{
		Use:   "delete-version <code> <version>",
		Short: "Delete a version (cannot be undone)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirm {
				return fmt.Errorf("add --confirm to actually delete (this is irreversible)")
			}
			var version int64
			if _, err := fmt.Sscanf(args[1], "%d", &version); err != nil {
				return err
			}
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().DeleteVersion(ctx, args[0], version)
			if err != nil {
				printErr(err)
				return errSilent
			}
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(resp)
				return nil
			}
			fmt.Printf("Deleted %s v%d (current now v%d)\n", resp.Code, version, resp.CurrentVersion)
			return nil
		},
	}
	c.Flags().BoolVar(&confirm, "confirm", false, "confirm deletion")
	return c
}

// ===== 子命令：token =====

func cmdToken() *cobra.Command {
	root := &cobra.Command{
		Use:   "token",
		Short: "Manage bearer tokens",
	}

	// token create
	var isAdmin bool
	var ttl string
	var expiresAt string
	createC := &cobra.Command{
		Use:   "create [label]",
		Short: "Create a new token (plaintext is shown only once)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			label := ""
			if len(args) > 0 {
				label = args[0]
			}
			ctx, cancel := withSignalCancel()
			defer cancel()
			ttlSeconds, err := parseTTLSeconds(ttl)
			if err != nil {
				return err
			}
			resp, err := buildClient().CreateToken(ctx, label, isAdmin, expiresAt, ttlSeconds)
			if err != nil {
				printErr(err)
				return errSilent
			}
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(resp)
				return nil
			}
			green, reset := color()
			fmt.Printf("%sToken created (store it now; you won't see it again)%s\n", green, reset)
			fmt.Printf("  ID:     %s\n", resp.ID)
			fmt.Printf("  Label:  %s\n", resp.Label)
			fmt.Printf("  Token:  %s\n", resp.Token)
			if resp.ExpiresAt != nil {
				fmt.Printf("  Expires:%s\n", resp.ExpiresAt.Format(time.RFC3339))
			} else {
				fmt.Printf("  Expires: permanent\n")
			}
			if resp.IsAdmin {
				fmt.Printf("  Admin:  yes\n")
			}
			return nil
		},
	}
	createC.Flags().BoolVar(&isAdmin, "admin", false, "create an admin token")
	createC.Flags().StringVar(&ttl, "ttl", "", "temporary token lifetime, for example 24h or 30m")
	createC.Flags().StringVar(&expiresAt, "expires-at", "", "absolute expiry time in RFC3339 format")

	// token list
	listC := &cobra.Command{
		Use:   "list",
		Short: "List all tokens (no plaintext)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().ListTokens(ctx)
			if err != nil {
				printErr(err)
				return errSilent
			}
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(resp)
				return nil
			}
			fmt.Printf("%-36s %-8s %-7s %-9s %-20s %s\n", "ID", "ADMIN", "REVOKED", "LABEL", "EXPIRES", "CREATED")
			for _, t := range resp.Tokens {
				admin := "no"
				if t.IsAdmin {
					admin = "yes"
				}
				revoked := "no"
				if t.IsRevoked {
					revoked = "YES"
				}
				expires := "permanent"
				if t.ExpiresAt != nil {
					expires = t.ExpiresAt.Format("2006-01-02 15:04")
				}
				fmt.Printf("%-36s %-8s %-7s %-9s %-20s %s\n",
					t.ID, admin, revoked, t.Label, expires,
					t.CreatedAt.Format("2006-01-02"))
			}
			return nil
		},
	}

	// token revoke
	revokeC := &cobra.Command{
		Use:   "revoke <id>",
		Short: "Revoke a token (cannot be undone)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().RevokeToken(ctx, args[0])
			if err != nil {
				printErr(err)
				return errSilent
			}
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(resp)
				return nil
			}
			fmt.Printf("Revoked %s\n", resp.ID)
			return nil
		},
	}

	root.AddCommand(createC, listC, revokeC)
	return root
}

// ===== 子命令：admin =====

func cmdAdmin() *cobra.Command {
	root := &cobra.Command{
		Use:   "admin",
		Short: "Admin site operations",
	}

	var unpin bool
	pinC := &cobra.Command{
		Use:   "pin-site <code>",
		Short: "Pin or unpin a marketplace site",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().SetSitePin(ctx, args[0], !unpin)
			if err != nil {
				printErr(err)
				return errSilent
			}
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(resp)
				return nil
			}
			if unpin {
				fmt.Printf("Unpinned marketplace site %s\n", args[0])
			} else {
				fmt.Printf("Pinned marketplace site %s\n", args[0])
			}
			return nil
		},
	}
	pinC.Flags().BoolVar(&unpin, "unpin", false, "clear the marketplace pin")
	root.AddCommand(pinC)
	return root
}

func parseTTLSeconds(value string) (*int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return nil, fmt.Errorf("--ttl must be a Go duration such as 30m, 24h, or 168h: %w", err)
	}
	if d <= 0 {
		return nil, fmt.Errorf("--ttl must be positive")
	}
	seconds := int64(d.Seconds())
	if seconds <= 0 {
		seconds = 1
	}
	return &seconds, nil
}

func cmdClaimSession() *cobra.Command {
	return &cobra.Command{
		Use:   "claim-session <session-id>",
		Short: "Claim anonymous-session deployments for the current token/user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().ClaimAnonymousSession(ctx, args[0])
			if err != nil {
				printErr(err)
				return errSilent
			}
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(resp)
				return nil
			}
			fmt.Printf("Claimed session %s for user %s: %d sites, %d deploys\n",
				resp.SessionID, resp.UserID, resp.SiteCount, resp.DeployCount)
			return nil
		},
	}
}

// ===== 子命令：config =====

func cmdConfig() *cobra.Command {
	root := &cobra.Command{
		Use:   "config",
		Short: "Manage ~/.hostctl/config.json",
	}

	setC := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set config key (server / token)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			cfg[args[0]] = args[1]
			if err := saveConfig(cfg); err != nil {
				return err
			}
			fmt.Printf("Set %s\n", args[0])
			return nil
		},
	}

	getC := &cobra.Command{
		Use:   "get <key>",
		Short: "Get config value (token is masked)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			v, ok := cfg[args[0]]
			if !ok {
				fmt.Println("(unset)")
				return nil
			}
			if args[0] == "token" && len(v) > 8 {
				v = v[:4] + "..." + v[len(v)-4:]
			}
			fmt.Println(v)
			return nil
		},
	}

	showC := &cobra.Command{
		Use:   "show",
		Short: "Show all config (token masked)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			if len(cfg) == 0 {
				fmt.Println("(empty)")
				return nil
			}
			for k, v := range cfg {
				if k == "token" && len(v) > 8 {
					v = v[:4] + "..." + v[len(v)-4:]
				}
				fmt.Printf("%s: %s\n", k, v)
			}
			return nil
		},
	}

	root.AddCommand(setC, getC, showC)
	return root
}

// ===== 子命令：market（应用市场，公开 API，无需 token） =====

func cmdMarket() *cobra.Command {
	root := &cobra.Command{
		Use:   "market",
		Short: "Browse the public marketplace (no token required)",
	}

	// market search [query]
	var (
		mSort     string
		mPage     int
		mPageSize int
	)
	searchC := &cobra.Command{
		Use:   "search [query]",
		Short: "Search / browse deploys in the marketplace",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := ""
			if len(args) > 0 {
				q = args[0]
			}
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().SearchMarketplace(ctx, q, mSort, mPage, mPageSize)
			if err != nil {
				printErr(err)
				return errSilent
			}
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(resp)
				return nil
			}
			printMarketplaceList(resp)
			return nil
		},
	}
	searchC.Flags().StringVar(&mSort, "sort", "newest", "sort: newest / oldest / likes_desc / views_desc")
	searchC.Flags().IntVar(&mPage, "page", 1, "page number")
	searchC.Flags().IntVar(&mPageSize, "page-size", 24, "page size (max 50)")

	// market show <publicId|code>
	showC := &cobra.Command{
		Use:   "show <public-id|code>",
		Short: "Show a single deploy's marketplace metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().GetDeployDetail(ctx, args[0])
			if err != nil {
				printErr(err)
				return errSilent
			}
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(resp)
				return nil
			}
			printMarketplaceDetail(resp)
			return nil
		},
	}

	root.AddCommand(searchC, showC)
	return root
}

// printMarketplaceList 把 SearchMarketplace 的 map 渲染成表格。
func printMarketplaceList(resp map[string]any) {
	deploys, _ := resp["deploys"].([]any)
	total, _ := resp["total"].(float64)
	page, _ := resp["page"].(float64)
	pageSize, _ := resp["pageSize"].(float64)
	if len(deploys) == 0 {
		fmt.Println("No deploys found.")
		return
	}
	fmt.Printf("Showing page %d of ~%d (total %d, page size %d)\n\n",
		int(page), (int(total)+int(pageSize)-1)/int(pageSize), int(total), int(pageSize))
	fmt.Printf("%-12s %-30s %-8s %-7s %-7s %s\n", "CODE", "TITLE", "SIZE", "LIKES", "VIEWS", "CREATED")
	for _, it := range deploys {
		m, _ := it.(map[string]any)
		code, _ := m["code"].(string)
		title, _ := m["title"].(string)
		if len(title) > 28 {
			title = title[:25] + "..."
		}
		size := toInt64(m["fileSize"])
		likes := toInt64(m["likeCount"])
		views := toInt64(m["viewCount"])
		created := prettyTime(m["createdAt"])
		fmt.Printf("%-12s %-30s %-8d %-7d %-7d %s\n", code, title, size, likes, views, created)
	}
}

// printMarketplaceDetail 把单条详情 map 渲染成多行 key:value。
func printMarketplaceDetail(m map[string]any) {
	keys := []string{"id", "code", "title", "description", "filename", "filePath",
		"fileSize", "status", "primaryVersionStrategy", "primaryVersionId",
		"currentVersionId", "versionCount", "viewCount", "likeCount",
		"qrCodePath", "createdAt", "updatedAt"}
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		fmt.Printf("%-24s %v\n", k+":", v)
	}
}

// prettyTime 把 ISO 时间字符串截短到分钟，便于表格展示。
func prettyTime(v any) string {
	s, _ := v.(string)
	if s == "" {
		return "-"
	}
	if len(s) >= 16 {
		s = s[:16]
	}
	return strings.ReplaceAll(s, "T", " ")
}

// toInt64 处理 JSON 数字（默认解析为 float64）。
func toInt64(v any) int64 {
	if f, ok := v.(float64); ok {
		return int64(f)
	}
	return 0
}

// ===== 子命令：like =====

func cmdLike() *cobra.Command {
	return &cobra.Command{
		Use:   "like <code>",
		Short: "Like a deploy. Likes are deduplicated and only affect marketplace ranking.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().LikeDeploy(ctx, args[0])
			if err != nil {
				printErr(err)
				return errSilent
			}
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(resp)
				return nil
			}
			count, _ := resp["likeCount"].(float64)
			fmt.Printf("Liked %s (total likes: %d)\n", args[0], int64(count))
			return nil
		},
	}
}

// ===== 子命令：strategy =====

func cmdStrategy() *cobra.Command {
	return &cobra.Command{
		Use:   "strategy <code> <likes|latest>",
		Short: "Switch primary version strategy: likes (default, top-liked) or latest (newest version)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			strategy := args[1]
			if strategy != "likes" && strategy != "latest" {
				return fmt.Errorf("strategy must be 'likes' or 'latest'")
			}
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().SetPrimaryStrategy(ctx, args[0], strategy)
			if err != nil {
				printErr(err)
				return errSilent
			}
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(resp)
				return nil
			}
			fmt.Printf("Set %s primary strategy = %s (primary version now v%d)\n",
				resp.Code, resp.PrimaryVersionStrategy, resp.PrimaryVersionNumber)
			return nil
		},
	}
}

// ===== 子命令：access =====

func cmdAccess() *cobra.Command {
	var (
		password string
		clear    bool
	)
	c := &cobra.Command{
		Use:   "access <code>",
		Short: "Set or clear a site's visit password",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clear {
				password = ""
			} else if len(strings.TrimSpace(password)) < 4 {
				return fmt.Errorf("--password must be at least 4 characters, or pass --clear")
			}
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().SetSiteAccessPassword(ctx, args[0], password)
			if err != nil {
				printErr(err)
				return errSilent
			}
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(resp)
				return nil
			}
			if clear {
				fmt.Printf("Cleared visit password for %s\n", args[0])
			} else {
				fmt.Printf("Set visit password for %s\n", args[0])
			}
			return nil
		},
	}
	c.Flags().StringVar(&password, "password", "", "visit password, at least 4 characters")
	c.Flags().BoolVar(&clear, "clear", false, "clear password protection")
	return c
}

// errSilent 表示错误已经被 printErr 打印过，cobra 不要再打印。
var errSilent = fmt.Errorf("__silent__")
