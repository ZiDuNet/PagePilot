package main

import (
	"testing"

	"github.com/yourorg/hostctl/internal/config"
)

func TestApplyServerFlagOverridesPreservesExplicitValues(t *testing.T) {
	cfg := config.Config{
		HTTPAddr:  "127.0.0.1:8787",
		HostedDir: "data/hosted",
		DBPath:    "data/hostctl.db",
	}
	got := applyServerFlagOverrides(cfg, serverFlagValues{
		addr:      "127.0.0.1:8790",
		hostedDir: "/tmp/hosted",
		dbPath:    "/tmp/pagepilot.db",
	}, map[string]bool{
		"addr": true,
		"db":   true,
	})
	if got.HTTPAddr != "127.0.0.1:8790" {
		t.Fatalf("HTTPAddr = %q, want explicit override", got.HTTPAddr)
	}
	if got.DBPath != "/tmp/pagepilot.db" {
		t.Fatalf("DBPath = %q, want explicit override", got.DBPath)
	}
	if got.HostedDir != "data/hosted" {
		t.Fatalf("HostedDir = %q, want dev default when flag was not provided", got.HostedDir)
	}
}

func TestLoadMasterKeyDevFallbackUsesLegacyKey(t *testing.T) {
	t.Setenv("HOSTCTL_DEV", "1")
	t.Setenv("HOSTCTL_MASTER_KEY", "")

	key, err := loadMasterKey()
	if err != nil {
		t.Fatalf("loadMasterKey() error = %v", err)
	}
	var want [32]byte
	copy(want[:], "pagepilot-dev-master-key-0000000")
	if key != want {
		t.Fatalf("loadMasterKey() = %x; want legacy fallback", key)
	}
}

func TestLoadMasterKeyAcceptsLegacyRawKey(t *testing.T) {
	t.Setenv("HOSTCTL_DEV", "0")
	t.Setenv("HOSTCTL_MASTER_KEY", "pagepilot-dev-master-key-0000000")

	key, err := loadMasterKey()
	if err != nil {
		t.Fatalf("loadMasterKey() error = %v", err)
	}
	var want [32]byte
	copy(want[:], "pagepilot-dev-master-key-0000000")
	if key != want {
		t.Fatalf("loadMasterKey() = %x; want legacy raw key", key)
	}
}

func TestLoadMasterKeyAcceptsHexKey(t *testing.T) {
	t.Setenv("HOSTCTL_DEV", "0")
	t.Setenv("HOSTCTL_MASTER_KEY", "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")

	key, err := loadMasterKey()
	if err != nil {
		t.Fatalf("loadMasterKey() error = %v", err)
	}
	for i, value := range key {
		if value != byte(i) {
			t.Fatalf("loadMasterKey()[%d] = %x; want %x", i, value, byte(i))
		}
	}
}

func TestLoadMasterKeyRejectsLongRawKey(t *testing.T) {
	t.Setenv("HOSTCTL_DEV", "0")
	t.Setenv("HOSTCTL_MASTER_KEY", "this raw key is intentionally longer than exactly 32 bytes")

	if _, err := loadMasterKey(); err != errMasterKeyLength {
		t.Fatalf("loadMasterKey() error = %v; want %v", err, errMasterKeyLength)
	}
}

func TestEffectiveRequireAuthDefaultsToProductionProtection(t *testing.T) {
	t.Setenv("HOSTCTL_DEV", "")
	if !effectiveRequireAuth(false) {
		t.Fatal("production must require authentication even without a flag")
	}
}

func TestEffectiveRequireAuthAllowsExplicitDevelopmentMode(t *testing.T) {
	t.Setenv("HOSTCTL_DEV", "enabled")
	if effectiveRequireAuth(false) {
		t.Fatal("development mode unexpectedly forced authentication")
	}
	if !effectiveRequireAuth(true) {
		t.Fatal("explicit development authentication was not preserved")
	}
}
