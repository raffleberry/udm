// Package cfg owns everything udm persists: where downloads land, and which
// localhost port the browser extension talks to.
//
// Both the app and the native messaging host read this file, so the port is
// declared in exactly one place. Run `udm -print-port` if you need it in a script.
package cfg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// App is the folder name used under the OS config dir.
	App = "udm"

	// Port is the default localhost API port. The extension hardcodes the same
	// number (see extension/background.js) — change both, or change settings.json.
	Port = 33210

	// MaxConc is the default number of simultaneous transfers.
	MaxConc = 3
)

// Config is the on-disk settings document.
type Config struct {
	// Dir is where finished and in-flight files are written.
	Dir string `json:"dir"`
	// Port is the localhost port for the extension API.
	Port int `json:"port"`
	// MaxConc caps simultaneous transfers; the rest queue.
	MaxConc int `json:"max_conc"`
}

// Dir returns the folder holding settings.json and the log.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(base, App)
	return d, os.MkdirAll(d, 0o755)
}

// Path is the settings file.
func Path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "settings.json"), nil
}

// Load reads settings.json, falling back to defaults for anything missing so a
// partial or corrupt file still yields a usable app.
func Load() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, err
	}
	c := Config{
		Dir:     filepath.Join(home, "Downloads"),
		Port:    Port,
		MaxConc: MaxConc,
	}

	path, err := Path()
	if err != nil {
		return c, err
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// First run: write the defaults out so there is always a file to edit.
		return c, c.Save()
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	c.fix()
	return c, c.Save()
}

// fix repairs values that would make the app unusable.
func (c *Config) fix() {
	if c.Dir == "" {
		home, _ := os.UserHomeDir()
		c.Dir = filepath.Join(home, "Downloads")
	}
	if c.Port <= 0 || c.Port > 65535 {
		c.Port = Port
	}
	if c.MaxConc <= 0 {
		c.MaxConc = MaxConc
	}
}

// Save writes settings.json.
func (c Config) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
