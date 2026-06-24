package whitelist

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const configDirName = "com.svpchain.agent-gui"
const prefsFileName = "prefs.json"

// prefsPathOverride is set by tests to redirect whitelist loading.
var prefsPathOverride string

// SetPrefsPathOverride redirects LoadStore to path for tests.
func SetPrefsPathOverride(path string) {
	prefsPathOverride = path
}

// PrefsPath returns the GUI prefs.json path shared with internal/desktop.
func PrefsPath() string {
	if prefsPathOverride != "" {
		return prefsPathOverride
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, configDirName, prefsFileName)
}

// LoadStore reads whitelist entries from prefs.json. Missing or invalid files yield an empty store.
func LoadStore() *Store {
	path := PrefsPath()
	if path == "" {
		return NewStore(nil)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return NewStore(nil)
	}
	var partial struct {
		Whitelist []Entry `json:"whitelist"`
	}
	if err := json.Unmarshal(data, &partial); err != nil {
		return NewStore(nil)
	}
	return NewStore(partial.Whitelist)
}
