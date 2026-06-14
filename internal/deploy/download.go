package deploy

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/yourorg/hostctl/internal/api"
	"github.com/yourorg/hostctl/internal/store"
)

// StreamDownload 处理 GET /api/deploy/content?download=1。
//   - 单文件 site：直接 serve 主入口（text/html; charset=utf-8）
//   - 多文件 site：打包成 zip 流式下载（application/zip + Content-Disposition）
//
// 错误码：NOT_FOUND / INTERNAL。
// 一旦开始写 body（Header + WriteHeader），后续错误只能通过截断响应体现。
func (d *Deployer) StreamDownload(ctx context.Context, code string, versionPtr *int64, w http.ResponseWriter) *api.APIError {
	site, err := d.store.GetSite(ctx, code)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return api.NewError(api.CodeNotFound, "load_site", fmt.Sprintf("code %q not found", code))
		}
		return api.NewError(api.CodeInternal, "load_site", err.Error())
	}
	var version int64
	if versionPtr != nil {
		version = *versionPtr
	} else if site.CurrentVersion != nil {
		version = *site.CurrentVersion
	} else {
		return api.NewError(api.CodeNotFound, "no_current",
			fmt.Sprintf("code %q has no active version", code))
	}

	_, err = d.store.GetVersion(ctx, code, version)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return api.NewError(api.CodeNotFound, "load_version",
				fmt.Sprintf("version %d of %q not found", version, code))
		}
		return api.NewError(api.CodeInternal, "load_version", err.Error())
	}
	files, err := d.store.ListFiles(ctx, code, version)
	if err != nil {
		return api.NewError(api.CodeInternal, "list_files", err.Error())
	}

	versionDir := d.versionDir(code, version)

	// 始终以 zip 包下载，方便用户拿到完整源码。
	zipName := fmt.Sprintf("%s-v%d.zip", code, version)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, zipName))
	w.WriteHeader(http.StatusOK)

	zw := zip.NewWriter(w)
	defer zw.Close()

	for _, mf := range files {
		if !zipPathSafe(mf.Path) {
			return nil
		}
		full := filepath.Join(versionDir, mf.Path)
		if err := ensureWithin(versionDir, full); err != nil {
			// 已 write header，无法返回错误；只能 break
			return nil
		}
		writer, err := zw.Create(mf.Path)
		if err != nil {
			return nil
		}
		f, err := os.Open(full)
		if err != nil {
			return nil
		}
		_, _ = io.Copy(writer, f)
		_ = f.Close()
	}
	return nil
}

func zipPathSafe(path string) bool {
	if path == "" || strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\`) {
		return false
	}
	path = filepath.ToSlash(path)
	for _, seg := range strings.Split(path, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

// ensureWithin 校验 path 在 root 内（防穿越）。
func ensureWithin(root, path string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return err
	}
	if rel == ".." || len(rel) >= 3 && rel[0:3] == ".."+string(filepath.Separator) {
		return fmt.Errorf("path escapes root: %s", path)
	}
	return nil
}
