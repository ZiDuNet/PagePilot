package deploy

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yourorg/hostctl/internal/api"
	"github.com/yourorg/hostctl/internal/config"
	"github.com/yourorg/hostctl/internal/store"
)

func TestAnonymousDeployForcesUnlistedEvenWhenPublicRequested(t *testing.T) {
	tmp := t.TempDir()
	st, err := store.NewSQLiteStore(filepath.Join(tmp, "hostctl.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	}()

	cfg := config.Default()
	cfg.HostedDir = filepath.Join(tmp, "hosted")
	cfg.MaxSingleFileBytes = 1 << 20
	cfg.MaxSiteTotalBytes = 2 << 20
	cfg.MaxFilesPerSite = 20

	d := New(cfg, st)
	resp, apiErr := d.Deploy(context.Background(), api.DeployRequest{
		Filename:    "index.html",
		Title:       "Anonymous public request",
		Description: "Anonymous deploys must stay out of the marketplace unless claimed by a user.",
		Content:     "<!doctype html><html><body><h1>Hello PagePilot</h1></body></html>",
		Visibility:  "public",
	}, "anon:test-session", "127.0.0.1")
	if apiErr != nil {
		t.Fatalf("Deploy returned API error: %s", apiErr.Detail)
	}

	site, err := st.GetSite(context.Background(), resp.Code)
	if err != nil {
		t.Fatalf("get site: %v", err)
	}
	if site.OwnerTokenID != "anon:test-session" {
		t.Fatalf("OwnerTokenID = %q; want anon:test-session", site.OwnerTokenID)
	}
	if site.Visibility != "unlisted" {
		t.Fatalf("Visibility = %q; want unlisted", site.Visibility)
	}

	_, total, err := st.ListMarketplaceDeploys(context.Background(), "", "", "likes_desc", "", "", "", "", 1, 10)
	if err != nil {
		t.Fatalf("list marketplace: %v", err)
	}
	if total != 0 {
		t.Fatalf("marketplace total = %d; want 0 for anonymous unlisted deploy", total)
	}
}
