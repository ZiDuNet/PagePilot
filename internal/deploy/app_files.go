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
	if _, err := d.store.GetVersion(ctx, code, version); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, time.Time{}, api.NewError(api.CodeNotFound, "load_version", fmt.Sprintf("version %d of %q not found", version, code))
		}
		return nil, time.Time{}, api.NewError(api.CodeInternal, "load_version", err.Error())
	}
	if d.useOSS() {
		oss := newOSSStorage(d.cfg)
		data, modTime, err := oss.get(ctx, oss.versionObjectKey(code, version, path))
		if err != nil {
			if errors.Is(err, errFileNotFound) {
				return nil, time.Time{}, api.NewError(api.CodeNotFound, "read_file", "file not found")
			}
			return nil, time.Time{}, api.NewError(api.CodeInternal, "read_file", err.Error())
		}
		return data, modTime, nil
	}
	versionDir := d.versionDir(code, version)
	full := filepath.Join(versionDir, path)
	if err := ensureWithin(versionDir, full); err != nil {
		return nil, time.Time{}, api.NewError(api.CodeNotFound, "file_path", "file not found")
	}
	data, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, time.Time{}, api.NewError(api.CodeNotFound, "read_file", "file not found")
		}
		return nil, time.Time{}, api.NewError(api.CodeInternal, "read_file", err.Error())
	}
	modTime := time.Time{}
	if st, statErr := os.Stat(full); statErr == nil {
		modTime = st.ModTime()
	}
	return data, modTime, nil
}
