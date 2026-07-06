package deploy

import (
	"context"

	"github.com/yourorg/hostctl/internal/store"
)

func (d *Deployer) GetRenderCache(ctx context.Context, cacheKey string) (store.RenderCacheEntry, bool, error) {
	return d.store.GetRenderCache(ctx, cacheKey)
}

func (d *Deployer) PutRenderCache(ctx context.Context, entry store.RenderCacheEntry) error {
	return d.store.PutRenderCache(ctx, entry)
}
