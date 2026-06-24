package deploy

import (
	"context"

	"github.com/yourorg/hostctl/internal/store"
)

func (d *Deployer) CreateScreenPairing(ctx context.Context, pairing store.ScreenPairing) error {
	return d.store.CreateScreenPairing(ctx, pairing)
}

func (d *Deployer) BindScreenPairing(ctx context.Context, code, ownerUserID, name string) (store.Screen, error) {
	return d.store.BindScreenPairing(ctx, code, ownerUserID, name)
}

func (d *Deployer) CompleteScreenPairing(ctx context.Context, pairingID, pairingSecretHash, deviceTokenHash string) error {
	return d.store.CompleteScreenPairing(ctx, pairingID, pairingSecretHash, deviceTokenHash)
}

func (d *Deployer) GetScreen(ctx context.Context, id string) (store.Screen, error) {
	return d.store.GetScreen(ctx, id)
}

func (d *Deployer) GetScreenByDeviceTokenHash(ctx context.Context, hash string) (store.Screen, error) {
	return d.store.GetScreenByDeviceTokenHash(ctx, hash)
}

func (d *Deployer) ListScreensByUser(ctx context.Context, ownerUserID string) ([]store.Screen, error) {
	return d.store.ListScreensByUser(ctx, ownerUserID)
}

func (d *Deployer) ListScreens(ctx context.Context) ([]store.Screen, error) {
	return d.store.ListScreens(ctx)
}

func (d *Deployer) PublishScreen(ctx context.Context, screenID, ownerUserID, siteCode string, version *int64) error {
	return d.store.PublishScreen(ctx, screenID, ownerUserID, siteCode, version)
}

func (d *Deployer) TouchScreenHeartbeat(ctx context.Context, screenID, appVersion, runtime string) (store.Screen, error) {
	return d.store.TouchScreenHeartbeat(ctx, screenID, appVersion, runtime)
}

func (d *Deployer) UnbindScreen(ctx context.Context, screenID, ownerUserID string) error {
	return d.store.UnbindScreen(ctx, screenID, ownerUserID)
}
