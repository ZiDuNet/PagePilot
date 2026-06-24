// Package deploy 实现部署核心逻辑：把 DeployRequest 落到磁盘 + 元数据库。
package deploy

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/yourorg/hostctl/internal/api"
	"github.com/yourorg/hostctl/internal/auth"
	"github.com/yourorg/hostctl/internal/config"
	"github.com/yourorg/hostctl/internal/nanoid"
	"github.com/yourorg/hostctl/internal/store"
)

// 默认主入口文件名
const defaultMainEntry = "index.html"

var (
	// customCode 白名单（严格对齐 OpenAPI）：
	//   ^[a-z0-9](?:[a-z0-9-]{2,30}[a-z0-9])?$
	// 长度 3-32，必须小写字母/数字开头结尾，中间可有 -
	customCodeRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{2,30}[a-z0-9])?$`)

	// filename 必须 .html 或 .htm 结尾（对齐 OpenAPI `\.html?$`）
	htmlFilenameRe = regexp.MustCompile(`(?i)\.html?$`)

	// 自动生成的短码（nanoid 默认 6 字符 [0-9a-zA-Z]）也要满足 OpenAPI pattern。
	// nanoid 默认含大写字母，但 OpenAPI 仅允许小写。我们把生成的码全部 lowercase
	// 然后再正则校验一次，不合法重试。
	autoCodeRe = customCodeRe
)

// Deployer 持有部署所需的依赖。
type Deployer struct {
	cfg      config.Config
	store    store.Store
	cooldown *Cooldown
}

// New 构造一个 Deployer。
func New(cfg config.Config, s store.Store) *Deployer {
	return &Deployer{
		cfg:      cfg,
		store:    s,
		cooldown: NewCooldown(time.Duration(cfg.CooldownSeconds) * time.Second),
	}
}

// resolvedFile 是经过校验 + 解码的文件。
type resolvedFile struct {
	Path     string // 相对路径，已规范化
	Bytes    []byte
	IsBinary bool
	Sha256   string
}

// Deploy 执行一次部署。
// 支持两种模式：
//   - 单 HTML：req.Content（filename 必填，必须 .html/.htm）
//   - 多文件：req.Files（filename 必填，必须 .html/.htm，作为主入口）
//
// 严格对齐项目 OpenAPI 3.1。
// ownerTokenID 用于按 token 维度限流；clientIP 用于按 IP 维度限流。
func (d *Deployer) Deploy(ctx context.Context, req api.DeployRequest, ownerTokenID, clientIP string) (*api.DeployResponse, *api.APIError) {
	// 0. 冷却检查（必须最先；任何字段错误都不应该消耗冷却）
	if ok, retry := d.cooldown.Check(ownerTokenID, clientIP); !ok {
		secs := int(retry/time.Second) + 1
		return nil, api.NewError(api.CodeRateLimited, "cooldown",
			fmt.Sprintf("global deploy cooldown active; retry in ~%ds", secs)).
			WithRetryAfter(secs).
			WithHint("Slow down. Use createVersion=true to iterate on an existing code instead of creating new ones.")
	}

	// 1. filename 必填且必须 .html/.htm（对齐 OpenAPI `\.html?$`）
	filename := strings.TrimSpace(req.Filename)
	if filename == "" {
		return nil, api.NewError(api.CodeInvalidInput, "validate",
			"filename is required").WithHint("filename must be the main HTML entry, e.g. 'index.html'.")
	}
	if !htmlFilenameRe.MatchString(filename) {
		return nil, api.NewError(api.CodeInvalidInput, "validate",
			fmt.Sprintf("filename %q must end with .html or .htm", filename))
	}
	// filename 同时也要过路径白名单（防 ../foo.html）
	if apiErr := validateFilePath(filename); apiErr != nil {
		return nil, apiErr
	}
	mainEntry := filename

	// 2. description 必填 + 长度
	desc := strings.TrimSpace(req.Description)
	if desc == "" {
		return nil, api.NewError(api.CodeInvalidDescription, "validate",
			"description is required").WithHint("Provide one concise sentence, max 240 characters.")
	}
	if len(desc) > 240 {
		return nil, api.NewError(api.CodeInvalidDescription, "validate",
			"description must be at most 240 characters")
	}
	visibility, updateVisibility, visibilityErr := normalizeVisibility(req.Visibility)
	if visibilityErr != nil {
		return nil, visibilityErr
	}

	// 3. 解析 + 校验内容（content 或 files）
	rfiles, apiErr := d.resolveContent(req, mainEntry)
	if apiErr != nil {
		return nil, apiErr
	}
	if apiErr := validateEntrypointHTML(rfiles, mainEntry); apiErr != nil {
		return nil, apiErr
	}

	// 4. 决定 code
	code, isCustom, apiErr := d.decideCode(ctx, req)
	if apiErr != nil {
		return nil, apiErr
	}

	// 5. 决定 version_number + 是否新建 site
	isNewSite, versionNumber, apiErr := d.decideVersion(ctx, code, isCustom, req.CreateVersion)
	if apiErr != nil {
		return nil, apiErr
	}

	// 6. 写文件到磁盘
	if !isNewSite {
		site, gerr := d.store.GetSite(ctx, code)
		if gerr != nil {
			return nil, api.NewError(api.CodeInternal, "site_owner",
				fmt.Sprintf("failed to check site owner: %v", gerr))
		}
		if ownerTokenID != "" && ownerTokenID != "dev-owner" && !strings.HasPrefix(ownerTokenID, "admin:") && site.OwnerTokenID != ownerTokenID {
			return nil, api.NewError(api.CodeForbidden, "owner",
				"you can only append versions to sites you own").
				WithHint("Use the same account, token, or anonymous session that created this site.")
		}
	}

	if err := d.writeFiles(code, versionNumber, rfiles); err != nil {
		_ = os.RemoveAll(d.versionDir(code, versionNumber))
		return nil, api.NewError(api.CodeInternal, "write_files",
			fmt.Sprintf("failed to write files: %v", err))
	}

	// 7. 计算 sha256 聚合
	aggregateSha := aggregateSha256(rfiles)
	var totalSize int64
	for _, f := range rfiles {
		totalSize += int64(len(f.Bytes))
	}

	// 8. 写元数据
	now := time.Now().UTC()
	versionID, err := newUUID()
	if err != nil {
		_ = os.RemoveAll(d.versionDir(code, versionNumber))
		return nil, api.NewError(api.CodeInternal, "uuid",
			fmt.Sprintf("failed to generate uuid: %v", err))
	}

	if isNewSite {
		accessHash := ""
		if strings.TrimSpace(req.AccessPassword) != "" {
			hash, herr := auth.HashPassword(req.AccessPassword)
			if herr != nil {
				_ = os.RemoveAll(d.versionDir(code, versionNumber))
				return nil, api.NewError(api.CodeInternal, "access_password", herr.Error())
			}
			accessHash = hash
		}
		if err := d.store.CreateSite(ctx, store.Site{
			Code:                   code,
			OwnerTokenID:           ownerTokenID,
			PrimaryVersionStrategy: string(api.StrategyLikes),
			Visibility:             visibility,
			AccessPasswordHash:     accessHash,
			CreatedAt:              now,
			Source:                 normalizeSource(req.Source),
		}); err != nil {
			_ = os.RemoveAll(d.versionDir(code, versionNumber))
			return nil, api.NewError(api.CodeInternal, "create_site",
				fmt.Sprintf("failed to create site: %v", err))
		}
	} else if updateVisibility {
		if err := d.store.SetSiteVisibility(ctx, code, visibility); err != nil {
			_ = os.RemoveAll(d.versionDir(code, versionNumber))
			return nil, api.NewError(api.CodeInternal, "site_visibility",
				fmt.Sprintf("failed to update site visibility: %v", err))
		}
	}

	if err := d.store.CreateVersion(ctx, store.Version{
		ID:            versionID,
		SiteCode:      code,
		VersionNumber: versionNumber,
		Title:         sanitizeSiteTitle(req.Title),
		Description:   desc,
		MainEntry:     mainEntry,
		TotalSize:     totalSize,
		FileCount:     len(rfiles),
		ContentSha256: aggregateSha,
		IsLocked:      false,
		Status:        "active",
		CreatedAt:     now,
	}); err != nil {
		_ = os.RemoveAll(d.versionDir(code, versionNumber))
		return nil, api.NewError(api.CodeInternal, "create_version",
			fmt.Sprintf("failed to create version: %v", err))
	}

	// 写 files 表
	fileMetas := make([]store.FileMeta, 0, len(rfiles))
	for _, f := range rfiles {
		fileMetas = append(fileMetas, store.FileMeta{
			SiteCode:      code,
			VersionNumber: versionNumber,
			Path:          f.Path,
			Size:          int64(len(f.Bytes)),
			Sha256:        f.Sha256,
			IsBinary:      f.IsBinary,
		})
	}
	if err := d.store.CreateFiles(ctx, fileMetas); err != nil {
		_ = os.RemoveAll(d.versionDir(code, versionNumber))
		return nil, api.NewError(api.CodeInternal, "create_files",
			fmt.Sprintf("failed to create files: %v", err))
	}

	// 9. 切 current_version + symlink
	if err := d.store.SetCurrentVersion(ctx, code, &versionNumber); err != nil {
		return nil, api.NewError(api.CodeInternal, "set_current",
			fmt.Sprintf("failed to set current version: %v", err))
	}
	if err := d.swapCurrentSymlink(code, versionNumber); err != nil {
		return nil, api.NewError(api.CodeInternal, "swap_symlink",
			fmt.Sprintf("failed to swap current symlink: %v", err))
	}

	// 10. 部署成功，消耗冷却
	retryAfter := d.cooldown.Consume(ownerTokenID, clientIP)

	// 11. 拼响应（OpenAPI 全字段）
	site, _ := d.store.GetSite(ctx, code)
	allVersions, _ := d.store.ListVersions(ctx, code)
	base := strings.TrimRight(d.cfg.PublicBaseURL, "/")
	appURLs := d.AppURLConfig()
	publicURL := appURLs.PrimaryAppURL(code, nil)
	strategy := api.StrategyLikes
	if site.PrimaryVersionStrategy == string(api.StrategyLatest) {
		strategy = api.StrategyLatest
	}
	preserveHint := ""
	agentGuideURL := ""
	if isNewSite {
		preserveHint = "Use createVersion=true with the same code to publish updates."
		agentGuideURL = fmt.Sprintf("%s/api-docs.html", base)
	}
	return &api.DeployResponse{
		Success:                true,
		ID:                     versionID,
		Code:                   code,
		URL:                    publicURL,
		DetailURL:              publicURL,
		VersionURL:             appURLs.PrimaryAppURL(code, &versionNumber),
		QRCode:                 generateQRCodeDataURL(publicURL),
		Description:            desc,
		VersionID:              versionID,
		VersionNumber:          int(versionNumber),
		CurrentVersionID:       versionID,
		PreserveHint:           preserveHint,
		AgentGuideURL:          agentGuideURL,
		PrimaryVersionStrategy: strategy,
		Visibility:             site.Visibility,
		CooldownSeconds:        d.cfg.CooldownSeconds,
		NextAvailableAt:        time.Now().UTC().Add(retryAfter),
		VersionCount:           len(allVersions),
		Size:                   totalSize,
		CreatedAt:              now.Format(time.RFC3339),
	}, nil
}

// resolveContent 把请求里的 content 或 files 解析成统一的 resolvedFile 列表。
// 同时做所有大小、路径、数量校验。
func (d *Deployer) resolveContent(req api.DeployRequest, mainEntry string) ([]resolvedFile, *api.APIError) {
	hasContent := req.Content != ""
	hasFiles := len(req.Files) > 0

	if !hasContent && !hasFiles {
		return nil, api.NewError(api.CodeInvalidInput, "validate",
			"either 'content' or 'files' must be provided").WithHint("Single HTML uses 'content'; multiple files use 'files' array.")
	}
	if hasContent && hasFiles {
		return nil, api.NewError(api.CodeInvalidInput, "validate",
			"provide 'content' or 'files', not both")
	}

	// 单 HTML 模式：包成单个 file
	if hasContent {
		if int64(len(req.Content)) > d.cfg.MaxSingleFileBytes {
			return nil, api.NewError(api.CodeContentTooLarge, "validate",
				fmt.Sprintf("content exceeds max single-file size (%d bytes)", d.cfg.MaxSingleFileBytes))
		}
		// 校验 mainEntry 合法
		if apiErr := validateFilePath(mainEntry); apiErr != nil {
			return nil, apiErr
		}
		bytes := []byte(req.Content)
		sha := sha256sum(bytes)
		return []resolvedFile{{
			Path:     mainEntry,
			Bytes:    bytes,
			IsBinary: false,
			Sha256:   sha,
		}}, nil
	}

	// 多文件模式
	if len(req.Files) > d.cfg.MaxFilesPerSite {
		return nil, api.NewError(api.CodeContentTooLarge, "validate",
			fmt.Sprintf("too many files (%d); max %d per site", len(req.Files), d.cfg.MaxFilesPerSite))
	}

	// 路径必须唯一
	seen := make(map[string]bool, len(req.Files))
	out := make([]resolvedFile, 0, len(req.Files))
	var totalSize int64

	for _, f := range req.Files {
		// path 校验
		if apiErr := validateFilePath(f.Path); apiErr != nil {
			return nil, apiErr
		}
		// 大小写敏感去重
		if seen[f.Path] {
			return nil, api.NewError(api.CodeInvalidInput, "validate",
				fmt.Sprintf("duplicate file path: %s", f.Path))
		}
		seen[f.Path] = true

		// 解码内容
		bytes, isBinary, apiErr := resolveFileContent(f)
		if apiErr != nil {
			return nil, apiErr
		}

		// 单文件大小校验
		if int64(len(bytes)) > d.cfg.MaxSingleFileBytes {
			return nil, api.NewError(api.CodeContentTooLarge, "validate",
				fmt.Sprintf("file %s exceeds max single-file size (%d bytes)", f.Path, d.cfg.MaxSingleFileBytes))
		}

		totalSize += int64(len(bytes))
		if totalSize > d.cfg.MaxSiteTotalBytes {
			return nil, api.NewError(api.CodeContentTooLarge, "validate",
				fmt.Sprintf("total size exceeds site limit (%d bytes)", d.cfg.MaxSiteTotalBytes))
		}

		out = append(out, resolvedFile{
			Path:     f.Path,
			Bytes:    bytes,
			IsBinary: isBinary,
			Sha256:   sha256sum(bytes),
		})
	}

	return out, nil
}

func validateEntrypointHTML(files []resolvedFile, mainEntry string) *api.APIError {
	for _, f := range files {
		if f.Path != mainEntry || f.IsBinary {
			continue
		}
		html := strings.ToLower(strings.TrimSpace(string(f.Bytes)))
		if len(html) < 32 {
			return api.NewError(api.CodeInvalidInput, "validate",
				"main HTML entry is too short to be a page").WithHint("Upload a real HTML file with tags such as <html>, <body>, <main>, <script>, or <style>.")
		}
		hasPageTag := strings.Contains(html, "<html") ||
			strings.Contains(html, "<body") ||
			strings.Contains(html, "<main") ||
			strings.Contains(html, "<section") ||
			strings.Contains(html, "<div") ||
			strings.Contains(html, "<script") ||
			strings.Contains(html, "<style")
		hasAngleStructure := strings.Contains(html, "<") && strings.Contains(html, ">")
		if !hasPageTag || !hasAngleStructure {
			return api.NewError(api.CodeInvalidInput, "validate",
				"main entry does not look like an HTML page").WithHint("Plain text is not deployable here. Provide a valid HTML document or generated static site.")
		}
		return nil
	}
	return api.NewError(api.CodeInvalidInput, "validate",
		fmt.Sprintf("main entry %q was not found in uploaded files", mainEntry))
}

// decideCode 决定短码。
func (d *Deployer) decideCode(ctx context.Context, req api.DeployRequest) (code string, isCustom bool, err *api.APIError) {
	if req.EnableCustomCode {
		c := strings.TrimSpace(req.CustomCode)
		if c == "" {
			return "", false, api.NewError(api.CodeInvalidCustomCode, "validate",
				"customCode is required when enableCustomCode=true")
		}
		if !customCodeRe.MatchString(c) {
			return "", false, api.NewError(api.CodeInvalidCustomCode, "validate",
				"customCode must match ^[a-z0-9](?:[a-z0-9-]{2,30}[a-z0-9])?$ (3-32 chars, lowercase, letter/digit at both ends)").WithHint("Use lowercase letters, digits, and hyphens. Must start and end with a letter or digit.")
		}
		// 自定义码的"是否存在"由 decideVersion 判断（决定是新建还是追加版本）
		return c, true, nil
	}

	// 自动生成 nanoid(6)，重试 5 次（OpenAPI 严格 pattern，碰撞概率极低）
	for i := 0; i < 5; i++ {
		candidate, gerr := nanoid.Default()
		if gerr != nil {
			return "", false, api.NewError(api.CodeInternal, "nanoid",
				fmt.Sprintf("failed to generate code: %v", gerr))
		}
		// candidate 必须满足 OpenAPI pattern（保险起见再校验一次）
		if !autoCodeRe.MatchString(candidate) {
			continue
		}
		exists, gerr := d.store.SiteExists(ctx, candidate)
		if gerr != nil {
			return "", false, api.NewError(api.CodeInternal, "site_exists",
				fmt.Sprintf("failed to check code: %v", gerr))
		}
		if !exists {
			return candidate, false, nil
		}
	}
	return "", false, api.NewError(api.CodeInternal, "nanoid",
		"failed to generate unique code after 5 attempts")
}

// decideVersion 决定 version_number 和是否新建 site。
func (d *Deployer) decideVersion(ctx context.Context, code string, isCustom bool, createVersion bool) (isNewSite bool, versionNumber int64, err *api.APIError) {
	exists, gerr := d.store.SiteExists(ctx, code)
	if gerr != nil {
		return false, 0, api.NewError(api.CodeInternal, "site_exists",
			fmt.Sprintf("failed to check code: %v", gerr))
	}

	if !exists {
		// 新 site，version=1
		return true, 1, nil
	}

	// 已存在
	if isCustom && !createVersion {
		// CONFLICT：需要明确 createVersion=true 才能追加
		return false, 0, api.NewError(api.CodeConflict, "validate",
			fmt.Sprintf("code %q already exists; pass createVersion=true to append a new version", code)).
			WithHint("Iterative work should append versions rather than overwrite.")
	}

	// 追加：取 max(version_number) + 1
	maxVer, gerr := d.store.MaxVersionNumber(ctx, code)
	if gerr != nil {
		return false, 0, api.NewError(api.CodeInternal, "max_version",
			fmt.Sprintf("failed to get max version: %v", gerr))
	}
	return false, maxVer + 1, nil
}

// writeFiles 把文件写到磁盘。所有文件先写到 tmp 目录，再原子 rename。
// 写前用 safeJoin 做路径校验，杜绝穿越。
func (d *Deployer) writeFiles(code string, version int64, files []resolvedFile) error {
	vDir := d.versionDir(code, version)
	if err := os.MkdirAll(vDir, 0o755); err != nil {
		return fmt.Errorf("mkdir version dir: %w", err)
	}

	for _, f := range files {
		full, err := safeJoin(vDir, f.Path)
		if err != nil {
			return fmt.Errorf("safe join %s: %w", f.Path, err)
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return fmt.Errorf("mkdir for %s: %w", f.Path, err)
		}
		// 原子写：tmp + rename
		tmp, err := os.CreateTemp(filepath.Dir(full), ".tmp-"+filepath.Base(full)+"-*")
		if err != nil {
			return fmt.Errorf("create tmp for %s: %w", f.Path, err)
		}
		if _, err := tmp.Write(f.Bytes); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
			return fmt.Errorf("write tmp %s: %w", f.Path, err)
		}
		if err := tmp.Close(); err != nil {
			return fmt.Errorf("close tmp %s: %w", f.Path, err)
		}
		if err := os.Rename(tmp.Name(), full); err != nil {
			_ = os.Remove(tmp.Name())
			return fmt.Errorf("rename tmp %s: %w", f.Path, err)
		}
	}
	return nil
}

// swapCurrentSymlink 切换 current symlink 到新版本目录。
//
// Linux：用 tmp + rename 原子覆盖（rename 是原子的）。
// Windows：os.Rename 不能覆盖现有 symlink，会 Access Denied。
//
//	fallback 到 Remove + Symlink（非原子，但 Windows 上无完美方案）。
//
// 生产环境部署在 Linux，Windows 路径仅用于本地开发测试。
func (d *Deployer) swapCurrentSymlink(code string, version int64) error {
	siteDir := d.siteDir(code)
	if err := os.MkdirAll(siteDir, 0o755); err != nil {
		return fmt.Errorf("mkdir site dir: %w", err)
	}
	currentLink := filepath.Join(siteDir, "current")
	target := filepath.Join("versions", fmt.Sprintf("%d", version))

	// 用相对 symlink（便于迁移、备份）
	tmpLink := filepath.Join(siteDir, ".current.tmp."+randomSuffix())
	if err := os.Symlink(target, tmpLink); err != nil {
		if runtime.GOOS == "windows" {
			return d.replaceCurrentWithCopy(code, version)
		}
		return fmt.Errorf("create tmp symlink: %w", err)
	}

	// 先试原子 rename（Linux 友好）
	if err := os.Rename(tmpLink, currentLink); err == nil {
		return nil
	}

	// Fallback：删除旧的 + rename tmp。Windows 走这条。
	if err := removeCurrentPath(currentLink); err != nil {
		_ = os.Remove(tmpLink)
		return fmt.Errorf("remove old current path: %w", err)
	}
	if err := os.Rename(tmpLink, currentLink); err != nil {
		_ = os.Remove(tmpLink)
		if runtime.GOOS == "windows" {
			if copyErr := d.replaceCurrentWithCopy(code, version); copyErr == nil {
				return nil
			}
		}
		return fmt.Errorf("rename symlink (fallback): %w", err)
	}
	return nil
}

func removeCurrentPath(path string) error {
	if err := os.Remove(path); err == nil || os.IsNotExist(err) {
		return nil
	}
	if err := os.RemoveAll(path); err == nil || os.IsNotExist(err) {
		return nil
	} else {
		return err
	}
}

// Windows 开发机常见场景没有创建目录 symlink 的权限，这里退化为复制目录。
func (d *Deployer) replaceCurrentWithCopy(code string, version int64) error {
	siteDir := d.siteDir(code)
	currentDir := filepath.Join(siteDir, "current")
	srcDir := d.versionDir(code, version)
	tmpDir := filepath.Join(siteDir, ".current.dir."+randomSuffix())
	_ = os.RemoveAll(tmpDir)
	if err := copyDir(srcDir, tmpDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return fmt.Errorf("copy version dir: %w", err)
	}
	if err := removeCurrentPath(currentDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return fmt.Errorf("remove current dir: %w", err)
	}
	if err := os.Rename(tmpDir, currentDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return fmt.Errorf("rename current dir: %w", err)
	}
	return nil
}

func copyDir(srcDir, dstDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := dstDir
		if rel != "." {
			target = filepath.Join(dstDir, rel)
		}
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

// 路径辅助方法
func (d *Deployer) siteDir(code string) string {
	return filepath.Join(d.cfg.HostedDir, code)
}
func (d *Deployer) versionDir(code string, version int64) string {
	return filepath.Join(d.siteDir(code), "versions", fmt.Sprintf("%d", version))
}

func normalizeSource(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "api"
	}
	return s
}

func normalizeVisibility(raw string) (string, bool, *api.APIError) {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return "public", false, nil
	}
	switch value {
	case "public", "unlisted":
		return value, true, nil
	default:
		return "", false, api.NewError(api.CodeInvalidInput, "visibility",
			"visibility must be public or unlisted")
	}
}

func sha256sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// aggregateSha256 把多个文件的 sha256 按 path 排序后拼接，再做一次 sha256。
// 这样相同文件集合（任意顺序）产出相同聚合 hash。
func aggregateSha256(files []resolvedFile) string {
	// 按 path 排序
	paths := make([]string, len(files))
	idxByPath := make(map[string]int, len(files))
	for i, f := range files {
		paths[i] = f.Path
		idxByPath[f.Path] = i
	}
	// 简单插入排序（文件数 ≤100，不引外部依赖）
	for i := 1; i < len(paths); i++ {
		for j := i; j > 0 && paths[j-1] > paths[j]; j-- {
			paths[j-1], paths[j] = paths[j], paths[j-1]
		}
	}
	h := sha256.New()
	for _, p := range paths {
		h.Write([]byte(p))
		h.Write([]byte{0})
		h.Write([]byte(files[idxByPath[p]].Sha256))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// newUUID 生成 v4 UUID，不引外部依赖。
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	// version 4
	b[6] = (b[6] & 0x0f) | 0x40
	// variant 10
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func randomSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// DeleteSite 删除整个 site：先清磁盘目录，再清数据库。
// 任一步失败都返回 INTERNAL 错误，但已删除的步骤不会回滚。
func (d *Deployer) DeleteSite(ctx context.Context, code string) *api.APIError {
	if !customCodeRe.MatchString(code) && !isValidAutoCode(code) {
		return api.NewError(api.CodeInvalidInput, "validate",
			fmt.Sprintf("invalid code %q", code))
	}
	// 校验是否存在
	if _, err := d.store.GetSite(ctx, code); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return api.NewError(api.CodeNotFound, "lookup",
				fmt.Sprintf("site %q not found", code))
		}
		return api.NewError(api.CodeInternal, "lookup",
			fmt.Sprintf("failed to look up site: %v", err))
	}
	// 1. 删磁盘
	dir := d.siteDir(code)
	if err := os.RemoveAll(dir); err != nil {
		return api.NewError(api.CodeInternal, "rm_tree",
			fmt.Sprintf("failed to remove site dir %s: %v", dir, err))
	}
	// 2. 删数据库
	if err := d.store.DeleteSite(ctx, code); err != nil {
		return api.NewError(api.CodeInternal, "delete_site",
			fmt.Sprintf("failed to delete site from db: %v", err))
	}
	return nil
}

// isValidAutoCode 校验自动生成的短码是否合法。
// nanoid 字母表是 [0-9a-zA-Z]，所以允许大小写。
// 长度 1-32，仅 [a-zA-Z0-9-]。
func isValidAutoCode(code string) bool {
	if len(code) < 1 || len(code) > 32 {
		return false
	}
	for _, r := range code {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return false
		}
	}
	return true
}

// PublicBaseURL 暴露当前 baseURL（用于 GET /api/config）。
func (d *Deployer) PublicBaseURL() string { return d.cfg.PublicBaseURL }

func (d *Deployer) AppURLConfig() api.AppURLConfig {
	return api.NewAppURLConfig(d.cfg)
}

// SetPublicBaseURL 把新的 baseURL 写入数据库并热更新内存。
// 下一次部署就会用新值拼 URL。
func (d *Deployer) SetPublicBaseURL(ctx context.Context, baseURL string) error {
	baseURL = strings.TrimSpace(baseURL)
	if err := d.store.SetSetting(ctx, "public_base_url", baseURL); err != nil {
		return fmt.Errorf("persist baseURL: %w", err)
	}
	d.cfg.PublicBaseURL = baseURL
	return nil
}

func (d *Deployer) SetAppURLConfig(ctx context.Context, cfg api.AppURLConfig) error {
	if err := d.store.SetSetting(ctx, "app_url_mode", api.NormalizeAppURLModeForConfig(cfg.AppURLMode)); err != nil {
		return fmt.Errorf("persist app url mode: %w", err)
	}
	if err := d.store.SetSetting(ctx, "app_domain_suffix", api.NormalizeAppDomainSuffixForConfig(cfg.AppDomainSuffix)); err != nil {
		return fmt.Errorf("persist app domain suffix: %w", err)
	}
	if err := d.store.SetSetting(ctx, "app_url_scheme", api.NormalizeAppURLSchemeForConfig(cfg.AppURLScheme)); err != nil {
		return fmt.Errorf("persist app url scheme: %w", err)
	}
	if err := d.store.SetSetting(ctx, "app_url_port", api.NormalizeAppURLPortForConfig(cfg.AppURLPort)); err != nil {
		return fmt.Errorf("persist app url port: %w", err)
	}
	d.cfg.AppURLMode = api.NormalizeAppURLModeForConfig(cfg.AppURLMode)
	d.cfg.AppDomainSuffix = api.NormalizeAppDomainSuffixForConfig(cfg.AppDomainSuffix)
	d.cfg.AppURLScheme = api.NormalizeAppURLSchemeForConfig(cfg.AppURLScheme)
	d.cfg.AppURLPort = api.NormalizeAppURLPortForConfig(cfg.AppURLPort)
	return nil
}

func (d *Deployer) SetAnonymousDeployLimit(ctx context.Context, n int) error {
	if err := d.store.SetSetting(ctx, "anonymous_deploy_limit", strconv.Itoa(n)); err != nil {
		return fmt.Errorf("persist anonymous deploy limit: %w", err)
	}
	d.cfg.AnonymousDeployLimit = n
	return nil
}

func (d *Deployer) SetCooldownSeconds(ctx context.Context, n int) error {
	if err := d.store.SetSetting(ctx, "cooldown_seconds", strconv.Itoa(n)); err != nil {
		return fmt.Errorf("persist cooldown seconds: %w", err)
	}
	d.cfg.CooldownSeconds = n
	d.cooldown.SetWindow(time.Duration(n) * time.Second)
	return nil
}

func (d *Deployer) SetUploadLimits(ctx context.Context, singleFileBytes, siteTotalBytes int64, filesPerSite int) error {
	if err := d.store.SetSetting(ctx, "max_single_file_bytes", strconv.FormatInt(singleFileBytes, 10)); err != nil {
		return fmt.Errorf("persist max single file bytes: %w", err)
	}
	if err := d.store.SetSetting(ctx, "max_site_total_bytes", strconv.FormatInt(siteTotalBytes, 10)); err != nil {
		return fmt.Errorf("persist max site total bytes: %w", err)
	}
	if err := d.store.SetSetting(ctx, "max_files_per_site", strconv.Itoa(filesPerSite)); err != nil {
		return fmt.Errorf("persist max files per site: %w", err)
	}
	d.cfg.MaxSingleFileBytes = singleFileBytes
	d.cfg.MaxSiteTotalBytes = siteTotalBytes
	d.cfg.MaxFilesPerSite = filesPerSite
	return nil
}

func (d *Deployer) SetCORSAllowOrigins(ctx context.Context, origins string) error {
	origins = config.NormalizeCORSAllowOrigins(origins)
	if err := d.store.SetSetting(ctx, "cors_allow_origins", origins); err != nil {
		return fmt.Errorf("persist CORS allow origins: %w", err)
	}
	d.cfg.CORSAllowOrigins = origins
	return nil
}

func sanitizeSiteTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	if strings.EqualFold(title, "index.html") || strings.EqualFold(title, "index.htm") {
		return ""
	}
	return title
}

// LoadPersistedSettings 启动时从数据库恢复持久化设置。
// 数据库里没有则保持 cfg 原值不变，并返回恢复后的配置快照。
func (d *Deployer) LoadPersistedSettings(ctx context.Context) config.Config {
	if v, err := d.store.GetSetting(ctx, "public_base_url"); err == nil && v != "" {
		d.cfg.PublicBaseURL = v
	}
	if v, err := d.store.GetSetting(ctx, "app_url_mode"); err == nil && v != "" {
		d.cfg.AppURLMode = api.NormalizeAppURLModeForConfig(v)
	}
	if v, err := d.store.GetSetting(ctx, "app_domain_suffix"); err == nil {
		d.cfg.AppDomainSuffix = api.NormalizeAppDomainSuffixForConfig(v)
	}
	if v, err := d.store.GetSetting(ctx, "app_url_scheme"); err == nil && v != "" {
		d.cfg.AppURLScheme = api.NormalizeAppURLSchemeForConfig(v)
	}
	if v, err := d.store.GetSetting(ctx, "app_url_port"); err == nil {
		d.cfg.AppURLPort = api.NormalizeAppURLPortForConfig(v)
	}
	if v, err := d.store.GetSetting(ctx, "anonymous_deploy_limit"); err == nil && v != "" {
		if n, perr := strconv.Atoi(v); perr == nil && n >= -1 {
			d.cfg.AnonymousDeployLimit = n
		}
	}
	if v, err := d.store.GetSetting(ctx, "cooldown_seconds"); err == nil && v != "" {
		if n, perr := strconv.Atoi(v); perr == nil && n >= 0 {
			d.cfg.CooldownSeconds = n
			d.cooldown.SetWindow(time.Duration(n) * time.Second)
		}
	}
	if v, err := d.store.GetSetting(ctx, "max_single_file_bytes"); err == nil && v != "" {
		if n, perr := strconv.ParseInt(v, 10, 64); perr == nil && n > 0 {
			d.cfg.MaxSingleFileBytes = n
		}
	}
	if v, err := d.store.GetSetting(ctx, "max_site_total_bytes"); err == nil && v != "" {
		if n, perr := strconv.ParseInt(v, 10, 64); perr == nil && n > 0 {
			d.cfg.MaxSiteTotalBytes = n
		}
	}
	if v, err := d.store.GetSetting(ctx, "max_files_per_site"); err == nil && v != "" {
		if n, perr := strconv.Atoi(v); perr == nil && n > 0 {
			d.cfg.MaxFilesPerSite = n
		}
	}
	if v, err := d.store.GetSetting(ctx, "cors_allow_origins"); err == nil && v != "" {
		d.cfg.CORSAllowOrigins = config.NormalizeCORSAllowOrigins(v)
	}
	return d.cfg
}

// ListSites 透传到 store（admin UI 列表用）。
func (d *Deployer) ListSites(ctx context.Context) ([]store.SiteWithMeta, error) {
	return d.store.ListSites(ctx)
}

func (d *Deployer) CreateAnonymousSession(ctx context.Context, id string) (store.AnonymousSession, error) {
	session := store.AnonymousSession{ID: id}
	if err := d.store.CreateAnonymousSession(ctx, session); err != nil {
		return store.AnonymousSession{}, err
	}
	return d.store.GetAnonymousSession(ctx, id)
}

func (d *Deployer) GetAnonymousSession(ctx context.Context, id string) (store.AnonymousSession, error) {
	return d.store.GetAnonymousSession(ctx, id)
}

func (d *Deployer) UpdateAnonymousSessionMeta(ctx context.Context, id, agentID, agentLabel, deviceIP, userAgent string) error {
	return d.store.UpdateAnonymousSessionMeta(ctx, id, agentID, agentLabel, deviceIP, userAgent)
}

func (d *Deployer) IncrementAnonymousSessionDeployCount(ctx context.Context, id string) (store.AnonymousSession, error) {
	return d.store.IncrementAnonymousSessionDeployCount(ctx, id)
}

func (d *Deployer) ClaimAnonymousSession(ctx context.Context, id, userID string) (store.AnonymousSessionClaimResult, error) {
	return d.store.ClaimAnonymousSession(ctx, id, userID)
}

func (d *Deployer) ListAnonymousSessions(ctx context.Context, limit int) ([]store.AnonymousSession, error) {
	return d.store.ListAnonymousSessions(ctx, limit)
}

// ===== Marketplace（公开应用商城） =====

func (d *Deployer) ListMarketplaceDeploys(ctx context.Context, q, status, sort string, page, pageSize int) ([]store.MarketplaceDeploy, int, error) {
	return d.store.ListMarketplaceDeploys(ctx, q, status, sort, page, pageSize)
}

func (d *Deployer) GetMarketplaceDeploy(ctx context.Context, code string) (store.MarketplaceDeploy, error) {
	return d.store.GetMarketplaceDeploy(ctx, code)
}

func (d *Deployer) GetMarketplaceDeployByUUID(ctx context.Context, publicID string) (store.MarketplaceDeploy, error) {
	return d.store.GetMarketplaceDeployByUUID(ctx, publicID)
}

func (d *Deployer) IncrementViewCount(ctx context.Context, code string) error {
	return d.store.IncrementViewCount(ctx, code)
}

func (d *Deployer) AddLike(ctx context.Context, code, fingerprint string) (int64, error) {
	return d.store.AddLike(ctx, code, fingerprint)
}

func (d *Deployer) SiteExists(ctx context.Context, code string) (bool, error) {
	return d.store.SiteExists(ctx, code)
}

func (d *Deployer) GetSite(ctx context.Context, code string) (store.Site, error) {
	return d.store.GetSite(ctx, code)
}

func (d *Deployer) SetSitePinned(ctx context.Context, code string, pinned bool) error {
	return d.store.SetSitePinned(ctx, code, pinned)
}

func (d *Deployer) SetSiteVisibility(ctx context.Context, code, visibility string) error {
	value, _, apiErr := normalizeVisibility(visibility)
	if apiErr != nil {
		return errors.New(apiErr.Detail)
	}
	return d.store.SetSiteVisibility(ctx, code, value)
}

func (d *Deployer) SetSiteAccessPassword(ctx context.Context, code, password string) error {
	hash := ""
	if strings.TrimSpace(password) != "" {
		var err error
		hash, err = auth.HashPassword(password)
		if err != nil {
			return err
		}
	}
	return d.store.SetSiteAccessPasswordHash(ctx, code, hash)
}

// 让 errors import 不被自动移除（部分错误路径用到 errors.Is）
var _ = errors.Is
