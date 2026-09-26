// Package tray puts udm in the system notification area, which is how the
// window is restored after it is minimised away.
//
// Only Linux has an implementation. Elsewhere New returns nil and the window
// simply hides, so callers must treat a nil Tray as normal rather than fatal.
package tray

// Item is one row of the tray's right-click menu.
type Item struct {
	Label string
}

// Tray is the notification area. Show is called once per process.
type Tray interface {
	// Show registers the icon and its menu. icon is looked up in the current
	// icon theme — the installer puts udm.png there under the name "udm", and
	// the implementation falls back to a stock name if that is missing.
	//
	// onShow fires when the icon itself is activated, which is the gesture
	// users expect to restore a minimised window. onPick receives the index of
	// the menu item chosen.
	Show(icon string, items []Item, onShow func(), onPick func(int)) error
	// Close takes the icon back down.
	Close() error
}
