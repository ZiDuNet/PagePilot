package store

import (
	"context"
	"testing"
	"time"
)

func TestMarketplaceSearchUsesFTSAndBackfills(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedSearchableSite(t, store, "ai-report", "PagePilot 渗透测试报告", "发现 CORS 和源码泄露风险", "security,report", now)

	got, total, err := store.ListMarketplaceDeploys(ctx, "源码泄露", "active", "newest", "", "", "", "", 1, 10)
	if err != nil {
		t.Fatalf("ListMarketplaceDeploys returned error: %v", err)
	}
	if total != 1 || len(got) != 1 || got[0].Code != "ai-report" {
		t.Fatalf("search result = total %d %#v, want ai-report", total, got)
	}
}

func TestAuditLogsCanBeRecordedAndFiltered(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := store.RecordAuditLog(ctx, AuditLog{
		ActorType:  "user",
		ActorID:    "user-1",
		ActorRole:  "admin",
		Action:     "deploy",
		SiteCode:   "demo",
		TargetType: "site",
		TargetID:   "demo",
		IP:         "127.0.0.1",
		UserAgent:  "test",
		DetailJSON: `{"version":1}`,
		CreatedAt:  now,
	}); err != nil {
		t.Fatalf("record audit log: %v", err)
	}

	logs, total, err := store.ListAuditLogs(ctx, AuditLogFilter{
		SiteCode:  "demo",
		Action:    "deploy",
		ActorRole: "admin",
		TargetID:  "demo",
		Query:     "version",
		Limit:     20,
	})
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("audit logs = total %d %#v, want one", total, logs)
	}
	if logs[0].ActorID != "user-1" || logs[0].ActorRole != "admin" || logs[0].DetailJSON != `{"version":1}` {
		t.Fatalf("unexpected audit log: %+v", logs[0])
	}
}

func TestAuditLogSearchCoversSourceFieldsAndTimeRange(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	oldTime := now.Add(-24 * time.Hour)

	if err := store.RecordAuditLog(ctx, AuditLog{
		ActorType:  "anonymous",
		ActorID:    "anon-old",
		ActorRole:  "anonymous",
		Action:     "deploy.create",
		Result:     "success",
		SiteCode:   "old-site",
		TargetType: "site",
		TargetID:   "old-site",
		IP:         "10.0.0.1",
		UserAgent:  "OldAgent",
		DetailJSON: `{"title":"old"}`,
		CreatedAt:  oldTime,
	}); err != nil {
		t.Fatalf("record old audit log: %v", err)
	}
	if err := store.RecordAuditLog(ctx, AuditLog{
		ActorType:  "user",
		ActorID:    "user-2",
		ActorRole:  "admin",
		Action:     "skill.package_upload",
		Result:     "success",
		SiteCode:   "",
		TargetType: "skill_package",
		TargetID:   "hostctl-deploy.zip",
		IP:         "172.16.0.8",
		UserAgent:  "AuditBrowser/1.0",
		DetailJSON: `{"filename":"pagep.zip"}`,
		CreatedAt:  now,
	}); err != nil {
		t.Fatalf("record new audit log: %v", err)
	}

	since := now.Add(-time.Hour)
	until := now.Add(time.Hour)
	logs, total, err := store.ListAuditLogs(ctx, AuditLogFilter{
		Query: "AuditBrowser",
		Since: &since,
		Until: &until,
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("list audit logs by user agent: %v", err)
	}
	if total != 1 || len(logs) != 1 || logs[0].TargetID != "hostctl-deploy.zip" {
		t.Fatalf("user-agent audit search = total %d %#v, want skill package log", total, logs)
	}

	logs, total, err = store.ListAuditLogs(ctx, AuditLogFilter{
		Query: "172.16.0.8",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("list audit logs by ip: %v", err)
	}
	if total != 1 || len(logs) != 1 || logs[0].UserAgent != "AuditBrowser/1.0" {
		t.Fatalf("ip audit search = total %d %#v, want source IP match", total, logs)
	}
}

func TestRenderCacheStoresAndInvalidatesByKey(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	entry := RenderCacheEntry{
		CacheKey:      "docs:1:readme:sha:default",
		SiteCode:      "docs",
		VersionNumber: 1,
		MainEntry:     "README.md",
		ContentSHA256: "sha",
		Theme:         "default",
		HTML:          "<h1>Docs</h1>",
		CreatedAt:     now,
	}
	if err := store.PutRenderCache(ctx, entry); err != nil {
		t.Fatalf("put render cache: %v", err)
	}
	got, ok, err := store.GetRenderCache(ctx, entry.CacheKey)
	if err != nil {
		t.Fatalf("get render cache: %v", err)
	}
	if !ok || got.HTML != entry.HTML {
		t.Fatalf("cache = ok:%v %+v, want stored entry", ok, got)
	}
	if err := store.DeleteRenderCacheForVersion(ctx, "docs", 1); err != nil {
		t.Fatalf("delete render cache: %v", err)
	}
	_, ok, err = store.GetRenderCache(ctx, entry.CacheKey)
	if err != nil {
		t.Fatalf("get render cache after delete: %v", err)
	}
	if ok {
		t.Fatal("expected render cache to be invalidated")
	}
}

func seedSearchableSite(t *testing.T, store *SQLiteStore, code, title, description, tags string, createdAt time.Time) {
	t.Helper()
	ctx := context.Background()
	if err := store.CreateSite(ctx, Site{
		Code:         code,
		PublicID:     code + "-public-id",
		OwnerTokenID: "user:owner",
		Visibility:   "public",
		Category:     "security",
		Tags:         tags,
		Status:       "active",
		CreatedAt:    createdAt,
		UpdatedAt:    createdAt,
		Source:       "api",
	}); err != nil {
		t.Fatalf("create searchable site %s: %v", code, err)
	}
	if err := store.CreateVersion(ctx, Version{
		ID:            code + "-version-id",
		SiteCode:      code,
		VersionNumber: 1,
		Title:         title,
		Description:   description,
		MainEntry:     "index.html",
		TotalSize:     100,
		FileCount:     1,
		ContentSha256: code + "-sha",
		Status:        "active",
		CreatedAt:     createdAt,
	}); err != nil {
		t.Fatalf("create searchable version %s: %v", code, err)
	}
	v := int64(1)
	if err := store.SetCurrentVersion(ctx, code, &v); err != nil {
		t.Fatalf("set searchable current %s: %v", code, err)
	}
}
