package deploy

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yourorg/hostctl/internal/config"
	"github.com/yourorg/hostctl/internal/store"
)

func TestLoadPersistedSettingsNormalizesWildcardCORS(t *testing.T) {
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

	if err := st.SetSetting(context.Background(), "cors_allow_origins", "*"); err != nil {
		t.Fatalf("set setting: %v", err)
	}

	d := New(config.Default(), st)
	cfg := d.LoadPersistedSettings(context.Background())
	if cfg.CORSAllowOrigins != "" {
		t.Fatalf("CORSAllowOrigins = %q, want empty", cfg.CORSAllowOrigins)
	}
}

func TestLoadPersistedSettingsIgnoresOutOfRangeCooldown(t *testing.T) {
	tmp := t.TempDir()
	st, err := store.NewSQLiteStore(filepath.Join(tmp, "hostctl.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	if err := st.SetSetting(context.Background(), "cooldown_seconds", "9223372037"); err != nil {
		t.Fatalf("set setting: %v", err)
	}
	cfg := config.Default()
	cfg.CooldownSeconds = 7
	d := New(cfg, st)
	got := d.LoadPersistedSettings(context.Background())
	if got.CooldownSeconds != 7 {
		t.Fatalf("CooldownSeconds = %d; want persisted invalid value to be ignored", got.CooldownSeconds)
	}
}

func TestSetCooldownSecondsRejectsOutOfRangeValue(t *testing.T) {
	d, _ := newDeployTestHarness(t)
	if err := d.SetCooldownSeconds(context.Background(), config.MaxCooldownSeconds+1); err == nil {
		t.Fatal("SetCooldownSeconds accepted a value above the maximum")
	}
}
