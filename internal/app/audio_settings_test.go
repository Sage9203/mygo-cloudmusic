package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Selecting exclusive output must persist the choice without starting a track.
func TestExclusiveOutputSetting(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows output")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := ui.NewTester(a.settingsView, 560, 720)
	tt.Frame()
	if err := tt.Click("启用原采样率独占输出"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	data, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved settings
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.Exclusive || a.pl.Ready() {
		t.Fatal("choice was not saved, or unexpectedly started playback")
	}
}
