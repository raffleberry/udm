//go:build linux

package tray

// New returns the D-Bus StatusNotifierItem tray.
func New() Tray { return &sni{} }
