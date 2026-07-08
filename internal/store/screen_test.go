package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestScreenPairingBindPublishAndDeviceLookup(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	pairing := ScreenPairing{
		ID:                "pair-1",
		Code:              "123456",
		PairingSecretHash: "pair-secret-hash",
		ScreenID:          "screen-1",
		DeviceName:        "门店一号屏",
		ExpiresAt:         now.Add(5 * time.Minute),
		CreatedAt:         now,
	}
	if err := store.CreateScreenPairing(ctx, pairing); err != nil {
		t.Fatalf("create screen pairing: %v", err)
	}

	screen, err := store.BindScreenPairing(ctx, "123456", "user-1", "大厅屏")
	if err != nil {
		t.Fatalf("bind screen pairing: %v", err)
	}
	if screen.ID != "screen-1" || screen.OwnerUserID != "user-1" || screen.Name != "大厅屏" {
		t.Fatalf("screen after bind = %+v", screen)
	}

	screens, err := store.ListScreensByUser(ctx, "user-1")
	if err != nil {
		t.Fatalf("list screens: %v", err)
	}
	if len(screens) != 1 || screens[0].ID != "screen-1" {
		t.Fatalf("screens = %+v, want screen-1", screens)
	}

	if err := store.CompleteScreenPairing(ctx, "pair-1", "pair-secret-hash", "device-token-hash"); err != nil {
		t.Fatalf("complete screen pairing: %v", err)
	}
	byToken, err := store.GetScreenByDeviceTokenHash(ctx, "device-token-hash")
	if err != nil {
		t.Fatalf("get screen by device token: %v", err)
	}
	if byToken.ID != "screen-1" || byToken.OwnerUserID != "user-1" {
		t.Fatalf("screen by token = %+v", byToken)
	}

	version := int64(2)
	if err := store.PublishScreen(ctx, "screen-1", "user-1", "demo-app", &version); err != nil {
		t.Fatalf("publish screen: %v", err)
	}
	published, err := store.GetScreen(ctx, "screen-1")
	if err != nil {
		t.Fatalf("get published screen: %v", err)
	}
	if published.CurrentSiteCode != "demo-app" || published.CurrentVersion == nil || *published.CurrentVersion != 2 {
		t.Fatalf("published screen = %+v", published)
	}
}

func TestBindScreenPairingRejectsExpiredCode(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	err := store.CreateScreenPairing(ctx, ScreenPairing{
		ID:                "pair-expired",
		Code:              "654321",
		PairingSecretHash: "hash",
		ScreenID:          "screen-expired",
		DeviceName:        "旧屏",
		ExpiresAt:         now.Add(-time.Minute),
		CreatedAt:         now.Add(-10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("create expired pairing: %v", err)
	}

	if _, err := store.BindScreenPairing(ctx, "654321", "user-1", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bind expired pairing err = %v, want ErrNotFound", err)
	}
}

func TestAssignScreenOwnerAllowsDeviceToCompletePairing(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := store.CreateScreenPairing(ctx, ScreenPairing{
		ID:                "pair-assign",
		Code:              "112233",
		PairingSecretHash: "pair-secret-hash",
		ScreenID:          "screen-assign",
		DeviceName:        "screen device",
		AppVersion:        "0.1.0",
		Runtime:           "X5 WebView",
		DeviceInfo:        `{"model":"demo"}`,
		ExpiresAt:         now.Add(5 * time.Minute),
		CreatedAt:         now,
	}); err != nil {
		t.Fatalf("create pairing: %v", err)
	}

	before, err := store.GetScreen(ctx, "screen-assign")
	if err != nil {
		t.Fatalf("get pending screen: %v", err)
	}
	if before.OwnerUserID != "" || before.Status != "pairing" || before.AppVersion != "0.1.0" {
		t.Fatalf("pending screen = %+v", before)
	}

	assigned, err := store.AssignScreenOwner(ctx, "screen-assign", "user-1", "lobby screen")
	if err != nil {
		t.Fatalf("assign screen owner: %v", err)
	}
	if assigned.OwnerUserID != "user-1" || assigned.Name != "lobby screen" || assigned.Status != "bound" {
		t.Fatalf("assigned screen = %+v", assigned)
	}

	if err := store.CompleteScreenPairing(ctx, "pair-assign", "pair-secret-hash", "device-token-hash"); err != nil {
		t.Fatalf("complete assigned pairing: %v", err)
	}
	byToken, err := store.GetScreenByDeviceTokenHash(ctx, "device-token-hash")
	if err != nil {
		t.Fatalf("get assigned screen by token: %v", err)
	}
	if byToken.ID != "screen-assign" || byToken.OwnerUserID != "user-1" || byToken.Status != "online" {
		t.Fatalf("screen by token = %+v", byToken)
	}
}

func TestPublishScreenRejectsWrongOwner(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := store.CreateScreenPairing(ctx, ScreenPairing{
		ID:                "pair-1",
		Code:              "123456",
		PairingSecretHash: "hash",
		ScreenID:          "screen-1",
		DeviceName:        "屏幕",
		ExpiresAt:         now.Add(5 * time.Minute),
		CreatedAt:         now,
	}); err != nil {
		t.Fatalf("create pairing: %v", err)
	}
	if _, err := store.BindScreenPairing(ctx, "123456", "user-1", ""); err != nil {
		t.Fatalf("bind pairing: %v", err)
	}

	if err := store.PublishScreen(ctx, "screen-1", "user-2", "demo-app", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("publish wrong owner err = %v, want ErrNotFound", err)
	}
}
