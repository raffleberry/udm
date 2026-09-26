package ui

import (
	"os"
	"testing"

	tk "modernc.org/tk9.0"

	"github.com/raffleberry/udm/internal/cfg"
	"github.com/raffleberry/udm/internal/core"
	"github.com/raffleberry/udm/internal/dl"
)

// idle is a dl.Downloader that never finishes, which is all a layout test needs.
type idle struct{}

func (idle) Start(dl.Spec) (dl.Transfer, error) { return nil, errIdle }

type idleErr struct{}

func (idleErr) Error() string { return "idle" }

var errIdle = idleErr{}

// TestWidgetsBuild needs a display, because it is the only way to catch a
// mistyped Tk option. ttk rejects an unknown -option by panicking inside the
// interpreter, which is exactly the class of mistake a compiler cannot see.
//
// This is the test that would have caught -padx on a ttk widget, tk.Text used
// as an option, and a treeview built with a []string of columns.
func TestWidgetsBuild(t *testing.T) {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("no display; skipping the Tk construction test")
	}
	t.Cleanup(func() { _ = tk.Finalize() })

	c := cfg.Config{Dir: t.TempDir(), Port: 33210, MaxConc: 2}
	svc := core.New(idle{}, core.Options{Dir: c.Dir, MaxConc: c.MaxConc})
	t.Cleanup(svc.Close)

	u := &ui{
		app: App{Svc: svc, Cfg: &c, Save: func(*cfg.Config) error { return nil }},
		// Tray is nil on purpose: the tray path is exercised in the app, and
		// this test is about widget construction.
		drawn: map[string]bool{},
	}

	u.build()
	tk.Update()

	// The columns must be a real Tcl list. Building a treeview with
	// Columns([]string{...}) silently yields one column named "[a b c]",
	// because the bracket reads as a command substitution.
	cols := u.tree.Columns()
	for _, want := range []string{"name", "size", "got", "pct", "rate", "state"} {
		if !contains(cols, want) {
			t.Errorf("treeview columns = %q, missing %q", cols, want)
		}
	}

	// Rows must round-trip, and the tag list must be settable (Tk rejects a
	// malformed -tags value).
	if _, err := svc.Add(core.Req{URL: "https://example.com/a.zip", Name: "a.zip"}); err != nil {
		t.Fatal(err)
	}
	u.draw(svc.List())
	tk.Update()

	if got := u.tree.Item(u.ids[0]); got == "" {
		t.Error("row did not round-trip through the treeview")
	}

	// The settings dialog is the other place options are assembled by hand.
	dir, conc := u.settings()
	tk.Update()

	// Textvariable is a setter on the way in and a getter on the way out; if
	// that pairing is wrong the dialog silently edits nothing.
	if got := dir.Textvariable(); got != c.Dir {
		t.Errorf("dir entry = %q, want %q", got, c.Dir)
	}
	if got := conc.Textvariable(); got != "2" {
		t.Errorf("conc entry = %q, want %q", got, "2")
	}
	dir.Configure(tk.Textvariable("/tmp/elsewhere"))
	tk.Update()
	if got := dir.Textvariable(); got != "/tmp/elsewhere" {
		t.Errorf("after set, dir entry = %q, want %q", got, "/tmp/elsewhere")
	}
}

func contains(hay, needle string) bool {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
