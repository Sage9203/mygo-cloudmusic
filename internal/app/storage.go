package app

import (
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

// Renaming the app must not strand settings, playback history or cached songs
// from earlier previews. Fresh installations use the current application name.
func preservePreviewData(config, cache string) error {
	const previousName = "MyGo Music Local"
	data := filepath.Join(config, previousName)
	info, err := os.Stat(data)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return nil
	}
	mygo.App.SetPath(mygo.PathUserData, data)
	if cache != "" {
		mygo.App.SetPath(mygo.PathCache, filepath.Join(cache, previousName))
	}
	return nil
}
