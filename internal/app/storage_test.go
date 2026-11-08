package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
)

// The public app name must still read and update settings from an earlier preview.
func TestPreviewSettingsSurviveRename(t *testing.T) {
	config, cache := t.TempDir(), t.TempDir()
	old := filepath.Join(config, "MyGo Music Local")
	if err := os.MkdirAll(old, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(old, "settings.json")
	if err := os.WriteFile(file, []byte(`{"volume":0.3,"quality":"hires"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, p := range []mygo.PathName{mygo.PathUserData, mygo.PathCache, mygo.PathLogs} {
			mygo.App.SetPath(p, "")
		}
	})
	if err := preservePreviewData(config, cache); err != nil {
		t.Fatal(err)
	}
	s := loadSettings()
	if s.Volume != 0.3 || s.Quality != "hires" {
		t.Fatal("renamed app did not load prior settings")
	}
	s.Volume = 0.4
	if err := s.save(); err != nil {
		t.Fatal(err)
	}
	if s = loadSettings(); s.Volume != 0.4 {
		t.Fatal("renamed app did not save in the prior directory")
	}
}

// Without prior preview data, an existing path configuration is left alone.
func TestFreshInstallKeepsCurrentPath(t *testing.T) {
	current := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, current)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	if err := preservePreviewData(t.TempDir(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	got, err := mygo.App.Path(mygo.PathUserData)
	if err != nil || got != current {
		t.Fatal("fresh installation selected a legacy directory")
	}
}
