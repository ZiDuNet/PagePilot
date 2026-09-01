// pagep 是 PagePilot 命令行客户端，封装 PagePilot HTTP API。
package main

import (
	"archive/zip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/yourorg/hostctl/internal/api"
	"github.com/yourorg/hostctl/internal/client"
	appversion "github.com/yourorg/hostctl/internal/version"
)

var htmlTitleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

// 全局 flag
var (
	flagServer  string
	flagToken   string
	flagJSON    bool
	flagNoColor bool
)

const defaultServer = "https://pagepilot.dell.4dbim.cc:1143/"

// 配置文件路径
func configPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	pagepPath := filepath.Join(home, ".pagep", "config.json")
	legacyPath := filepath.Join(home, ".hostctl", "config.json")
	if _, err := os.Stat(legacyPath); err == nil {
		if _, pagepErr := os.Stat(pagepPath); os.IsNotExist(pagepErr) {
			return legacyPath
		}
	}
	return pagepPath
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

// loadConfig 从 ~/.pagep/config.json 读取默认 server / token。
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

func normalizeServerAddress(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

// savedTokenForServer prevents a token saved for one PagePilot instance from
// being sent to a different server selected with --server or environment.
func savedTokenForServer(cfg map[string]string, server string) string {
	token := strings.TrimSpace(cfg["token"])
	if token == "" {
		return ""
	}
	savedServer := normalizeServerAddress(cfg["server"])
	if savedServer != "" && savedServer != normalizeServerAddress(server) {
		return ""
	}
	return token
}

func maskedToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "..." + token[len(token)-4:]
}

func resolvedToken(cfg map[string]string, server, explicit string) string {
	if token := strings.TrimSpace(explicit); token != "" {
		return token
	}
	if token := firstEnv("PAGEPILOT_TOKEN", "HOSTCTL_TOKEN"); token != "" {
		return token
	}
	return savedTokenForServer(cfg, server)
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
		flagServer = firstEnv("PAGEPILOT_SERVER", "HOSTCTL_SERVER")
	}
	if flagServer == "" {
		flagServer = defaultServer
	}
	flagToken = resolvedToken(cfg, flagServer, flagToken)
	return client.New(flagServer, flagToken)
}

// withSignalCancel 创建可被 Ctrl-C 取消的 context。
func withSignalCancel() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			signal.Stop(sigCh)
			cancel()
		})
	}
	go func() {
		select {
		case <-sigCh:
			stop()
		case <-ctx.Done():
			stop()
		}
	}()
	return ctx, stop
}

func main() {
	root := &cobra.Command{
		Use:   "pagep",
		Short: "PagePilot CLI for Agent-driven application publishing",
		// 抑制 cobra 内置错误输出（printErr 已负责友好输出）
		SilenceErrors: true,
		SilenceUsage:  true,
		// 持久 flag（所有子命令可用）
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// cobra 默认会把 -h 处理掉；这里什么都不做
		},
	}
	root.PersistentFlags().StringVar(&flagServer, "server", "", "PagePilot server URL (default: from ~/.pagep/config.json or $PAGEPILOT_SERVER)")
	root.PersistentFlags().StringVar(&flagToken, "token", "", "bearer token (default: from ~/.pagep/config.json or $PAGEPILOT_TOKEN)")
	root.PersistentFlags().BoolVar(&flagJSON, "json", false, "output structured JSON (Agent mode)")
	root.PersistentFlags().BoolVar(&flagNoColor, "no-color", false, "disable ANSI color output")

	root.AddCommand(
		cmdVersion(),
		cmdDoctor(),
		cmdPreflight(),
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
		// Commands that call printErr return errSilent to avoid duplicate output.
		// Validation and Cobra parsing errors can arrive here directly, so render
		// them as well; this keeps --json useful for failures, not only successes.
		if !errors.Is(err, errSilent) {
			printErr(err)
		}
		os.Exit(1)
	}
}

func cmdVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the pagep CLI version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{
				"success": true,
				"name":    "pagep",
				"version": appversion.Current,
			}
			if flagJSON {
				return json.NewEncoder(os.Stdout).Encode(payload)
			}
			fmt.Printf("pagep %s\n", appversion.Current)
			return nil
		},
	}
}

type doctorClient interface {
	Health(context.Context) (*client.HealthResponse, error)
	Config(context.Context) (*api.ConfigResponse, error)
	AdminSession(context.Context) (*api.AdminSessionResponse, error)
	OpenAPI(context.Context) (map[string]any, error)
	ListTokens(context.Context) (*api.TokenListResponse, error)
}

type doctorCheck struct {
	Name       string `json:"name"`
	OK         bool   `json:"ok"`
	Skipped    bool   `json:"skipped,omitempty"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
	ErrorCode  string `json:"errorCode,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

type doctorReport struct {
	Success          bool          `json:"success"`
	Server           string        `json:"server"`
	Mode             string        `json:"mode,omitempty"`
	ServerVersion    string        `json:"serverVersion,omitempty"`
	TokenConfigured  bool          `json:"tokenConfigured"`
	RequireAdmin     bool          `json:"requireAdmin"`
	UploadLimits     *api.Limits   `json:"uploadLimits,omitempty"`
	Checks           []doctorCheck `json:"checks"`
	RecommendedSteps []string      `json:"recommendedSteps,omitempty"`
}

// cmdDoctor checks the endpoint and the currently configured credential without
// printing credential material. --require-admin adds an explicit admin check.
func cmdDoctor() *cobra.Command {
	var requireAdmin bool
	c := &cobra.Command{
		Use:   "doctor",
		Short: "Check PagePilot service, upload limits, and credential readiness",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := buildClient()
			ctx, cancel := withSignalCancel()
			defer cancel()
			report := runDoctor(ctx, cl, strings.TrimRight(flagServer, "/"), strings.TrimSpace(flagToken) != "", requireAdmin)
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(report)
			} else {
				printDoctorReport(report)
			}
			if !report.Success {
				return errSilent
			}
			return nil
		},
	}
	c.Flags().BoolVar(&requireAdmin, "require-admin", false, "fail unless the configured token is an administrator token")
	return c
}

func runDoctor(ctx context.Context, cl doctorClient, server string, tokenConfigured, requireAdmin bool) doctorReport {
	report := doctorReport{
		Success:         true,
		Server:          server,
		TokenConfigured: tokenConfigured,
		RequireAdmin:    requireAdmin,
		Checks:          make([]doctorCheck, 0, 6),
	}
	appendFailure := func(name string, err error, required bool) {
		check := doctorCheck{Name: name}
		var apiErr *client.APIError
		if errors.As(err, &apiErr) {
			check.HTTPStatus = apiErr.Status
			if apiErr.Body != nil {
				check.ErrorCode = string(apiErr.Body.ErrorCode)
				check.Detail = apiErr.Body.Detail
			}
		}
		if check.Detail == "" {
			check.Detail = err.Error()
		}
		report.Checks = append(report.Checks, check)
		if required {
			report.Success = false
		}
	}

	health, err := cl.Health(ctx)
	if err != nil {
		appendFailure("health", err, true)
	} else if health == nil {
		appendFailure("health", errors.New("health endpoint returned an empty response"), true)
	} else if !health.Success {
		report.Checks = append(report.Checks, doctorCheck{Name: "health", Detail: "health endpoint did not report success"})
		report.Success = false
	} else {
		report.Checks = append(report.Checks, doctorCheck{Name: "health", OK: true, HTTPStatus: 200, Detail: health.Status})
	}

	config, err := cl.Config(ctx)
	if err != nil {
		appendFailure("config", err, true)
	} else if config == nil {
		appendFailure("config", errors.New("config endpoint returned an empty response"), true)
	} else if !config.Success {
		report.Checks = append(report.Checks, doctorCheck{Name: "config", Detail: "config endpoint did not report success"})
		report.Success = false
	} else {
		report.Mode = config.Mode
		report.ServerVersion = config.Version
		limits := config.Limits
		report.UploadLimits = &limits
		report.Checks = append(report.Checks, doctorCheck{Name: "config", OK: true, HTTPStatus: 200})
	}

	openAPI, err := cl.OpenAPI(ctx)
	if err != nil {
		appendFailure("openapi", err, true)
	} else if strings.TrimSpace(asString(openAPI["openapi"])) == "" {
		report.Checks = append(report.Checks, doctorCheck{Name: "openapi", Detail: "OpenAPI version field is missing"})
		report.Success = false
	} else {
		report.Checks = append(report.Checks, doctorCheck{Name: "openapi", OK: true, HTTPStatus: 200, Detail: asString(openAPI["openapi"])})
	}

	if tokenConfigured {
		if _, err := cl.ListTokens(ctx); err != nil {
			appendFailure("credential", err, true)
		} else {
			report.Checks = append(report.Checks, doctorCheck{Name: "credential", OK: true, HTTPStatus: 200, Detail: "registered user token"})
		}
	} else {
		report.Checks = append(report.Checks, doctorCheck{Name: "credential", OK: true, Skipped: true, Detail: "no bearer token configured; anonymous deployment quota may apply"})
		// GET /api/session creates persistent state when no session is supplied.
		// Doctor must remain read-only; anonymous deployment creates its session on demand.
		report.Checks = append(report.Checks, doctorCheck{Name: "anonymous_session", OK: true, Skipped: true, Detail: "anonymous session is created on first deployment; doctor does not create persistent sessions"})
	}

	if requireAdmin {
		if !tokenConfigured {
			report.Checks = append(report.Checks, doctorCheck{Name: "admin_session", Detail: "no bearer token configured"})
			report.Success = false
		} else if session, err := cl.AdminSession(ctx); err != nil {
			appendFailure("admin_session", err, true)
		} else if session == nil {
			appendFailure("admin_session", errors.New("admin session endpoint returned an empty response"), true)
		} else if !session.Success || !session.IsAdmin {
			report.Checks = append(report.Checks, doctorCheck{Name: "admin_session", Detail: "configured token is not an administrator token"})
			report.Success = false
		} else {
			report.Checks = append(report.Checks, doctorCheck{Name: "admin_session", OK: true, HTTPStatus: 200, Detail: session.Username})
		}
	}

	if !report.Success {
		report.RecommendedSteps = append(report.RecommendedSteps, "Check the server URL and reverse-proxy route, then run pagep doctor again.")
		if tokenConfigured {
			report.RecommendedSteps = append(report.RecommendedSteps, "Verify that the saved token is active and belongs to the intended PagePilot user.")
		} else {
			report.RecommendedSteps = append(report.RecommendedSteps, "Create or save a user token for persistent publishing: pagep token create <label> --save.")
		}
	}
	return report
}

func printDoctorReport(report doctorReport) {
	status := "READY"
	if !report.Success {
		status = "NEEDS ATTENTION"
	}
	fmt.Printf("PagePilot doctor: %s\n", status)
	fmt.Printf("  Server: %s\n", report.Server)
	if report.Mode != "" {
		fmt.Printf("  Mode:   %s", report.Mode)
		if report.ServerVersion != "" {
			fmt.Printf(" (server %s)", report.ServerVersion)
		}
		fmt.Println()
	}
	if report.UploadLimits != nil {
		fmt.Printf("  Limits: %d bytes/file, %d bytes/site, %d files/site\n", report.UploadLimits.MaxSingleFileBytes, report.UploadLimits.MaxSiteTotalBytes, report.UploadLimits.MaxFilesPerSite)
	}
	for _, check := range report.Checks {
		marker := "FAIL"
		if check.OK {
			marker = "OK"
		}
		if check.Skipped {
			marker = "INFO"
		}
		fmt.Printf("  [%s] %s", marker, check.Name)
		if check.Detail != "" {
			fmt.Printf(": %s", check.Detail)
		}
		if check.ErrorCode != "" {
			fmt.Printf(" (%s)", check.ErrorCode)
		}
		fmt.Println()
	}
	for _, step := range report.RecommendedSteps {
		fmt.Printf("  Next: %s\n", step)
	}
}

// ===== 输出工具 =====

type cliErrorPayload struct {
	Success           bool   `json:"success"`
	HTTPStatus        int    `json:"httpStatus,omitempty"`
	ErrorCode         string `json:"errorCode"`
	Stage             string `json:"stage,omitempty"`
	Detail            string `json:"detail"`
	Hint              string `json:"hint,omitempty"`
	RetryAfterSeconds *int   `json:"retryAfterSeconds,omitempty"`
	RequestID         string `json:"requestId,omitempty"`
}

func jsonErrorPayload(err error) cliErrorPayload {
	payload := cliErrorPayload{
		Success:   false,
		ErrorCode: "CLI_ERROR",
		Detail:    "unknown error",
	}
	if err == nil {
		return payload
	}
	payload.Detail = err.Error()
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) {
		return payload
	}
	payload.HTTPStatus = apiErr.Status
	if apiErr.Body == nil {
		payload.ErrorCode = "HTTP_ERROR"
		return payload
	}
	payload.ErrorCode = string(apiErr.Body.ErrorCode)
	payload.Stage = apiErr.Body.Stage
	payload.Detail = apiErr.Body.Detail
	payload.Hint = apiErr.Body.Hint
	payload.RetryAfterSeconds = apiErr.Body.RetryAfterSeconds
	payload.RequestID = apiErr.Body.RequestID
	return payload
}

func printErr(err error) {
	if flagJSON {
		// Keep machine-readable failures on stdout, alongside successful JSON
		// responses. Human-readable diagnostics remain on stderr below.
		_ = json.NewEncoder(os.Stdout).Encode(jsonErrorPayload(err))
		return
	}
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && apiErr.Body != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", apiErr.Body.ErrorCode)
		fmt.Fprintf(os.Stderr, "  detail: %s\n", apiErr.Body.Detail)
		if apiErr.Body.Hint != "" {
			fmt.Fprintf(os.Stderr, "  hint:   %s\n", apiErr.Body.Hint)
		}
		if apiErr.Body.RetryAfterSeconds != nil {
			fmt.Fprintf(os.Stderr, "  retry:  %ds\n", *apiErr.Body.RetryAfterSeconds)
		}
		if apiErr.Body.RequestID != "" {
			fmt.Fprintf(os.Stderr, "  request: %s\n", apiErr.Body.RequestID)
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
	if r.TemplateSourceCode != "" {
		fmt.Printf("  Template:    %s v%d\n", r.TemplateSourceCode, r.TemplateSourceVersion)
	}
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
	info, err := os.Lstat(source)
	if err != nil {
		return nil, "", fmt.Errorf("stat source: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, "", fmt.Errorf("refusing symbolic-link source: %s", source)
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
	readmeEntry := ""
	pageEntry := ""
	entries := []string{}
	err = filepath.Walk(source, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symbolic link in source directory: %s", p)
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
		// 默认主入口优先级：index.html > README.md > 第一个 HTML/Markdown
		lowerRel := strings.ToLower(rel)
		if lowerRel == "index.html" || lowerRel == "index.htm" {
			mainEntry = rel
		} else if (lowerRel == "readme.md" || lowerRel == "readme.markdown") && readmeEntry == "" {
			readmeEntry = rel
		} else if (strings.HasSuffix(lowerRel, ".html") || strings.HasSuffix(lowerRel, ".htm") || strings.HasSuffix(lowerRel, ".md") || strings.HasSuffix(lowerRel, ".markdown")) && pageEntry == "" {
			pageEntry = rel
		}
	}
	if mainEntry == "" {
		mainEntry = readmeEntry
	}
	if mainEntry == "" {
		mainEntry = pageEntry
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

type multipartSource struct {
	Path    string
	Name    string
	Cleanup func()
}

func prepareMultipartSource(source string) (multipartSource, error) {
	info, err := os.Lstat(source)
	if err != nil {
		return multipartSource{}, fmt.Errorf("stat source: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return multipartSource{}, fmt.Errorf("refusing symbolic-link source: %s", source)
	}
	if !info.IsDir() {
		return multipartSource{Path: source, Name: filepath.Base(source), Cleanup: func() {}}, nil
	}
	tmp, err := os.CreateTemp("", "pagepilot-*.zip")
	if err != nil {
		return multipartSource{}, fmt.Errorf("create temp zip: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	zipWriter := zip.NewWriter(tmp)
	err = filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symbolic link in source directory: %s", path)
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		rel = strings.ReplaceAll(filepath.ToSlash(rel), "\\", "/")
		if !safePreflightPath(rel) {
			return fmt.Errorf("refusing unsafe path in source directory: %s", rel)
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = rel
		header.Method = zip.Deflate
		part, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(part, file)
		return err
	})
	closeErr := zipWriter.Close()
	fileCloseErr := tmp.Close()
	if err != nil {
		cleanup()
		return multipartSource{}, fmt.Errorf("zip directory: %w", err)
	}
	if closeErr != nil {
		cleanup()
		return multipartSource{}, fmt.Errorf("close zip: %w", closeErr)
	}
	if fileCloseErr != nil {
		cleanup()
		return multipartSource{}, fmt.Errorf("close temp zip: %w", fileCloseErr)
	}
	return multipartSource{Path: tmpPath, Name: filepath.Base(source) + ".zip", Cleanup: cleanup}, nil
}

func normalizeFilenameHint(raw string) (string, error) {
	value := strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/")
	if value == "" {
		return "", nil
	}
	if !safePreflightPath(value) {
		return "", fmt.Errorf("--filename %q is not a safe relative path", value)
	}
	return value, nil
}

func parseVersionArg(raw string) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value <= 0 {
		if err == nil {
			err = fmt.Errorf("version must be positive")
		}
		return 0, fmt.Errorf("invalid version %q: %w", raw, err)
	}
	return value, nil
}

func sourceEntryHint(source string) (string, error) {
	info, err := os.Stat(source)
	if err != nil {
		return "", fmt.Errorf("stat source: %w", err)
	}
	if !info.IsDir() {
		if strings.EqualFold(filepath.Ext(source), ".zip") {
			reader, err := zip.OpenReader(source)
			if err != nil {
				return "index.html", nil
			}
			defer reader.Close()
			paths := make([]string, 0, len(reader.File))
			for _, file := range reader.File {
				if file.FileInfo().IsDir() {
					continue
				}
				paths = append(paths, filepath.ToSlash(file.Name))
			}
			return chooseMainEntry(paths), nil
		}
		return filepath.Base(source), nil
	}
	paths := []string{}
	if err := filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	}); err != nil {
		return "", fmt.Errorf("inspect source entries: %w", err)
	}
	return chooseMainEntry(paths), nil
}

func chooseMainEntry(paths []string) string {
	lowered := map[string]string{}
	for _, path := range paths {
		lowered[strings.ToLower(path)] = path
	}
	for _, preferred := range []string{"index.html", "index.htm", "readme.md", "readme.markdown"} {
		if path := lowered[preferred]; path != "" {
			return path
		}
	}
	for _, path := range paths {
		lower := strings.ToLower(path)
		if strings.HasSuffix(lower, ".html") || strings.HasSuffix(lower, ".htm") ||
			strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown") {
			return path
		}
	}
	return "index.html"
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
		description           string
		title                 string
		customCode            string
		filename              string
		accessPass            string
		category              string
		templateSourceCode    string
		templateSourceVersion int64
	)
	c := &cobra.Command{
		Use:   "deploy <source>",
		Short: "Deploy a static site from HTML, Markdown, ZIP, or a directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			normalizedFilename, err := normalizeFilenameHint(filename)
			if err != nil {
				return err
			}
			filename = normalizedFilename
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
			source, err := prepareMultipartSource(args[0])
			if err != nil {
				return err
			}
			defer source.Cleanup()
			req := client.MultipartDeployRequest{
				SourcePath:            source.Path,
				UploadName:            source.Name,
				Description:           description,
				Title:                 title,
				Filename:              filename,
				Source:                "cli",
				AccessPassword:        accessPass,
				Category:              category,
				Visibility:            "",
				TemplateSourceCode:    templateSourceCode,
				TemplateSourceVersion: templateSourceVersion,
			}
			if customCode != "" {
				req.EnableCustomCode = true
				req.CustomCode = customCode
			}
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().DeployMultipart(ctx, req)
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
	c.Flags().StringVarP(&filename, "filename", "f", "", "optional explicit entry path; omit for automatic server detection")
	c.Flags().StringVar(&accessPass, "access-password", "", "optional visit password for a new user-owned site")
	c.Flags().StringVar(&category, "category", "", "marketplace category slug, e.g. landing/dashboard/docs/tool/game/screen")
	c.Flags().StringVar(&templateSourceCode, "template-source-code", "", "record that this deploy is derived from a marketplace site code")
	c.Flags().Int64Var(&templateSourceVersion, "template-source-version", 0, "record the source marketplace version number")
	_ = c.MarkFlagRequired("description")
	return c
}

// ===== 子命令：append =====

func cmdAppend() *cobra.Command {
	var (
		description           string
		title                 string
		templateSourceCode    string
		templateSourceVersion int64
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
			source, err := prepareMultipartSource(args[1])
			if err != nil {
				return err
			}
			defer source.Cleanup()
			req := client.MultipartDeployRequest{
				SourcePath:            source.Path,
				UploadName:            source.Name,
				Description:           description,
				Title:                 title,
				EnableCustomCode:      true,
				CustomCode:            args[0],
				CreateVersion:         true,
				Source:                "cli",
				TemplateSourceCode:    templateSourceCode,
				TemplateSourceVersion: templateSourceVersion,
			}
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().DeployMultipart(ctx, req)
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
	c.Flags().StringVar(&templateSourceCode, "template-source-code", "", "record that this version is derived from a marketplace site code")
	c.Flags().Int64Var(&templateSourceVersion, "template-source-version", 0, "record the source marketplace version number")
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
				outFile := ""
				if strings.Contains(ct, "zip") {
					outFile = filepath.Join(output, fmt.Sprintf("%s.zip", args[0]))
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
				} else {
					// 单 HTML，直接保存
					outFile = filepath.Join(output, "index.html")
					f, err := os.Create(outFile)
					if err != nil {
						return err
					}
					defer f.Close()
					if _, err := io.Copy(f, r); err != nil {
						return err
					}
				}
				if flagJSON {
					payload := map[string]any{
						"success":     true,
						"code":        args[0],
						"output":      outFile,
						"contentType": ct,
					}
					if versionPtr != nil {
						payload["version"] = *versionPtr
					}
					return json.NewEncoder(os.Stdout).Encode(payload)
				}
				fmt.Printf("Downloaded %s\n", outFile)
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
			version, err := parseVersionArg(args[1])
			if err != nil {
				return err
			}
			source, err := prepareMultipartSource(args[2])
			if err != nil {
				return err
			}
			defer source.Cleanup()
			req := client.MultipartOverwriteRequest{
				SourcePath:  source.Path,
				UploadName:  source.Name,
				Description: description,
			}
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().OverwriteMultipart(ctx, args[0], version, req)
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
			version, err := parseVersionArg(args[1])
			if err != nil {
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
			version, err := parseVersionArg(args[1])
			if err != nil {
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
			version, err := parseVersionArg(args[1])
			if err != nil {
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
			version, err := parseVersionArg(args[1])
			if err != nil {
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
			version, err := parseVersionArg(args[1])
			if err != nil {
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
	var saveCreated bool
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
			if saveCreated {
				cfg := loadConfig()
				cfg["server"] = strings.TrimRight(flagServer, "/")
				cfg["token"] = resp.Token
				if err := saveConfig(cfg); err != nil {
					return err
				}
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
			if saveCreated {
				fmt.Printf("  Saved:  %s\n", configPath())
			}
			return nil
		},
	}
	createC.Flags().BoolVar(&isAdmin, "admin", false, "create an admin token")
	createC.Flags().StringVar(&ttl, "ttl", "", "temporary token lifetime, for example 24h or 30m")
	createC.Flags().StringVar(&expiresAt, "expires-at", "", "absolute expiry time in RFC3339 format")
	createC.Flags().BoolVar(&saveCreated, "save", false, "save returned plaintext token into ~/.pagep/config.json")

	saveC := &cobra.Command{
		Use:   "save <token>",
		Short: "Save an existing plaintext token into ~/.pagep/config.json",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			buildClient()
			cfg := loadConfig()
			cfg["server"] = strings.TrimRight(flagServer, "/")
			cfg["token"] = strings.TrimSpace(args[0])
			if cfg["token"] == "" {
				return fmt.Errorf("token value is required")
			}
			if err := saveConfig(cfg); err != nil {
				return err
			}
			if flagJSON {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success":    true,
					"key":        "token",
					"server":     cfg["server"],
					"tokenSaved": true,
					"configFile": configPath(),
				})
			}
			fmt.Printf("Saved token to %s\n", configPath())
			return nil
		},
	}

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

	root.AddCommand(createC, saveC, listC, revokeC)
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

	siteDetailC := &cobra.Command{
		Use:   "site-detail <code>",
		Short: "Show admin site detail, bundle metadata, file tree, versions, and reuse hints",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().AdminSiteDetail(ctx, args[0])
			if err != nil {
				printErr(err)
				return errSilent
			}
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(resp)
				return nil
			}
			printAdminSiteDetail(resp)
			return nil
		},
	}

	var auditQuery client.AuditLogQuery
	auditC := &cobra.Command{
		Use:   "audit-logs",
		Short: "Query admin audit logs with filters",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().ListAuditLogs(ctx, auditQuery)
			if err != nil {
				printErr(err)
				return errSilent
			}
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(resp)
				return nil
			}
			printAuditLogList(resp)
			return nil
		},
	}
	auditC.Flags().StringVar(&auditQuery.ActorType, "actor-type", "", "actor type filter")
	auditC.Flags().StringVar(&auditQuery.ActorID, "actor-id", "", "actor id filter")
	auditC.Flags().StringVar(&auditQuery.ActorRole, "actor-role", "", "actor role filter")
	auditC.Flags().StringVar(&auditQuery.Action, "action", "", "action filter, e.g. site.pin")
	auditC.Flags().StringVar(&auditQuery.Result, "result", "", "result filter: success / failed")
	auditC.Flags().StringVar(&auditQuery.SiteCode, "site-code", "", "site code filter")
	auditC.Flags().StringVar(&auditQuery.TargetType, "target-type", "", "target type filter")
	auditC.Flags().StringVar(&auditQuery.TargetID, "target-id", "", "target id filter")
	auditC.Flags().StringVarP(&auditQuery.Query, "query", "q", "", "keyword search over audit fields and detail JSON")
	auditC.Flags().StringVar(&auditQuery.Since, "since", "", "RFC3339 lower bound, e.g. 2026-07-06T00:00:00Z")
	auditC.Flags().StringVar(&auditQuery.Until, "until", "", "RFC3339 upper bound, e.g. 2026-07-07T00:00:00Z")
	auditC.Flags().IntVar(&auditQuery.Page, "page", 1, "page number")
	auditC.Flags().IntVar(&auditQuery.PageSize, "page-size", 50, "page size, max 200")

	var reusePolicy string
	var sourceDownloadPolicy string
	reuseC := &cobra.Command{
		Use:   "reuse-policy <code>",
		Short: "Set source download and template reuse policy for a site",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !validReusePolicy(reusePolicy) {
				return fmt.Errorf("--reuse must be auto, allow, or deny")
			}
			if !validReusePolicy(sourceDownloadPolicy) {
				return fmt.Errorf("--source-download must be auto, allow, or deny")
			}
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().SetSiteReusePolicy(ctx, args[0], reusePolicy, sourceDownloadPolicy)
			if err != nil {
				printErr(err)
				return errSilent
			}
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(resp)
				return nil
			}
			site, _ := resp["site"].(map[string]any)
			fmt.Printf("Updated %s reusePolicy=%s sourceDownloadPolicy=%s\n",
				args[0], asString(site["reusePolicy"]), asString(site["sourceDownloadPolicy"]))
			return nil
		},
	}
	reuseC.Flags().StringVar(&reusePolicy, "reuse", "auto", "template reuse policy: auto / allow / deny")
	reuseC.Flags().StringVar(&sourceDownloadPolicy, "source-download", "auto", "source download policy: auto / allow / deny")

	var securityMode string
	securityC := &cobra.Command{
		Use:   "security-mode <code>",
		Short: "Set site runtime security mode",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !validSiteSecurityMode(securityMode) {
				return fmt.Errorf("--mode must be auto, strict, compatible, or trusted")
			}
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().SetSiteSecurityMode(ctx, args[0], securityMode)
			if err != nil {
				printErr(err)
				return errSilent
			}
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(resp)
				return nil
			}
			site, _ := resp["site"].(map[string]any)
			fmt.Printf("Updated %s securityMode=%s\n", args[0], asString(site["securityMode"]))
			return nil
		},
	}
	securityC.Flags().StringVar(&securityMode, "mode", "auto", "security mode: auto / strict / compatible / trusted")

	root.AddCommand(pinC, siteDetailC, auditC, reuseC, securityC)
	return root
}

func validReusePolicy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto", "allow", "deny":
		return true
	default:
		return false
	}
}

func validSiteSecurityMode(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto", "strict", "compatible", "trusted":
		return true
	default:
		return false
	}
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
		Short: "Manage ~/.pagep/config.json",
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
			if flagJSON {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success":    true,
					"key":        args[0],
					"server":     cfg["server"],
					"tokenSaved": strings.TrimSpace(cfg["token"]) != "",
					"configFile": configPath(),
				})
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
				if flagJSON {
					return json.NewEncoder(os.Stdout).Encode(map[string]any{
						"success":    true,
						"key":        args[0],
						"value":      "",
						"configured": false,
						"configFile": configPath(),
					})
				}
				fmt.Println("(unset)")
				return nil
			}
			if args[0] == "token" {
				v = maskedToken(v)
			}
			if flagJSON {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success":    true,
					"key":        args[0],
					"value":      v,
					"configured": true,
					"configFile": configPath(),
				})
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
				if flagJSON {
					return json.NewEncoder(os.Stdout).Encode(map[string]any{
						"success":    true,
						"configFile": configPath(),
						"config":     map[string]string{},
					})
				}
				fmt.Println("(empty)")
				return nil
			}
			if flagJSON {
				masked := make(map[string]string, len(cfg))
				for k, v := range cfg {
					if k == "token" {
						v = maskedToken(v)
					}
					masked[k] = v
				}
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success":    true,
					"configFile": configPath(),
					"config":     masked,
				})
			}
			for k, v := range cfg {
				if k == "token" {
					v = maskedToken(v)
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
		mCategory string
		mKind     string
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
			resp, err := buildClient().SearchMarketplaceWithFilters(ctx, q, mSort, mCategory, mKind, mPage, mPageSize)
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
	searchC.Flags().StringVar(&mCategory, "category", "", "marketplace category slug, e.g. landing/dashboard/docs/tool/game/screen")
	searchC.Flags().StringVar(&mKind, "kind", "", "derived filter: html / md / protected / featured / mine")
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

	categoriesC := &cobra.Command{
		Use:   "categories",
		Short: "List marketplace categories",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := withSignalCancel()
			defer cancel()
			resp, err := buildClient().MarketCategories(ctx)
			if err != nil {
				printErr(err)
				return errSilent
			}
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(resp)
				return nil
			}
			items, _ := resp["categories"].([]any)
			for _, it := range items {
				m, _ := it.(map[string]any)
				fmt.Printf("%-16s %-18s %s\n", asString(m["slug"]), asString(m["label"]), asString(m["note"]))
			}
			return nil
		},
	}

	root.AddCommand(searchC, showC, categoriesC)
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
	if bundle, ok := m["bundle"].(map[string]any); ok {
		fmt.Println("\nBundle")
		for _, k := range []string{"kindLabel", "kind", "mainEntry", "securityMode", "siteSecurityMode", "effectiveSecurityMode", "fileCount", "totalSize", "entryNote"} {
			if v, exists := bundle[k]; exists && v != nil {
				fmt.Printf("  %-20s %v\n", k+":", v)
			}
		}
	}
	if files, ok := m["files"].([]any); ok && len(files) > 0 {
		fmt.Println("\nFiles")
		limit := len(files)
		if limit > 12 {
			limit = 12
		}
		for _, item := range files[:limit] {
			file, _ := item.(map[string]any)
			fmt.Printf("  %-42s %8d\n", asString(file["path"]), toInt64(file["size"]))
		}
		if len(files) > limit {
			fmt.Printf("  ... and %d more files\n", len(files)-limit)
		}
	}
	if reuse, ok := m["reuse"].(map[string]any); ok {
		fmt.Println("\nReuse")
		for _, k := range []string{"allowReuse", "allowDownload", "policyNote", "downloadUrl", "detailUrl", "cli", "agentPrompt"} {
			if v, exists := reuse[k]; exists && v != nil && fmt.Sprint(v) != "" {
				fmt.Printf("  %-20s %v\n", k+":", v)
			}
		}
		if mcp, exists := reuse["mcp"]; exists && mcp != nil {
			b, _ := json.MarshalIndent(mcp, "  ", "  ")
			fmt.Printf("  %-20s %s\n", "mcp:", strings.TrimSpace(string(b)))
		}
	}
}

func printAdminSiteDetail(resp map[string]any) {
	site, _ := resp["site"].(map[string]any)
	fmt.Println("Site")
	for _, k := range []string{"code", "publicId", "ownerUsername", "ownerTokenId", "status", "visibility", "accessProtected", "reusePolicy", "sourceDownloadPolicy", "securityMode", "currentVersion", "versionCount", "totalSize", "createdAt", "lastVersionAt"} {
		if v, exists := site[k]; exists && v != nil && fmt.Sprint(v) != "" {
			fmt.Printf("  %-22s %v\n", k+":", v)
		}
	}
	if bundle, ok := resp["bundle"].(map[string]any); ok {
		fmt.Println("\nBundle")
		for _, k := range []string{"kindLabel", "kind", "mainEntry", "root", "securityMode", "siteSecurityMode", "effectiveSecurityMode", "fileCount", "totalSize", "entryNote"} {
			if v, exists := bundle[k]; exists && v != nil && fmt.Sprint(v) != "" {
				fmt.Printf("  %-22s %v\n", k+":", v)
			}
		}
	}
	if files, ok := resp["files"].([]any); ok && len(files) > 0 {
		fmt.Println("\nFiles")
		limit := len(files)
		if limit > 40 {
			limit = 40
		}
		for _, item := range files[:limit] {
			file, _ := item.(map[string]any)
			fmt.Printf("  %-48s %8d %s\n", asString(file["path"]), toInt64(file["size"]), binaryFlag(file["isBinary"]))
		}
		if len(files) > limit {
			fmt.Printf("  ... and %d more files\n", len(files)-limit)
		}
	}
	if reuse, ok := resp["reuse"].(map[string]any); ok {
		fmt.Println("\nReuse")
		for _, k := range []string{"allowReuse", "allowDownload", "policyNote", "downloadUrl", "detailUrl", "cli", "agentPrompt"} {
			if v, exists := reuse[k]; exists && v != nil && fmt.Sprint(v) != "" {
				fmt.Printf("  %-22s %v\n", k+":", v)
			}
		}
		if mcp, exists := reuse["mcp"]; exists && mcp != nil {
			b, _ := json.MarshalIndent(mcp, "  ", "  ")
			fmt.Printf("  %-22s %s\n", "mcp:", strings.TrimSpace(string(b)))
		}
	}
	if versions, ok := resp["versions"].([]any); ok && len(versions) > 0 {
		fmt.Println("\nVersions")
		limit := len(versions)
		if limit > 12 {
			limit = 12
		}
		fmt.Printf("  %-8s %-8s %-8s %-20s %s\n", "VERSION", "STATUS", "LOCKED", "CREATED", "TITLE")
		for _, item := range versions[:limit] {
			version, _ := item.(map[string]any)
			fmt.Printf("  %-8d %-8s %-8v %-20s %s\n",
				toInt64(version["versionNumber"]),
				asString(version["status"]),
				version["isLocked"],
				prettyTime(version["createdAt"]),
				shortString(asString(version["title"]), 42),
			)
		}
		if len(versions) > limit {
			fmt.Printf("  ... and %d more versions\n", len(versions)-limit)
		}
	}
}

func printAuditLogList(resp map[string]any) {
	logs, _ := resp["logs"].([]any)
	total := toInt64(resp["total"])
	page := toInt64(resp["page"])
	pageSize := toInt64(resp["pageSize"])
	fmt.Printf("Audit logs page %d (total %d, page size %d)\n\n", page, total, pageSize)
	if len(logs) == 0 {
		fmt.Println("No audit logs found.")
		return
	}
	fmt.Printf("%-16s %-24s %-8s %-18s %-10s %-12s %-22s %s\n", "TIME", "ACTION", "RESULT", "ACTOR", "ROLE", "SITE", "TARGET", "IP")
	for _, item := range logs {
		log, _ := item.(map[string]any)
		target := strings.Trim(strings.TrimSpace(asString(log["targetType"])+":"+asString(log["targetId"])), ":")
		actor := asString(log["actorId"])
		if actor == "" {
			actor = asString(log["actorType"])
		}
		fmt.Printf("%-16s %-24s %-8s %-18s %-10s %-12s %-22s %s\n",
			prettyTime(log["createdAt"]),
			shortString(asString(log["action"]), 24),
			shortString(asString(log["result"]), 8),
			shortString(actor, 18),
			shortString(asString(log["actorRole"]), 10),
			shortString(asString(log["siteCode"]), 12),
			shortString(target, 22),
			shortString(asString(log["ip"]), 20),
		)
	}
	fmt.Println("\nDetails")
	for _, item := range logs {
		log, _ := item.(map[string]any)
		id := toInt64(log["id"])
		action := asString(log["action"])
		if action == "" {
			action = "-"
		}
		fmt.Printf("- #%d %s %s\n", id, action, prettyTime(log["createdAt"]))
		fmt.Printf("  User-Agent: %s\n", fallbackString(asString(log["userAgent"]), "-"))
		fmt.Printf("  Detail: %s\n", formatAuditLogDetail(log["detail"]))
	}
}

func formatAuditLogDetail(detail any) string {
	if detail == nil {
		return "{}"
	}
	if s, ok := detail.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" {
			return "{}"
		}
		var parsed any
		if json.Unmarshal([]byte(s), &parsed) == nil {
			detail = parsed
		} else {
			return s
		}
	}
	b, err := json.Marshal(detail)
	if err != nil {
		return fmt.Sprint(detail)
	}
	return string(b)
}

func fallbackString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func binaryFlag(v any) string {
	if b, ok := v.(bool); ok && b {
		return "bin"
	}
	return ""
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

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func shortString(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= max {
		return string(runes)
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
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
// 保留向后兼容：所有 RunE 可以直接返回 printErr 包装后的 errSilent，
// 也可以返回 nil（cobra 已 SilenceErrors）— 推荐后者。
var errSilent = fmt.Errorf("__silent__")
