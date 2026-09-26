# AGENTS.md

Download manager: a Go app (`udm`) fetches files with a pluggable engine, and a
plain-JS Firefox extension hands it every download over HTTP on loopback.

## Architecture

```
popup ──sendMessage──▶ background.js ──POST /api/add──▶ udm ──▶ dl.Downloader ──▶ ~/Downloads
                          │                                ▲
                          └──native msg──▶ internal/host ─┘   (only to start udm lazily)
```

- `internal/core` — the domain. `Job`, `State`, `Service`. Imports nothing but
  `internal/dl`. Knows nothing about grab, HTTP, Tk or D-Bus.
- `internal/dl` — the engine port. `Downloader`/`Transfer` in `dl.go`;
  `dl/grab` is the only implementation.
- `internal/api` — loopback HTTP, the only channel the extension and the host use.
- `internal/ui` — the Tk window (adapter over `core`).
- `internal/tray` — `StatusNotifierItem` over D-Bus. Linux only; `New()` returns
  nil elsewhere and the window then quits on close instead of hiding.
- `internal/host` — the native messaging host: a lazy launcher, nothing more.
- `internal/cfg` — `settings.json` and the port.
- `cmd/udm` — the single binary. **The only place a download engine is chosen.**
- `extension/` — the add-on. No build step: `about:debugging` loads
  `extension/manifest.json` directly.

## Commands

```sh
just run             # go run ./cmd/udm
just check           # tidy -> vet -> test -> build (incl. GOOS=windows)
just install-host    # compile + register the native host with Firefox
just port            # print the port the extension and host agree on
go test ./internal/... -run TestPause -v
```

`go.sum` is committed. Run `go mod tidy` after changing `go.mod`, not before
the first build.

`just install-host` runs from `internal/host` on purpose: the installer resolves
`cmd/host/main.go` and `resources/udm.png` relative to the process cwd.

## The port must agree in three places

`internal/cfg.Port` (= 33210), `API` in `extension/background.js`, and `port` in
`~/.config/udm/settings.json`. The app and the native host both *read*
settings.json, so those two stay in step automatically; the extension constant is
the only one edited by hand. `just port` prints it.

## The extension id is pinned in two places

`udm@raffleberry` in `extension/manifest.json` (`browser_specific_settings.gecko.id`)
must equal `allowed_extensions` in the manifest the installer writes
(`internal/host/install.go`). Firefox refuses to start the host otherwise.

## Interception: one listener, on purpose

The only `webRequest` listener is `onHeadersReceived`. Cancelling in
`onBeforeRequest` instead would cancel every GET in the browser — pages, images,
XHR included — so the decision waits until the response headers say the body is a
file. `isDownload` is deliberately conservative; guessing wrong breaks a website.

## Tk 9.0 (`modernc.org/tk9.0`, package name `tk9_0`)

Imported as `tk "modernc.org/tk9.0"`. Every item below was found by running the
app, not by reading docs — ttk rejects a bad option by **panicking inside the
interpreter**, and a compile check cannot see any of it. `internal/ui/ui_test.go`
builds every widget against a real display so these stay fixed.

- **`tk.Text` is the text widget, not `-text`.** The option is `tk.Txt`.
  `tk.Label` is likewise the label widget, not an option.
- **`Columns` takes a Tcl list as a string**: `Columns("a b c")`. A `[]string`
  formats to `[a b c]`, which Tcl reads as a *command substitution*, leaving one
  column literally named `[a b c]`. `Values` and `Tags` do handle `[]string`.
- **Padding belongs to the geometry manager.** ttk widgets take no `-padx`/
  `-pady`; put them on the `Pack`/`Grid` call. Use `Ipadx`/`Ipady` on `Pack` to
  make a ttk button look big. The classic `Label` does accept `-padx`/`-pady`.
- **Parent widgets with the method form**: `win.TLabel(...)`, not `tk.TLabel(...)`.
  The package form parents to `App`; `tk.In(win)` cannot reparent afterwards
  ("can't put ... inside ...").
- **`ttk::toplevel` takes no `-title`** — use `win.WmTitle(...)`. Size a toplevel
  with `tk.WmGeometry(tk.App, "820x460")`; `-width`/`-height` on `Configure` are
  ignored.
- **`Textvariable(s)` is a value setter, and `Textvariable()` is the getter.**
  The first call creates the Tcl variable and sets it. It is *not* `tk.Variable`,
  which emits `-variable` for check/radio/scale and is rejected by `ttk::entry`.
- **`wm iconphoto` takes the image positionally**: `App.IconPhoto(img)`, not
  `App.IconPhoto(tk.Image(img))`. `-image` is a panic there.
- `NewPhoto(File(p), Width(px), Height(px))` does scale a PNG; `-subsample` is
  rejected at creation time.
- Tk is **not thread-safe**: marshal with `tk.PostEvent(f, canDrop)`. The redraw
  pump and the tray callbacks both go through it.
- The main loop is `tk.App.Wait()`; quit with `tk.TclAfter(0, "exit")`, never
  `os.Exit`, so the deferred cleanup in `cmd/udm` runs.
- `tk.WmProtocol(tk.App, "WM_DELETE_WINDOW", u.hide)` is what makes the close
  button minimise instead of quit.

## Icons are loaded from disk, not embedded

`internal/ui/icons.go` searches `resources/`, then `~/.local/share/udm/icons`,
`/usr/share/udm/icons`, then `~/.local/share/icons/hicolor/256x256/apps`. A
missing file yields **no image option**, so the button still works as text.
That is deliberate: the artwork is expected to be swapped, and re-embedding
stale bytes would fight that.

`just install-host` copies `resources/udm.png` into the hicolor apps dir, writes
`index.theme` if one is missing, and refreshes the icon cache. All three matter:
without the first the tray's `IconName: udm` cannot resolve, and without the
other two a machine with no local hicolor override ignores the directory
entirely — the tray registers and the panel shows nothing.

## Testing

- `internal/core/service_test.go` drives a fake `dl.Downloader` by hand — no
  network, no disk. It is the place to add tests for job state, resume and the
  concurrency cap.
- `internal/api/api_test.go` drives the routes against a stub. It pins the
  **origin guard** (a web page must not be able to drive udm) and the rule that
  an empty `ids` list means every job — the Stop All button.
- `internal/ui/ui_test.go` builds every widget against a real display and
  **skips without one**. This is the only thing standing between you and a
  mistyped Tk option panicking at startup, which no compile check catches. Run
  it after touching anything in `internal/ui`.
- The tray has no automated coverage; it needs a session bus. Verify it by hand:
  `gdbus call --session --dest org.kde.StatusNotifierWatcher --object-path
  /StatusNotifierWatcher --method org.freedesktop.DBus.Properties.Get
  org.kde.StatusNotifierWatcher RegisteredStatusNotifierItems`
- `go test -race` does not work here: it needs cgo and there is no gcc. The
  project is deliberately cgo-free, so this is expected, not a broken setup.

## Invariants worth not breaking

- `core.Service.s.mu` guards the job slice, the byID index and the rev channel.
  The `start`/`pause`/`stop`/`remove` helpers are called **with it already
  held**. It is never held across a downloader call.
- `Job.State` is the operator's *intent*. A transfer settling checks it: if it
  says `paused` or `stopped`, the engine's cancel error is expected and must not
  be recorded as a failure. This is why grab's `Pause` is "cancel, keep the bytes".
- Each start attempt gets a fresh `kill` channel, and `run` checks it is still
  current before adopting a transfer. That is what stops a superseded attempt
  from overwriting the state of the one that replaced it.
- `internal/host.start` leaves the child's stdio nil. The browser is holding the
  host's stdout; a child that inherited it would stall the download forever.

## Known gaps

- `dl.Spec` carries only a URL and a path, no request headers. `grab` therefore
  sends no `Referer` or `Cookie`, and sites that require one will answer 403.
  The extension already captures the referrer and the API already accepts it; the
  seam to widen is `dl.Spec` plus `grab.NewRequest`'s `HTTPRequest`.
- `Stop` cancels the transfer but does not delete the partial file. grab cleans
  up its own on error, so in practice nothing is left, but `dl.Transfer.Cancel`
  documents the discard and the adapter leans on grab to honour it.
- No resume persistence across restarts: the job list is in memory only. The old
  sqlite store is gone, and nothing reads it yet.
- The API has no auth beyond the origin check. Binding to loopback plus refusing
  foreign origins is enough against a browser; it is not enough against another
  local process that chooses not to send an `Origin` header.
