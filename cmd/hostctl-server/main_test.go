package main

import "testing"

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
