package deploy

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yourorg/hostctl/internal/api"
)

const (
	siteMutationLeaseTTL           = 45 * time.Second
	siteMutationLeaseRenewInterval = 10 * time.Second
	siteMutationLeaseAcquireWait   = 30 * time.Second
	siteMutationLeaseAcquirePoll   = 40 * time.Millisecond
)

// siteMutationLeaseStore is optional so lightweight test and extension stores
// remain compatible. SQLiteStore implements it for production deployments.
type siteMutationLeaseStore interface {
	TryAcquireSiteMutationLease(ctx context.Context, code, holder string, ttl time.Duration) (bool, error)
	RenewSiteMutationLease(ctx context.Context, code, holder string, ttl time.Duration) (bool, error)
	ReleaseSiteMutationLease(ctx context.Context, code, holder string) error
}

// siteMutationLease serializes mutations for a single code across server
// processes. The periodic renewal lets long file writes retain ownership;
// cleanup performs one final synchronous renewal before it removes anything.
type siteMutationLease struct {
	store     siteMutationLeaseStore
	code      string
	holder    string
	expiresAt atomic.Int64
	stop      chan struct{}
	done      chan struct{}
	release   sync.Once
}

func (d *Deployer) acquireSiteMutationLease(ctx context.Context, code string) (*siteMutationLease, *api.APIError) {
	leaseStore, ok := d.store.(siteMutationLeaseStore)
	if !ok {
		return &siteMutationLease{}, nil
	}
	holder, err := newUUID()
	if err != nil {
		return nil, api.NewError(api.CodeInternal, "site_lease", fmt.Sprintf("generate mutation lease holder: %v", err))
	}

	acquireCtx, cancel := context.WithTimeout(ctx, siteMutationLeaseAcquireWait)
	defer cancel()
	for {
		acquired, err := leaseStore.TryAcquireSiteMutationLease(acquireCtx, code, holder, siteMutationLeaseTTL)
		if err != nil {
			return nil, api.NewError(api.CodeInternal, "site_lease", fmt.Sprintf("acquire site mutation lease: %v", err))
		}
		if acquired {
			lease := &siteMutationLease{
				store:  leaseStore,
				code:   code,
				holder: holder,
				stop:   make(chan struct{}),
				done:   make(chan struct{}),
			}
			lease.expiresAt.Store(time.Now().Add(siteMutationLeaseTTL).UnixNano())
			go lease.renewUntilReleased()
			return lease, nil
		}

		timer := time.NewTimer(siteMutationLeaseAcquirePoll)
		select {
		case <-acquireCtx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, api.NewError(api.CodeConflict, "site_lease", "another request is updating this application").
				WithHint("Wait for the current publish or deletion to finish, then retry.")
		case <-timer.C:
		}
	}
}

func (l *siteMutationLease) renewUntilReleased() {
	ticker := time.NewTicker(siteMutationLeaseRenewInterval)
	defer ticker.Stop()
	defer close(l.done)
	for {
		select {
		case <-l.stop:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), siteMutationLeaseRenewInterval)
			_ = l.renew(ctx)
			cancel()
		}
	}
}

func (l *siteMutationLease) renew(ctx context.Context) error {
	if l.store == nil {
		return nil
	}
	renewed, err := l.store.RenewSiteMutationLease(ctx, l.code, l.holder, siteMutationLeaseTTL)
	if err != nil {
		return err
	}
	if !renewed {
		return fmt.Errorf("site mutation lease is no longer held")
	}
	l.expiresAt.Store(time.Now().Add(siteMutationLeaseTTL).UnixNano())
	return nil
}

func (l *siteMutationLease) ensure(ctx context.Context) error {
	if l.store == nil {
		return nil
	}
	if time.Now().UnixNano() >= l.expiresAt.Load() {
		return fmt.Errorf("site mutation lease expired")
	}
	return l.renew(ctx)
}

func (l *siteMutationLease) releaseLease() error {
	if l.store == nil {
		return nil
	}
	var releaseErr error
	l.release.Do(func() {
		close(l.stop)
		<-l.done
		ctx, cancel := context.WithTimeout(context.Background(), siteMutationLeaseRenewInterval)
		defer cancel()
		releaseErr = l.store.ReleaseSiteMutationLease(ctx, l.code, l.holder)
	})
	return releaseErr
}
