//go:build !linux

package host

import "errors"

// Install is Linux-only for now: the browser host directory, the hicolor icon
// install and the udm binary beside the host are all freedesktop conventions.
// The app itself and its HTTP API do build everywhere.
func Install() error {
	return errors.New("host: install is implemented for linux only; run the udm app directly and point the extension at it")
}
