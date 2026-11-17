package configs

import (
	"path/filepath"
	"testing"

	"github.com/go-musicfox/go-musicfox/internal/keybindings"
)

func TestDefaultSpaceBindingOnlyTogglesPlayback(t *testing.T) {
	_, err := NewConfigFromTomlFile(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	mapping := keybindings.BuildKeyToOperateTypeMap(EffectiveKeybindings)
	for _, key := range []string{" ", "　"} {
		if mapping[key] != keybindings.OpToggle {
			t.Fatal("space still uses mixed selected-song playback")
		}
	}
	if mapping["enter"] != keybindings.OpEnter {
		t.Fatal("Enter lost ordinary menu navigation")
	}
}
