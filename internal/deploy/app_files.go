package deploy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yourorg/hostctl/internal/api"
	"github.com/yourorg/hostctl/internal/store"
)

// ReadAppFile returns one deployed file from the active or requested version.
// Keeping this behind Deployer lets local disk and future OSS storage share one serving path.
func (d *Deployer) ReadAppFile(ctx context.Context, code string, versionPtr *int64, path string) ([]byte, time.Time, *api.APIError) {
	if !zipPathSafe(path) {
		return nil, time.Time{}, api.NewError(api.CodeNotFound, "file_path", "file not found")
	}
	site, err := d.store.GetSite(ctx, code)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, time.Time{}, api.NewError(api.CodeNotFound, "load_site", fmt.Sprintf("code %q not found", code))
		}
		return nil, time.Time{}, api.NewError(api.CodeInternal, "load_site", err.Error())
	}
	var version int64
	if versionPtr != nil {
		version = *versionPtr
	} else if site.CurrentVersion != nil {
		version = *site.CurrentVersion
	} else {
		return nil, time.Time{}, api.NewError(api.CodeNotFound, "no_current", fmt.Sprintf("code %q has no active version", code))
	}
	v, err := d.store.GetVersion(ctx, code, version)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, time.Time{}, api.NewError(api.CodeNotFound, "load_version", fmt.Sprintf("version %d of %q not found", version, code))
		}
		return nil, time.Time{}, api.NewError(api.CodeInternal, "load_version", err.Error())
	}
	data, modTime, err := d.readVersionFile(ctx, v, path)
	if err != nil {
		if errors.Is(err, errFileNotFound) {
			return nil, time.Time{}, api.NewError(api.CodeNotFound, "read_file", "file not found")
		}
		return nil, time.Time{}, api.NewError(api.CodeInternal, "read_file", err.Error())
	}
	return data, modTime, nil
}

// readVersionFile 统一读取版本文件。OSS 模式只在对象不存在时回退本地旧目录，
// 这样从本地存储平滑切到 OSS 时，历史发布不会立刻 404。
func (d *Deployer) readVersionFile(ctx context.Context, v store.Version, path string) ([]byte, time.Time, error) {
	storage := d.versionStorage(v)
	if storage.Backend == "oss" {
		oss := newOSSStorage(d.cfg)
		data, modTime, err := oss.get(ctx, storage.ossObjectKey(path))
		if err == nil {
			return data, modTime, nil
		}
		if !errors.Is(err, errFileNotFound) {
			return nil, time.Time{}, err
		}
		return d.readDefaultLocalVersionFile(v, path)
	}
	return d.readLocalVersionFile(v, path)
}

func (d *Deployer) readDefaultLocalVersionFile(v store.Version, path string) ([]byte, time.Time, error) {
	localVersion := v
	localVersion.StorageBackend = "local"
	localVersion.StoragePrefix = ""
	return d.readLocalVersionFile(localVersion, path)
}

func (d *Deployer) readLocalVersionFile(v store.Version, path string) ([]byte, time.Time, error) {
	versionDir, err := d.localVersionDir(d.versionStorage(v), v.SiteCode, v.VersionNumber)
	if err != nil {
		return nil, time.Time{}, err
	}
	full := filepath.Join(versionDir, path)
	if err := ensureWithin(versionDir, full); err != nil {
		return nil, time.Time{}, err
	}
	data, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, time.Time{}, errFileNotFound
		}
		return nil, time.Time{}, err
	}
	modTime := time.Time{}
	if st, statErr := os.Stat(full); statErr == nil {
		modTime = st.ModTime()
	}
	return data, modTime, nil
}
