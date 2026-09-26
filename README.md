# udm

A download manager for Firefox. The browser hands every download over to a small
local app; the app fetches it, shows progress, and lives in the system tray until
you tell it to quit.

```
  Firefox tab
      │  Content-Disposition: attachment
      ▼
  extension/            cancel the browser's own fetch, POST the url
      │  http://127.0.0.1:33210
      ▼
  udm (Go)              window + tray, grab does the transfer
      │
      ▼
  ~/Downloads
```

The extension talks **only** HTTP to the app. The native messaging host exists
solely because a browser cannot start a process: when nothing is listening, the
extension calls the host, the host starts `udm`, waits for the port, and hands
the download over.

## Build

Requires Go 1.27 and a Linux desktop with X11 (Tk 9.0, cgo-free).

```sh
go mod tidy     # first time only: go.sum is not committed
just run        # or: go run ./cmd/udm
```

## Install the browser side

```sh
go install github.com/raffleberry/udm/cmd/udm   # so the host can find the app
just install-host
```

`just install-host` compiles the native messaging host into `~/.config/udm/`,
writes Firefox's manifest to `~/.mozilla/native-messaging-hosts/`, and installs
`udm.png` plus an `index.theme` into the hicolor icon theme so the tray icon
resolves by name. If a `udm` binary is on your `PATH` it is copied next to the
host, which is what lets the host start the app on demand.

Then load `extension/` as a temporary add-on: `about:debugging` → This Firefox
→ Load Temporary Add-on → pick `extension/manifest.json`.

## The window

Closing the window hides it to the tray; it keeps downloading. Bring it back by
clicking the tray icon, or by picking **Show udm** from its menu. **File → Exit**
and the tray's **Exit** are the only ways to actually quit.

The toolbar acts on the rows you select, or on every job when nothing is
selected — so *Stop All* is just *Stop* with an empty selection.

| Button | Icon | Does |
| --- | --- | --- |
| Start | `play.png` | run, or resume from the partial file |
| Pause | `pause.png` | stop, keep the partial file |
| Settings | `gear.png` | download folder, concurrency |
| Remove | `wastebasket.png` | stop and forget the job |
| Stop | `stop.png` | stop and discard the partial file |
| Stop All | `red_square.png` | stop everything |

Icons load from `resources/` at runtime, falling back through
`/usr/share/udm/icons` and `~/.local/share/udm/icons`. A missing icon gives a
text-only button rather than an error, so the toolbar works before any artwork
is in place.

## Settings

`~/.config/udm/settings.json`:

```json
{
  "dir": "/home/you/Downloads",
  "port": 33210,
  "max_conc": 3
}
```

`port` is the one number three things have to agree on: this file,
`internal/cfg.Port`, and `API` in `extension/background.js`. Change it in the
file if you must — the app and the native host both read this file, so they stay
in step; the extension's constant is the only one you have to edit by hand.

## Swapping the download engine

`internal/dl` is the seam. It is four methods wide:

```go
type Downloader interface{ Start(Spec) (Transfer, error) }
type Transfer interface {
    Progress() (got, size, rate int64)
    Done() <-chan struct{}
    Err() error
    Pause() error
    Cancel() error
}
```

`internal/dl/grab` is the implementation in use. To add another engine, write a
package with that signature and change the one line in `cmd/udm/main.go` that
calls `dlgrab.New()`. Nothing in `core`, `api` or `ui` imports grab.

Pause is the one thing to think about: grab has no pause, so the adapter
implements it as "cancel, keep the bytes on disk" and relies on grab's Range
resume. An engine that pauses natively can do better.

## Layout

| Path | What it owns |
| --- | --- |
| `cmd/udm` | the one binary; the only place engines are chosen |
| `internal/core` | jobs, states, the service. No grab, no HTTP, no Tk |
| `internal/dl` | the `Downloader` port, and `dl/grab` the adapter |
| `internal/api` | loopback HTTP for the extension and the host |
| `internal/ui` | the Tk window |
| `internal/tray` | `StatusNotifierItem` over D-Bus (linux) |
| `internal/host` | the native messaging host and its installer |
| `internal/cfg` | settings.json and the port |
| `extension` | the Firefox add-on: plain JS, no build step |

`internal/core/ports.go` holds the interfaces the GUI and the API consume. They
are declared where they are used, each as small as its caller needs, and
`*core.Service` is the only implementation.
