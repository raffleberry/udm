package ui

import (
	"log/slog"
	"strconv"

	tk "modernc.org/tk9.0"
)

// settings opens the preferences dialog.
//
// The API port is deliberately not editable here: the extension and the native
// host both have to agree on it, so it lives in settings.json where changing it
// is an explicit act rather than something a stray keystroke can desync.
// It returns the two entries so a test can assert they round-trip; the
// production caller ignores them.
func (u *ui) settings() (dir, conc *tk.TEntryWidget) {
	// ttk::toplevel takes no -title, -padx or -pady. The title is a wm setting
	// and the margins come from the grid calls below.
	win := tk.Toplevel()
	win.WmTitle("udm — Settings")

	// Bind the dialog to the main window so it floats with it. wm transient
	// returns the container path, so it cannot be nested in the Toplevel call.
	tk.WmTransient(win, tk.App)

	// Textvariable is a value, not a binding: the first call on a widget
	// creates the Tcl variable and sets it, later calls update it, and the
	// getter reads the entry back. Note it is *not* tk.Variable, which emits
	// -variable and is for check/radio/scale only; ttk::entry has no such
	// option and rejects it.
	dir = win.TEntry(tk.Width(52), tk.Textvariable(u.app.Cfg.Dir))
	conc = win.TEntry(tk.Width(6), tk.Justify("right"),
		tk.Textvariable(strconv.Itoa(u.app.Cfg.MaxConc)))

	save := func() {
		c := *u.app.Cfg
		c.Dir = dir.Textvariable()
		if n, err := strconv.Atoi(conc.Textvariable()); err == nil && n > 0 {
			c.MaxConc = n
		}
		if c.Dir == "" {
			c.Dir = u.app.Cfg.Dir
		}
		if err := u.app.Save(&c); err != nil {
			slog.Error("ui: save settings", "err", err)
			tk.MessageBox(tk.Icon("error"), tk.Msg(err.Error()), tk.Title("Cannot save"))
			return
		}
		*u.app.Cfg = c
		tk.Destroy(win)
		u.flash("saved — restart to apply the new concurrency limit")
	}

	// Every widget is built with the win.TLabel / win.TEntry method form, not
	// the package-level constructor. The method form parents the widget to the
	// dialog; the package form parents it to App, and a later -in cannot move
	// it ("can't put ... inside ...").
	tk.Grid(win.TLabel(tk.Txt("Save to")), tk.Row(0), tk.Column(0), tk.Sticky("w"), tk.Pady(4))
	tk.Grid(dir, tk.Row(0), tk.Column(1), tk.Sticky("we"), tk.Pady(4))
	tk.Grid(win.TButton(tk.Txt("Browse..."), tk.Command(func() {
		if d := tk.ChooseDirectory(tk.Initialdir(dir.Textvariable()), tk.Title("Choose a folder")); d != "" {
			dir.Configure(tk.Textvariable(d))
		}
	})), tk.Row(0), tk.Column(2), tk.Padx(8))

	tk.Grid(win.TLabel(tk.Txt("Simultaneous downloads")),
		tk.Row(1), tk.Column(0), tk.Sticky("w"), tk.Pady(4))
	tk.Grid(conc, tk.Row(1), tk.Column(1), tk.Sticky("w"), tk.Pady(4))

	tk.Grid(win.TLabel(tk.Txt("API port "+strconv.Itoa(u.app.Cfg.Port)+" — edit settings.json")),
		tk.Row(2), tk.Columnspan(3), tk.Sticky("w"), tk.Pady(10))

	tk.Grid(win.TButton(tk.Txt("Cancel"), tk.Command(func() { tk.Destroy(win) })),
		tk.Row(3), tk.Column(0), tk.Pady(14))
	tk.Grid(win.TButton(tk.Txt("Save"), tk.Command(save)),
		tk.Row(3), tk.Column(1), tk.Pady(14))

	tk.Bind(win, "<Escape>", tk.Command(func() { tk.Destroy(win) }))
	tk.Bind(win, "<Return>", tk.Command(save))
	tk.GridColumnConfigure(win, 1, tk.Weight(1))
	tk.Focus(win)
	return dir, conc
}
