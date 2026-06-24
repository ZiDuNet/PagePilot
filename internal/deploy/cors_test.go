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
