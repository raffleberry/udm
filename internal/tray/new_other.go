//go:build !linux

package tray

// New returns nil: there is no cgo-free tray on these platforms, so the window
// just quits on close instead of hiding. Callers must handle a nil Tray.
func New() Tray { return nil }
