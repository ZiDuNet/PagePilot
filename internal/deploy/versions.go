package deploy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/yourorg/hostctl/internal/api"
	"github.com/yourorg/hostctl/internal/store"
)

// ListVersions 列出某 site 所有版本。
func (d *Deployer) ListVersions(ctx context.Context, code string) (*api.ListVersionsResponse, *api.APIError) {
	site, err := d.store.GetSite(ctx, code)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, api.NewError(api.CodeNotFound, "load_site", fmt.Sprintf("code %q not found", code))
		}
		return nil, api.NewError(api.CodeInternal, "load_site", err.Error())
	}
	versions, err := d.store.ListVersions(ctx, code)
	if err != nil {
		return nil, api.NewError(api.CodeInternal, "list_versions", err.Error())
	}

	items := make([]api.VersionItem, 0, len(versions))
	for _, v := range versions {
		isCurrent := site.CurrentVersion != nil && *site.CurrentVersion == v.VersionNumber
		items = append(items, api.VersionItem{
			VersionNumber: v.VersionNumber,
			ID:            v.ID,
			Title:         v.Title,
			Description:   v.Description,
			Size:          v.TotalSize,
			FileCount:     v.FileCount,
			IsLocked:      v.IsLocked,
			IsCurrent:     isCurrent,
			Status:        v.Status,
			CreatedAt:     v.CreatedAt,
		})
	}
	return &api.ListVersionsResponse{
		Success:        true,
		Code:           code,
		CurrentVersion: site.CurrentVersion,
		Versions:       items,
	}, nil
}

// LockVersion 设置/解除某版本锁定。
func (d *Deployer) LockVersion(ctx context.Context, code string, version int64, locked bool) (*api.LockResponse, *api.APIError) {
	// 校验版本存在
	if _, err := d.store.GetVersion(ctx, code, version); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, api.NewError(api.CodeNotFound, "load_version",
				fmt.Sprintf("version %d of %q not found", version, code))
		}
		return nil, api.NewError(api.CodeInternal, "load_version", err.Error())
	}
	if err := d.store.UpdateVersionLock(ctx, code, version, locked); err != nil {
		return nil, api.NewError(api.CodeInternal, "update_lock", err.Error())
	}
	return &api.LockResponse{
		Success:       true,
		Code:          code,
		VersionNumber: version,
		IsLocked:      locked,
	}, nil
}

// SwitchCurrent 切换某 site 的当前对外版本。
// 同时切磁盘 symlink。
func (d *Deployer) SwitchCurrent(ctx context.Context, code string, version int64) (*api.SetCurrentResponse, *api.APIError) {
	site, err := d.store.GetSite(ctx, code)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, api.NewError(api.CodeNotFound, "load_site", fmt.Sprintf("code %q not found", code))
		}
		return nil, api.NewError(api.CodeInternal, "load_site", err.Error())
	}
	v, err := d.store.GetVersion(ctx, code, version)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, api.NewError(api.CodeNotFound, "load_version",
				fmt.Sprintf("version %d of %q not found", version, code))
		}
		return nil, api.NewError(api.CodeInternal, "load_version", err.Error())
	}
	if v.Status != "active" {
		return nil, api.NewError(api.CodeInvalidInput, "switch_current",
			fmt.Sprintf("version %d is not active (status=%s)", version, v.Status)).
			WithHint("Activate the version with status=active before switching.")
	}

	if err := d.store.SetCurrentVersion(ctx, code, &version); err != nil {
		return nil, api.NewError(api.CodeInternal, "set_current", err.Error())
	}
	if err := d.swapCurrentSymlink(code, version); err != nil {
		// 回滚 DB 状态
		_ = d.store.SetCurrentVersion(ctx, code, site.CurrentVersion)
		return nil, api.NewError(api.CodeInternal, "swap_symlink",
			fmt.Sprintf("failed to swap symlink: %v", err))
	}
	return &api.SetCurrentResponse{
		Success:        true,
		Code:           code,
		CurrentVersion: version,
	}, nil
}

// SwitchCurrentByUUID 同上但用 versions.id。
func (d *Deployer) SwitchCurrentByUUID(ctx context.Context, code, versionID string) (*api.SetCurrentResponse, *api.APIError) {
	v, err := d.store.GetVersionByUUID(ctx, versionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, api.NewError(api.CodeNotFound, "load_version",
				fmt.Sprintf("version %q not found", versionID))
		}
		return nil, api.NewError(api.CodeInternal, "load_version", err.Error())
	}
	if v.SiteCode != code {
		return nil, api.NewError(api.CodeNotFound, "load_version",
			fmt.Sprintf("version %q does not belong to %q", versionID, code))
	}
	return d.SwitchCurrent(ctx, code, v.VersionNumber)
}

// OverwriteVersion 覆盖未锁定版本的内容（Day 4 用，先实现核心逻辑）。
func (d *Deployer) OverwriteVersion(ctx context.Context, code string, version int64, req api.OverwriteRequest) (*api.DeployResponse, *api.APIError) {
	// 加载版本
	v, err := d.store.GetVersion(ctx, code, version)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, api.NewError(api.CodeNotFound, "load_version",
				fmt.Sprintf("version %d of %q not found", version, code))
		}
		return nil, api.NewError(api.CodeInternal, "load_version", err.Error())
	}
	if v.IsLocked {
		return nil, api.NewError(api.CodeVersionLocked, "overwrite",
			fmt.Sprintf("version %d is locked and cannot be modified", version)).
			WithHint("Append a new version instead of overwriting a locked one.")
	}

	// 校验 description
	desc := strings.TrimSpace(req.Description)
	if desc == "" {
		return nil, api.NewError(api.CodeInvalidDescription, "validate",
			"description is required")
	}
	if len(desc) > 240 {
		return nil, api.NewError(api.CodeInvalidDescription, "validate",
			"description must be at most 240 characters")
	}

	// 解析内容（复用 DeployRequest 的解析路径）
	mainEntry := strings.TrimSpace(req.Filename)
	if mainEntry == "" {
		mainEntry = v.MainEntry
	}
	pseudoReq := api.DeployRequest{
		Description: desc,
		Title:       req.Title,
		Filename:    mainEntry,
		Content:     req.Content,
		Files:       req.Files,
	}
	rfiles, resolvedMainEntry, apiErr := d.resolveContent(pseudoReq, mainEntry)
	if apiErr != nil {
		return nil, apiErr
	}
	mainEntry = resolvedMainEntry
	if apiErr := validateEntrypoint(rfiles, mainEntry); apiErr != nil {
		return nil, apiErr
	}

	// 删除旧文件 + 写新文件
	oldDir := d.versionDir(code, version)
	// 写到临时目录再 swap
	tmpDir := d.versionDir(code, -1*version) // 负数避免与正常版本号冲突
	_ = os.RemoveAll(tmpDir)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return nil, api.NewError(api.CodeInternal, "mkdir_tmp", err.Error())
	}

	// 写文件到 tmpDir
	if err := d.writeFilesToDir(tmpDir, rfiles); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, api.NewError(api.CodeInternal, "write_files", err.Error())
	}

	// 原子替换：rename oldDir -> oldDir.bak，rename tmpDir -> oldDir
	bakDir := oldDir + ".bak." + randomSuffix()
	if err := os.Rename(oldDir, bakDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, api.NewError(api.CodeInternal, "rename_old", err.Error())
	}
	if err := os.Rename(tmpDir, oldDir); err != nil {
		// 回滚：把 bak 改回来
		_ = os.Rename(bakDir, oldDir)
		_ = os.RemoveAll(tmpDir)
		return nil, api.NewError(api.CodeInternal, "rename_new", err.Error())
	}
	_ = os.RemoveAll(bakDir)

	// 更新元数据
	aggregateSha := aggregateSha256(rfiles)
	var totalSize int64
	for _, f := range rfiles {
		totalSize += int64(len(f.Bytes))
	}

	if err := d.store.UpdateVersionContent(ctx, code, version, store.Version{
		Title:         sanitizeSiteTitle(req.Title),
		Description:   desc,
		MainEntry:     mainEntry,
		TotalSize:     totalSize,
		FileCount:     len(rfiles),
		ContentSha256: aggregateSha,
	}, toFileMetas(code, version, rfiles)); err != nil {
		return nil, api.NewError(api.CodeInternal, "update_version", err.Error())
	}

	// 如果是当前版本，重新切 symlink（指向同一个目录路径，但内容变了）
	site, err := d.store.GetSite(ctx, code)
	if err == nil && site.CurrentVersion != nil && *site.CurrentVersion == version {
		_ = d.swapCurrentSymlink(code, version)
	}

	// 拼响应
	appURLs := d.AppURLConfig()
	return &api.DeployResponse{
		Success:       true,
		Code:          code,
		URL:           appURLs.PrimaryAppURL(code, nil),
		DetailURL:     appURLs.PrimaryAppURL(code, nil),
		VersionURL:    appURLs.PrimaryAppURL(code, &version),
		VersionNumber: int(version),
		PreserveHint:  "Lock this version to prevent modifications.",
		Size:          totalSize,
		CreatedAt:     v.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

// SetVersionStatus 切换 active/inactive（Day 4）。
func (d *Deployer) SetVersionStatus(ctx context.Context, code string, version int64, status string) (*api.LockResponse, *api.APIError) {
	v, err := d.store.GetVersion(ctx, code, version)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, api.NewError(api.CodeNotFound, "load_version",
				fmt.Sprintf("version %d of %q not found", version, code))
		}
		return nil, api.NewError(api.CodeInternal, "load_version", err.Error())
	}
	if v.IsLocked {
		return nil, api.NewError(api.CodeVersionLocked, "set_status",
			fmt.Sprintf("version %d is locked and cannot be modified", version))
	}
	if status != "active" && status != "inactive" {
		return nil, api.NewError(api.CodeInvalidInput, "validate",
			"status must be 'active' or 'inactive'")
	}
	if err := d.store.UpdateVersionStatus(ctx, code, version, status); err != nil {
		return nil, api.NewError(api.CodeInternal, "update_status", err.Error())
	}
	return &api.LockResponse{
		Success:       true,
		Code:          code,
		VersionNumber: version,
		IsLocked:      v.IsLocked,
	}, nil
}

// DeleteVersion 删除版本（Day 4）。
func (d *Deployer) DeleteVersion(ctx context.Context, code string, version int64) (*api.SetCurrentResponse, *api.APIError) {
	v, err := d.store.GetVersion(ctx, code, version)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, api.NewError(api.CodeNotFound, "load_version",
				fmt.Sprintf("version %d of %q not found", version, code))
		}
		return nil, api.NewError(api.CodeInternal, "load_version", err.Error())
	}
	if v.IsLocked {
		return nil, api.NewError(api.CodeVersionLocked, "delete",
			fmt.Sprintf("version %d is locked and cannot be deleted", version))
	}

	// 删 DB 记录
	if err := d.store.DeleteVersion(ctx, code, version); err != nil {
		return nil, api.NewError(api.CodeInternal, "delete_version", err.Error())
	}

	// 删磁盘文件（用 .del 后缀再异步清理，避免 race；这里同步删简化）
	go func() {
		_ = os.RemoveAll(d.versionDir(code, version))
	}()

	// 如果删的是当前版本，找一个 active 版本顶上
	site, err := d.store.GetSite(ctx, code)
	if err != nil {
		return &api.SetCurrentResponse{Success: true, Code: code, CurrentVersion: 0}, nil
	}
	if site.CurrentVersion != nil && *site.CurrentVersion == version {
		// 找下一个 active
		versions, _ := d.store.ListVersions(ctx, code)
		var nextVersion *int64
		for i := len(versions) - 1; i >= 0; i-- {
			if versions[i].Status == "active" {
				vv := versions[i].VersionNumber
				nextVersion = &vv
				break
			}
		}
		_ = d.store.SetCurrentVersion(ctx, code, nextVersion)
		if nextVersion != nil {
			_ = d.swapCurrentSymlink(code, *nextVersion)
		} else {
			// 没有可服务的版本，删 symlink
			_ = os.Remove(filepath.Join(d.siteDir(code), "current"))
		}
		return &api.SetCurrentResponse{Success: true, Code: code, CurrentVersion: orZero(nextVersion)}, nil
	}

	cur := int64(0)
	if site.CurrentVersion != nil {
		cur = *site.CurrentVersion
	}
	return &api.SetCurrentResponse{Success: true, Code: code, CurrentVersion: cur}, nil
}

// GetContent 返回某版本的元数据 + 文件清单（Day 5 会加 zip 打包逻辑）。
func (d *Deployer) GetContent(ctx context.Context, code string, versionPtr *int64) (*api.GetContentResponse, *api.APIError) {
	site, err := d.store.GetSite(ctx, code)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, api.NewError(api.CodeNotFound, "load_site", fmt.Sprintf("code %q not found", code))
		}
		return nil, api.NewError(api.CodeInternal, "load_site", err.Error())
	}
	var version int64
	if versionPtr != nil {
		version = *versionPtr
	} else if site.CurrentVersion != nil {
		version = *site.CurrentVersion
	} else {
		return nil, api.NewError(api.CodeNotFound, "no_current",
			fmt.Sprintf("code %q has no active version", code))
	}

	v, err := d.store.GetVersion(ctx, code, version)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, api.NewError(api.CodeNotFound, "load_version",
				fmt.Sprintf("version %d of %q not found", version, code))
		}
		return nil, api.NewError(api.CodeInternal, "load_version", err.Error())
	}
	files, err := d.store.ListFiles(ctx, code, version)
	if err != nil {
		return nil, api.NewError(api.CodeInternal, "list_files", err.Error())
	}

	out := api.GetContentResponse{
		Success:     true,
		Code:        code,
		Version:     version,
		Title:       v.Title,
		Description: v.Description,
		MainEntry:   v.MainEntry,
		TotalSize:   v.TotalSize,
		IsLocked:    v.IsLocked,
		Files:       make([]api.ContentFile, 0, len(files)),
		CreatedAt:   v.CreatedAt,
	}
	for _, f := range files {
		cf := api.ContentFile{
			Path:     f.Path,
			Size:     f.Size,
			Sha256:   f.Sha256,
			IsBinary: f.IsBinary,
		}
		if !f.IsBinary && f.Size <= 2<<20 {
			full := filepath.Join(d.versionDir(code, version), f.Path)
			if err := ensureWithin(d.versionDir(code, version), full); err != nil {
				return nil, api.NewError(api.CodeInternal, "read_file", err.Error())
			}
			b, err := os.ReadFile(full)
			if err != nil {
				return nil, api.NewError(api.CodeInternal, "read_file", err.Error())
			}
			if utf8.Valid(b) {
				cf.Content = string(b)
				if f.Path == v.MainEntry {
					out.Content = cf.Content
				}
			}
		}
		out.Files = append(out.Files, cf)
	}
	return &out, nil
}

// GetPrimaryStrategy 读 main URL 选版本策略 + 当前 primary 版本信息。
//   - strategy="likes"：primary = 第一个 is_locked=true 的 active 版本；无锁定则退化为最新 active
//   - strategy="latest"：primary = 最新 active 版本
func (d *Deployer) GetPrimaryStrategy(ctx context.Context, code string) (*api.PrimaryStrategyResponse, *api.APIError) {
	site, err := d.store.GetSite(ctx, code)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, api.NewError(api.CodeNotFound, "load_site", fmt.Sprintf("code %q not found", code))
		}
		return nil, api.NewError(api.CodeInternal, "load_site", err.Error())
	}
	versions, err := d.store.ListVersions(ctx, code)
	if err != nil {
		return nil, api.NewError(api.CodeInternal, "list_versions", err.Error())
	}
	strategy := api.StrategyLikes
	if site.PrimaryVersionStrategy == string(api.StrategyLatest) {
		strategy = api.StrategyLatest
	}
	primary := pickPrimary(versions, strategy)
	currentID := ""
	if site.CurrentVersion != nil {
		for _, v := range versions {
			if v.VersionNumber == *site.CurrentVersion {
				currentID = v.ID
				break
			}
		}
	}
	resp := &api.PrimaryStrategyResponse{
		Success:                true,
		Code:                   code,
		PrimaryVersionStrategy: strategy,
		CurrentVersionID:       currentID,
	}
	if primary != nil {
		resp.PrimaryVersionID = primary.ID
		resp.PrimaryVersionNumber = primary.VersionNumber
	}
	return resp, nil
}

// SetPrimaryStrategy 写入 main URL 选版本策略。
func (d *Deployer) SetPrimaryStrategy(ctx context.Context, code string, strategy api.PrimaryVersionStrategy) (*api.PrimaryStrategyResponse, *api.APIError) {
	if strategy != api.StrategyLikes && strategy != api.StrategyLatest {
		return nil, api.NewError(api.CodeInvalidInput, "validate",
			"primaryVersionStrategy must be 'likes' or 'latest'")
	}
	if _, err := d.store.GetSite(ctx, code); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, api.NewError(api.CodeNotFound, "load_site", fmt.Sprintf("code %q not found", code))
		}
		return nil, api.NewError(api.CodeInternal, "load_site", err.Error())
	}
	if err := d.store.SetPrimaryStrategy(ctx, code, string(strategy)); err != nil {
		return nil, api.NewError(api.CodeInternal, "set_strategy", err.Error())
	}
	return d.GetPrimaryStrategy(ctx, code)
}

// pickPrimary 根据策略挑出 primary 版本。
func pickPrimary(versions []store.Version, strategy api.PrimaryVersionStrategy) *store.Version {
	if len(versions) == 0 {
		return nil
	}
	if strategy == api.StrategyLatest {
		for i := len(versions) - 1; i >= 0; i-- {
			if versions[i].Status == "active" {
				return &versions[i]
			}
		}
		return nil
	}
	// likes：先找锁定版本，按 version_number desc
	for i := len(versions) - 1; i >= 0; i-- {
		if versions[i].Status == "active" && versions[i].IsLocked {
			return &versions[i]
		}
	}
	// 退化：最新 active
	for i := len(versions) - 1; i >= 0; i-- {
		if versions[i].Status == "active" {
			return &versions[i]
		}
	}
	return nil
}

// writeFilesToDir 把文件写到指定目录（用于 OverwriteVersion 的 tmp 写入）。
func (d *Deployer) writeFilesToDir(dir string, files []resolvedFile) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, f := range files {
		full, err := safeJoin(dir, f.Path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		tmp, err := os.CreateTemp(filepath.Dir(full), ".tmp-"+filepath.Base(full)+"-*")
		if err != nil {
			return err
		}
		if _, err := tmp.Write(f.Bytes); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
			return err
		}
		if err := tmp.Close(); err != nil {
			return err
		}
		if err := os.Rename(tmp.Name(), full); err != nil {
			_ = os.Remove(tmp.Name())
			return err
		}
	}
	return nil
}

func toFileMetas(code string, version int64, files []resolvedFile) []store.FileMeta {
	out := make([]store.FileMeta, 0, len(files))
	for _, f := range files {
		out = append(out, store.FileMeta{
			SiteCode:      code,
			VersionNumber: version,
			Path:          f.Path,
			Size:          int64(len(f.Bytes)),
			Sha256:        f.Sha256,
			IsBinary:      f.IsBinary,
		})
	}
	return out
}

func orZero(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
