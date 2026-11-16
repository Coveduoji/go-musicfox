package configs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/anhoder/foxful-cli/model"
)

func TestMenuColumnsPreservesLegacyAndExplicitOverride(t *testing.T) {
	for _, tc := range []struct {
		toml    string
		columns int
		dual    bool
	}{
		{"[theme]\ndoubleColumn=false\n", 0, false},
		{"[theme]\ndoubleColumn=false\nmenuColumns=3\n", 3, false},
		{"[theme]\nmenuColumns=1\n", 1, true},
		{"[theme]\nmenuColumns=8\n", 3, true},
	} {
		path := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(path, []byte(tc.toml), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := NewConfigFromTomlFile(path)
		if err != nil {
			t.Fatal(err)
		}
		opts := model.DefaultOptions()
		cfg.FillToModelOpts(opts)
		if opts.MenuColumns != tc.columns || opts.DualColumn != tc.dual {
			t.Fatalf("wrong menu options: columns=%d dual=%v", opts.MenuColumns, opts.DualColumn)
		}
	}
}
