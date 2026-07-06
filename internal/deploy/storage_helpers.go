package deploy

import (
	"context"
	"os"
	"strings"
)

func (d *Deployer) useOSS() bool {
	return strings.EqualFold(strings.TrimSpace(d.cfg.StorageBackend), "oss")
}

func (d *Deployer) deleteVersionFiles(ctx context.Context, code string, version int64) error {
	if d.useOSS() {
		oss := newOSSStorage(d.cfg)
		return oss.deletePrefix(ctx, oss.versionPrefix(code, version))
	}
	return os.RemoveAll(d.versionDir(code, version))
}

func (d *Deployer) deleteSiteFiles(ctx context.Context, code string) error {
	if d.useOSS() {
		oss := newOSSStorage(d.cfg)
		return oss.deletePrefix(ctx, oss.objectKey(code)+"/")
	}
	return os.RemoveAll(d.siteDir(code))
}
