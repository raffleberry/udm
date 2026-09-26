//go:build linux

// sni implements tray.Tray as a freedesktop StatusNotifierItem on the session
// bus, with a com.canonical.dbusmenu for the right-click menu. That is what
// KDE, GNOME and Xfce all speak, and it is the only tray API available without
// cgo. Other platforms get a window that hides and no tray.
//
// ToolTip is intentionally not published: its signature packs a pixmap array
// that no pure-Go marshaller produces conveniently, and every host we have
// tested falls back to Title, which we do set.
package tray

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
)

const (
	itemIface = "org.kde.StatusNotifierItem"
	menuIface = "com.canonical.dbusmenu"
	menuPath  = dbus.ObjectPath("/MenuBar")

	watchPath = dbus.ObjectPath("/StatusNotifierWatcher")
	watchReg  = "org.kde.StatusNotifierWatcher.RegisterStatusNotifierItem"
)

// watchers are the bus names a desktop may publish the notifier under. GNOME
// ships the freedesktop one, KDE and Plasma the kde one; try both before giving
// up, because a missing tray is a silent degradation rather than an error.
var watchers = []string{
	"org.kde.StatusNotifierWatcher",
	"org.freedesktop.StatusNotifierWatcher",
}

// fallbackIcon is a stock name that exists in every icon theme, used when the
// installer has not put udm.png into the hicolor theme.
const fallbackIcon = "emblem-downloads"

type sni struct {
	conn  *dbus.Conn
	item  *statusItem
	menu  *menu
	props *prop.Properties
	name  string
	shown bool
}

type statusItem struct{ onShow func() }

type menu struct {
	items  []Item
	onPick func(int)
}

// node is one row of a dbusmenu layout: (id, properties, children).
type node struct {
	ID       int32
	Props    map[string]dbus.Variant
	Children []dbus.Variant
}

type pair struct {
	ID    int32
	Props map[string]dbus.Variant
}

// Show exports the item and its menu, then introduces itself to the watcher.
func (s *sni) Show(icon string, items []Item, onShow func(), onPick func(int)) error {
	if s.shown {
		return errors.New("tray: already shown")
	}

	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("tray: session bus: %w", err)
	}
	s.conn = conn

	pid := os.Getpid()
	// The object path must be the bus name turned into a path, plus
	// /StatusNotifierItem: that is where the watcher and the panel look for us.
	// The old KDE spelling, /StatusNotifierItem/<pid>, registers successfully
	// and then silently shows nothing.
	itemPath := dbus.ObjectPath(fmt.Sprintf("/org/kde/StatusNotifierItem/%d/StatusNotifierItem", pid))
	if icon == "" {
		icon = fallbackIcon
	}

	s.item = &statusItem{onShow: onShow}
	s.menu = &menu{items: items, onPick: onPick}
	if err := conn.Export(s.item, itemPath, itemIface); err != nil {
		return fmt.Errorf("tray: export item: %w", err)
	}
	if err := conn.Export(s.menu, menuPath, menuIface); err != nil {
		return fmt.Errorf("tray: export menu: %w", err)
	}

	// The *Pixmap and ToolTip properties are deliberately absent. Their D-Bus
	// signatures (a(iiay) and (sa(iiay)ss)) are tuples that godbus cannot
	// marshal, and every host falls back to IconName and Title, which we do
	// provide. Publishing a half-formed value is worse than publishing none:
	// it panics inside GetAll.
	s.props, err = prop.Export(conn, itemPath, map[string]map[string]*prop.Prop{
		itemIface: {
			// Category must be one of the four values the spec names. "Application"
			// is not among them, and a panel that validates it may drop the item
			// without saying anything.
			"Category":          {Value: "ApplicationStatus"},
			"Id":                {Value: "udm"},
			"Title":             {Value: "udm"},
			"Status":            {Value: "Active"},
			"IconName":          {Value: icon},
			"IconThemePath":     {Value: ""},
			"Menu":              {Value: menuPath},
			"ItemIsMenu":        {Value: false},
			"AttentionIconName": {Value: ""},
			"OverlayIconName":   {Value: ""},
		},
	})
	if err != nil {
		return fmt.Errorf("tray: export properties: %w", err)
	}

	// Claim a bus name first so the watcher sees a name that already resolves,
	// and so a name clash surfaces here rather than silently later.
	s.name = fmt.Sprintf("org.kde.StatusNotifierItem-%d", pid)
	reply, err := conn.RequestName(s.name, dbus.NameFlagDoNotQueue)
	if err != nil {
		slog.Warn("tray: request name", "err", err)
	} else if reply != dbus.RequestNameReplyPrimaryOwner {
		slog.Warn("tray: bus name already taken", "name", s.name)
	}

	if err := s.register(); err != nil {
		return err
	}
	s.shown = true
	return nil
}

// register introduces the item to the first watcher that answers. There is no
// matching unregister in the spec: dropping the bus name is what retires us.
func (s *sni) register() error {
	var last error
	for _, w := range watchers {
		if call := s.conn.Object(w, watchPath).Call(watchReg, 0, s.name); call.Err == nil {
			return nil
		} else {
			last = call.Err
		}
	}
	if last == nil {
		last = errors.New("no watcher answered")
	}
	return fmt.Errorf("tray: no status notifier watcher: %w", last)
}

// Close marks the item passive and releases the bus name.
func (s *sni) Close() error {
	if !s.shown {
		return nil
	}
	s.shown = false
	if s.props != nil {
		s.props.SetMust(itemIface, "Status", "Passive")
	}
	if _, err := s.conn.ReleaseName(s.name); err != nil {
		return err
	}
	return nil
}

// statusItem interface. Left and middle click both restore the window; the
// menu is served over dbusmenu, so ContextMenu is a no-op.

func (s *statusItem) Activate(x, y int32) *dbus.Error {
	if s.onShow != nil {
		s.onShow()
	}
	return nil
}

func (s *statusItem) SecondaryActivate(x, y int32) *dbus.Error { return s.Activate(x, y) }

func (s *statusItem) ContextMenu(x, y int32) *dbus.Error { return nil }

func (s *statusItem) Scroll(delta int32, orientation string) *dbus.Error { return nil }

// menu interface. The layout is a flat list of leaves, each its own clickable
// row, so ids are simply 1-based indexes into items.

func (m *menu) GetLayout(parent, depth int32, names []string) (uint32, *node, *dbus.Error) {
	if parent != 0 {
		return 0, &node{}, nil
	}
	root := &node{ID: 0}
	for i, it := range m.items {
		root.Children = append(root.Children, dbus.MakeVariant(m.node(int32(i+1), it)))
	}
	return 1, root, nil
}

func (m *menu) node(id int32, it Item) *node {
	return &node{ID: id, Props: map[string]dbus.Variant{
		"label":   dbus.MakeVariant(it.Label),
		"enabled": dbus.MakeVariant(true),
		"visible": dbus.MakeVariant(true),
	}}
}

func (m *menu) GetGroupProperties(ids []int32, names []string) []pair {
	out := make([]pair, 0, len(ids))
	for _, id := range ids {
		if id < 1 || int(id) > len(m.items) {
			continue
		}
		out = append(out, pair{id, m.node(id, m.items[id-1]).Props})
	}
	return out
}

func (m *menu) GetProperty(id int32, name string) (dbus.Variant, *dbus.Error) {
	if id < 1 || int(id) > len(m.items) {
		return dbus.Variant{}, nil
	}
	return m.node(id, m.items[id-1]).Props[name], nil
}

func (m *menu) AboutToShow(id int32) (bool, *dbus.Error) { return false, nil }

func (m *menu) Event(id int32, event string, data dbus.Variant, ts uint32) *dbus.Error {
	if event != "clicked" || id < 1 || int(id) > len(m.items) || m.onPick == nil {
		return nil
	}
	m.onPick(int(id) - 1)
	return nil
}
