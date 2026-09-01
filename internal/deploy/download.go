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
	"strconv"

	"github.com/yourorg/hostctl/internal/api"
	"github.com/yourorg/hostctl/internal/bundle"
	"github.com/yourorg/hostctl/internal/config"
	"github.com/yourorg/hostctl/internal/store"
)

const (
	maxDownloadUncompressedBytes = int64(256 << 20)
	zipArchiveOverheadBytes      = int64(1 << 20)
	maxDownloadArchiveBytes      = maxDownloadUncompressedBytes + zipArchiveOverheadBytes
	maxDownloadFiles             = 10000
)

var errDownloadArchiveTooLarge = errors.New("download archive exceeds limit")

type downloadLimitWriter struct {
	written int64
	limit   int64
	w       io.Writer
}

func (w *downloadLimitWriter) Write(p []byte) (int, error) {
	if w.limit > 0 && (int64(len(p)) > w.limit-w.written || w.written > w.limit) {
		return 0, errDownloadArchiveTooLarge
	}
	n, err := w.w.Write(p)
	w.written += int64(n)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	return n, err
}

func downloadLimits(cfg config.Config) (uncompressed, archive int64) {
	uncompressed = cfg.MaxSiteTotalBytes
	if uncompressed <= 0 || uncompressed > maxDownloadUncompressedBytes {
		uncompressed = maxDownloadUncompressedBytes
	}
	archive = uncompressed
	if archive <= maxDownloadArchiveBytes-zipArchiveOverheadBytes {
		archive += zipArchiveOverheadBytes
	} else {
		archive = maxDownloadArchiveBytes
	}
	return uncompressed, archive
}

// StreamDownload 处理 GET /api/deploy/content?download=1，并始终以 ZIP
// 流返回完整源码（application/zip + Content-Disposition）。
//
// 错误码：NOT_FOUND / INTERNAL。
// 先写入临时文件并校验 ZIP，再提交 HTTP 头，避免后端读取失败时返回
// 看似成功但实际损坏的 200 响应，同时避免为大下载保留两份内存副本。
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

	v, err := d.store.GetVersion(ctx, code, version)
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
	if len(files) > maxDownloadFiles {
		return api.NewError(api.CodeContentTooLarge, "download",
			fmt.Sprintf("version contains too many files to download safely (max %d)", maxDownloadFiles)).
			WithHint("Reduce the number of files in the version or download selected files from the deployment storage.")
	}
	maxUncompressed, maxArchive := downloadLimits(d.configSnapshot())
	if v.TotalSize < 0 || v.TotalSize > maxUncompressed {
		return api.NewError(api.CodeContentTooLarge, "download",
			fmt.Sprintf("version expands beyond the %d-byte download limit", maxUncompressed)).
			WithHint("Download a smaller version or ask an administrator to raise the site upload limit.")
	}

	tmp, err := os.CreateTemp("", "hostctl-download-*.zip")
	if err != nil {
		return api.NewError(api.CodeInternal, "download_temp", err.Error())
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()
	limited := &downloadLimitWriter{limit: maxArchive, w: tmp}
	zw := zip.NewWriter(limited)
	var uncompressedTotal int64

	for _, mf := range files {
		if mf.Size > 0 && (uncompressedTotal > maxUncompressed || mf.Size > maxUncompressed-uncompressedTotal) {
			_ = zw.Close()
			_ = tmp.Close()
			return api.NewError(api.CodeContentTooLarge, "download",
				fmt.Sprintf("version expands beyond the %d-byte download limit", maxUncompressed)).
				WithHint("Download a smaller version or ask an administrator to raise the site upload limit.")
		}
		if !zipPathSafe(mf.Path) {
			_ = zw.Close()
			_ = tmp.Close()
			return api.NewError(api.CodeInternal, "zip_path", fmt.Sprintf("unsafe file path %q in stored version", mf.Path))
		}
		writer, err := zw.Create(mf.Path)
		if err != nil {
			_ = zw.Close()
			_ = tmp.Close()
			return api.NewError(api.CodeInternal, "zip_create", err.Error())
		}
		body, _, apiErr := d.readAppFileLimited(ctx, code, &version, mf.Path, maxUncompressed-uncompressedTotal)
		if apiErr != nil {
			_ = zw.Close()
			_ = tmp.Close()
			return apiErr
		}
		if int64(len(body)) > maxUncompressed-uncompressedTotal {
			_ = zw.Close()
			_ = tmp.Close()
			return api.NewError(api.CodeContentTooLarge, "download",
				fmt.Sprintf("version expands beyond the %d-byte download limit", maxUncompressed)).
				WithHint("Download a smaller version or ask an administrator to raise the site upload limit.")
		}
		uncompressedTotal += int64(len(body))
		if n, writeErr := writer.Write(body); writeErr != nil || n != len(body) {
			_ = zw.Close()
			_ = tmp.Close()
			if errors.Is(writeErr, errDownloadArchiveTooLarge) {
				return api.NewError(api.CodeContentTooLarge, "download",
					fmt.Sprintf("compressed archive exceeds the %d-byte download limit", maxArchive)).
					WithHint("Download a smaller version or remove large generated assets.")
			}
			if writeErr == nil {
				writeErr = fmt.Errorf("short write: wrote %d of %d bytes", n, len(body))
			}
			return api.NewError(api.CodeInternal, "zip_write", writeErr.Error())
		}
	}
	if err := zw.Close(); err != nil {
		_ = tmp.Close()
		if errors.Is(err, errDownloadArchiveTooLarge) {
			return api.NewError(api.CodeContentTooLarge, "download",
				fmt.Sprintf("compressed archive exceeds the %d-byte download limit", maxArchive)).
				WithHint("Download a smaller version or remove large generated assets.")
		}
		return api.NewError(api.CodeInternal, "zip_close", err.Error())
	}
	if err := tmp.Close(); err != nil {
		return api.NewError(api.CodeInternal, "download_close", err.Error())
	}
	archive, err := os.Open(tmpName)
	if err != nil {
		return api.NewError(api.CodeInternal, "download_open", err.Error())
	}
	defer archive.Close()
	stat, err := archive.Stat()
	if err != nil {
		return api.NewError(api.CodeInternal, "download_stat", err.Error())
	}

	// 始终以 zip 包下载，方便用户拿到完整源码。
	zipName := fmt.Sprintf("%s-v%d.zip", code, version)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, zipName))
	w.Header().Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, archive)
	return nil
}

func zipPathSafe(path string) bool {
	return bundle.IsSafePath(path)
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
