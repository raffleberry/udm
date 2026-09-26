// Package ui is the Tk front end. It is an adapter: it renders core.Job
// snapshots and calls core.Controller methods, and knows nothing about grab,
// HTTP, or the extension.
//
// Threading: Tk is not thread-safe, so every widget call happens on the Tk main
// thread. Work arriving from elsewhere — the redraw pump, the tray — is handed
// over with tk.PostEvent.
package ui

import (
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"sync"
	"time"

	tk "modernc.org/tk9.0"
	_ "modernc.org/tk9.0/themes/azure"

	"github.com/raffleberry/udm/internal/cfg"
	"github.com/raffleberry/udm/internal/core"
	"github.com/raffleberry/udm/internal/tray"
)

// App is everything the window needs.
type App struct {
	Svc  *core.Service
	Cfg  *cfg.Config
	Tray tray.Tray
	// Save persists changed settings. Required.
	Save func(*cfg.Config) error
}

type ui struct {
	app   App
	tree  *tk.TTreeviewWidget
	bar   *tk.LabelWidget
	drawn map[string]bool // job ids currently shown as rows

	ids   []string // every known job id, from the last draw
	note  string
	since time.Time

	quitOnce sync.Once
	before   func()
}

// Run builds the window and blocks in Tk's event loop. before runs when the
// user quits, just before the loop is torn down, so the caller can stop its
// servers while the process is still healthy.
func Run(a App, before func()) {
	if a.Save == nil {
		panic("ui: Save is required")
	}
	u := &ui{app: a, before: before, drawn: map[string]bool{}}
	u.tray()
	u.build()
	go u.pump()
	slog.Info("ui: up")
	tk.App.Wait()
	slog.Info("ui: down")
}

// tray publishes the icon. If the desktop has no notification area to talk to,
// the tray is dropped and the window falls back to quitting on close — the one
// behaviour that is always available.
func (u *ui) tray() {
	t := u.app.Tray
	if t == nil {
		return
	}
	items := []tray.Item{{Label: "Show udm"}, {Label: "Exit"}}

	// Only name the icon if its file is actually installed. The tray falls back
	// to a stock theme name otherwise, which is far better than registering an
	// icon name nothing can resolve.
	icon := ""
	if Icon(icoApp, icoPx) != nil {
		icon = "udm"
	}

	err := t.Show(icon, items,
		func() { tk.PostEvent(u.restore, false) }, // activate: restore
		func(i int) {
			switch i {
			case 0:
				tk.PostEvent(u.restore, false)
			case 1:
				tk.PostEvent(u.quit, false)
			}
		})
	if err != nil {
		slog.Warn("ui: no tray, close button will quit", "err", err)
		u.app.Tray = nil
		return
	}
	slog.Info("ui: tray up")
}

// build lays out the window. Everything is packed rather than gridded: Tk
// refuses to pack and grid widgets into the same parent, and a vertical
// menubar/toolbar/list/status stack is exactly what pack is good at.
func (u *ui) build() {
	tk.ActivateTheme("azure light")
	tk.App.WmTitle("udm")
	// Sizing a toplevel means wm geometry; a toplevel takes no -padx/-pady and
	// ignores -width/-height.
	tk.WmGeometry(tk.App, "820x460")
	// The window icon is passed positionally: wm iconphoto takes image names,
	// not -image, and passing the wrong shape panics inside the interpreter.
	if app := Icon(icoApp, 64); app != nil {
		tk.App.IconPhoto(app)
	}

	// Closing the window hides it rather than ending the process. That is the
	// whole point of the tray: udm keeps taking downloads whether or not
	// anyone is looking at it.
	tk.WmProtocol(tk.App, "WM_DELETE_WINDOW", u.hide)

	// Columns takes a Tcl list as a string. Passing a []string looks reasonable
	// but formats to "[a b c]", which Tcl reads as a command substitution and
	// leaves the treeview with a single, oddly named column.
	u.tree = tk.TTreeview(
		tk.Columns("name size got pct rate state"),
		tk.Show("headings"),
		tk.Selectmode("extended"),
	)
	for _, c := range []struct {
		col, title string
		w          int
		anchor     string
	}{
		{"name", "Name", 330, "w"},
		{"size", "Size", 90, "e"},
		{"got", "Done", 90, "e"},
		{"pct", "%", 50, "e"},
		{"rate", "Speed", 95, "e"},
		{"state", "State", 80, "e"},
	} {
		u.tree.Heading(c.col, tk.Txt(c.title))
		u.tree.Column(c.col, tk.Width(c.w), tk.Anchor(c.anchor))
	}
	u.tree.TagConfigure("bad", tk.Foreground("#b00020"))

	// The classic label, not ttk: ttk widgets take no -padx/-pady. Padding is a
	// property of the geometry manager, so it goes on the Pack call instead.
	u.bar = tk.Label(tk.Txt(""), tk.Anchor("w"))

	tk.Pack(u.toolbar(), tk.Side("top"), tk.Pady(6))
	tk.Pack(u.bar, tk.Side("bottom"), tk.Fill("x"), tk.Padx(8), tk.Pady(3))
	tk.Pack(u.tree, tk.Side("top"), tk.Fill("both"), tk.Expand(1), tk.Padx(6), tk.Pady(4))

	u.menus()
	u.draw(u.app.Svc.List())
}

// toolbar is one row of big icon buttons. Each acts on the selected rows, or on
// every job when nothing is selected.
func (u *ui) toolbar() *tk.TFrameWidget {
	btn := func(text, icon string, fn func(...string) error) *tk.TButtonWidget {
		return tk.TButton(append(ico(icon, icoPx),
			tk.Txt(text),
			tk.Compound("top"),
			tk.Command(func() { u.act(fn) }),
		)...)
	}
	settings := func(...string) error {
		u.settings()
		return nil
	}

	bar := tk.TFrame()
	for _, b := range []*tk.TButtonWidget{
		btn("Start", icoPlay, u.app.Svc.Start),
		btn("Pause", icoPause, u.app.Svc.Pause),
		btn("Settings", icoGear, settings),
		btn("Remove", icoBin, u.app.Svc.Remove),
		btn("Stop", icoStop, u.app.Svc.Stop),
		btn("Stop All", icoStopAll, u.app.Svc.Stop),
	} {
		// In() names the parent. Without it pack falls back to the default
		// toplevel, which would put the buttons in the main window instead of
		// the toolbar frame. -ipadx/-ipady are what make a ttk button look big;
		// the widget itself accepts no padding options.
		tk.Pack(b, tk.In(bar), tk.Side("left"), tk.Ipadx(10), tk.Ipady(6))
	}
	return bar
}

func (u *ui) menus() {
	bar := tk.Menu()

	file := bar.Menu()
	file.AddCommand(tk.Lbl("Settings..."), tk.Underline(0), tk.Accelerator("Ctrl+,"),
		tk.Command(func() { u.settings() }))
	file.AddSeparator()
	file.AddCommand(tk.Lbl("Exit"), tk.Underline(1), tk.Accelerator("Ctrl+Q"),
		tk.Command(u.quit))
	bar.AddCascade(tk.Lbl("File"), tk.Underline(0), tk.Mnu(file))

	view := bar.Menu()
	view.AddCommand(tk.Lbl("Downloads folder"), tk.Command(u.reveal))
	view.AddCommand(tk.Lbl("Refresh"), tk.Command(func() { u.draw(u.app.Svc.List()) }))
	bar.AddCascade(tk.Lbl("View"), tk.Underline(0), tk.Mnu(view))

	help := bar.Menu()
	help.AddCommand(tk.Lbl("About udm"), tk.Command(func() {
		tk.MessageBox(tk.Icon("info"),
			tk.Msg(fmt.Sprintf("udm — download manager\n\nSaving to:\n%s", u.app.Cfg.Dir)),
			tk.Title("About udm"))
	}))
	bar.AddCascade(tk.Lbl("Help"), tk.Underline(0), tk.Mnu(help))

	tk.App.Configure(tk.Mnu(bar))
	tk.Bind(tk.App, "<Control-q>", tk.Command(u.quit))
	tk.Bind(tk.App, "<Control-comma>", tk.Command(func() { u.settings() }))
}

// act runs fn over the selected rows, or over everything when the selection is
// empty. One code path serves both a single click and Stop All.
func (u *ui) act(fn func(...string) error) {
	ids := u.tree.Selection("get")
	if len(ids) == 0 {
		ids = u.ids
	}
	if len(ids) == 0 {
		return
	}
	if err := fn(ids...); err != nil {
		u.flash(err.Error())
	}
}

// pump redraws whenever the core reports a change. The short settle delay
// coalesces a burst into a single repaint: finishing one download can touch
// five rows within a few milliseconds.
func (u *ui) pump() {
	for {
		<-u.app.Svc.Rev()
		time.Sleep(70 * time.Millisecond)
		jobs := u.app.Svc.List()
		tk.PostEvent(func() { u.draw(jobs) }, true)
	}
}

// draw reconciles the table against the job list: insert what is new, update
// what changed, drop rows whose job is gone. It never rebuilds the table, so
// the selection and scroll position survive a redraw.
func (u *ui) draw(js []core.Job) {
	live := make(map[string]bool, len(js))
	u.ids = u.ids[:0]

	for _, j := range js {
		live[j.ID] = true
		u.ids = append(u.ids, j.ID)

		vals := []string{
			j.Name, human(j.Size), human(j.Got),
			strconvI(j.Pct()), speed(j.Rate), string(j.State),
		}
		// Tags are set outright rather than added, so a row that recovered
		// loses its red.
		tags := []string{}
		if j.Err != "" {
			tags = append(tags, "bad")
		}

		if u.drawn[j.ID] {
			u.tree.Item(j.ID, tk.Values(vals), tk.Tags(tags...))
		} else {
			u.tree.Insert("", "end",
				tk.Id(j.ID), tk.Txt(j.Name), tk.Values(vals), tk.Tags(tags...))
			u.drawn[j.ID] = true
		}
	}

	var gone []any
	for id := range u.drawn {
		if !live[id] {
			gone = append(gone, id)
			delete(u.drawn, id)
		}
	}
	if len(gone) > 0 {
		u.tree.Delete(gone...)
	}

	u.status(js)
}

func (u *ui) status(js []core.Job) {
	if u.note != "" && time.Since(u.since) > 5*time.Second {
		u.note = ""
	}
	var active, done, failed int
	for _, j := range js {
		switch j.State {
		case core.Done:
			done++
		case core.Failed:
			failed++
		}
		if j.Live() {
			active++
		}
	}
	s := fmt.Sprintf("%d job(s)  ·  %d active  ·  %d done  ·  %d failed",
		len(js), active, done, failed)
	if u.note != "" {
		s += "   —   " + u.note
	}
	u.bar.Configure(tk.Txt(s))
}

func (u *ui) flash(msg string) {
	u.note, u.since = msg, time.Now()
	u.status(u.app.Svc.List())
}

// reveal opens the download folder in the desktop's file manager.
func (u *ui) reveal() {
	var cmd string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "explorer"
	default:
		cmd = "xdg-open"
	}
	if err := exec.Command(cmd, u.app.Cfg.Dir).Start(); err != nil {
		u.flash("could not open " + u.app.Cfg.Dir)
	}
}

// hide minimises to the tray, or quits when there is no tray to hide to.
func (u *ui) hide() {
	if u.app.Tray == nil {
		u.quit()
		return
	}
	tk.WmWithdraw(tk.App)
}

// restore brings the window back out of the tray.
func (u *ui) restore() {
	tk.WmDeiconify(tk.App)
	tk.Focus(tk.App)
}

// quit runs the shutdown hook once, then unwinds the Tk loop.
func (u *ui) quit() {
	u.quitOnce.Do(func() {
		if u.before != nil {
			u.before()
		}
	})
	tk.TclAfter(0, "exit")
}

const unit = 1024

func human(n int64) string {
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit && exp < 4; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTP"[exp])
}

func speed(n int64) string {
	if n <= 0 {
		return "—"
	}
	return human(n) + "/s"
}

func strconvI(n int) string { return fmt.Sprintf("%d%%", n) }
