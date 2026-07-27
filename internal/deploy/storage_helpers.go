package deploy

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/yourorg/hostctl/internal/store"
)

func (d *Deployer) useOSS() bool {
	return strings.EqualFold(strings.TrimSpace(d.cfg.StorageBackend), "oss")
}

type versionStorage struct {
	Backend string
	Prefix  string
}

func (d *Deployer) newVersionStorage(code string, version int64) versionStorage {
	if d.useOSS() {
		oss := newOSSStorage(d.cfg)
		return versionStorage{Backend: "oss", Prefix: strings.TrimSuffix(oss.versionPrefix(code, version), "/")}
	}
	return versionStorage{Backend: "local", Prefix: filepath.ToSlash(filepath.Join(code, "versions", fmt.Sprintf("%d", version)))}
}

func (d *Deployer) versionStorage(v store.Version) versionStorage {
	backend := strings.ToLower(strings.TrimSpace(v.StorageBackend))
	if backend != "oss" {
		backend = "local"
	}
	prefix := strings.Trim(strings.ReplaceAll(v.StoragePrefix, "\\", "/"), "/")
	if prefix == "" {
		if backend == "oss" {
			oss := newOSSStorage(d.cfg)
			prefix = strings.TrimSuffix(oss.versionPrefix(v.SiteCode, v.VersionNumber), "/")
		} else {
			prefix = filepath.ToSlash(filepath.Join(v.SiteCode, "versions", fmt.Sprintf("%d", v.VersionNumber)))
		}
	}
	return versionStorage{Backend: backend, Prefix: prefix}
}

func (s versionStorage) ossObjectKey(rel string) string {
	rel = strings.Trim(strings.ReplaceAll(rel, "\\", "/"), "/")
	if rel == "" {
		return path.Clean(s.Prefix)
	}
	return path.Clean(strings.Trim(s.Prefix, "/") + "/" + rel)
}

func (d *Deployer) localVersionDir(s versionStorage, code string, version int64) (string, error) {
	if strings.TrimSpace(s.Prefix) == "" {
		return d.versionDir(code, version), nil
	}
	root := d.cfg.HostedDir
	full := filepath.Join(root, filepath.FromSlash(strings.Trim(s.Prefix, "/")))
	if err := ensureWithin(root, full); err != nil {
		return "", err
	}
	return full, nil
}

func (d *Deployer) deleteVersionFiles(ctx context.Context, code string, version int64) error {
	v, err := d.store.GetVersion(ctx, code, version)
	if err != nil {
		return err
	}
	return d.deleteVersionFilesForStorage(ctx, code, version, d.versionStorage(v))
}

func (d *Deployer) deleteVersionFilesForStorage(ctx context.Context, code string, version int64, storage versionStorage) error {
	if storage.Backend == "oss" {
		oss := newOSSStorage(d.cfg)
		return oss.deletePrefix(ctx, strings.Trim(storage.Prefix, "/")+"/")
	}
	dir, err := d.localVersionDir(storage, code, version)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

func (d *Deployer) deleteSiteFiles(ctx context.Context, code string) error {
	versions, err := d.store.ListVersions(ctx, code)
	if err != nil {
		return err
	}
	for _, v := range versions {
		if err := d.deleteVersionFilesForStorage(ctx, code, v.VersionNumber, d.versionStorage(v)); err != nil {
			return err
		}
	}
	return os.RemoveAll(d.siteDir(code))
}
