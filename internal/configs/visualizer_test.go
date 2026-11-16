package configs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSpectrumConfigurationDefaultsAndOverride(t *testing.T) {
	for _, tc := range []struct {
		name, toml string
		enabled    bool
	}{
		{"existing config", "[main]\nframeRate=7\n", true},
		{"disabled", "[main.visualizer]\nenable=false\n", false},
		{"classic", "[main.lyric]\nlayout=\"classic\"\n[main.visualizer]\nenable=true\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(tc.toml), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := NewConfigFromTomlFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.SpectrumEnabled() != tc.enabled {
				t.Fatalf("spectrum enabled=%v", cfg.SpectrumEnabled())
			}
			if tc.name == "existing config" && cfg.Main.FrameRate != 7 {
				t.Fatal("existing value overwritten")
			}
		})
	}
}
