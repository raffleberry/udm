package ui

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"

	tk "modernc.org/tk9.0"
)

// Icon names, matching the files in resources/ (minus the .png).
const (
	icoPlay    = "play"
	icoPause   = "pause"
	icoGear    = "gear"
	icoBin     = "wastebasket"
	icoStop    = "stop"
	icoStopAll = "red_square"
	icoApp     = "udm"
)

// iconDirs is where Icon looks, in order. The relative entries come first so a
// `go run` from a checkout works with no setup; the rest are the places the
// installer and a system install put artwork. The hicolor entry is what makes
// the tray's "udm" icon name resolve to the same file.
var iconDirs = []string{
	"resources",
	filepath.Join("..", "..", "resources"),
	filepath.Join(os.Getenv("HOME"), ".local", "share", "udm", "icons"),
	filepath.Join("/usr", "share", "udm", "icons"),
	filepath.Join(os.Getenv("HOME"), ".local", "share", "icons", "hicolor", "256x256", "apps"),
}

var (
	mu    sync.Mutex
	cache = map[string]*tk.Img{}
)

// Icon returns a Tk photo for the named icon scaled to px, or nil when the file
// is not installed. Callers must cope with nil: a missing icon degrades to a
// text-only button rather than crashing, so the toolbar is usable before any
// artwork is dropped in.
func Icon(name string, px int) *tk.Img {
	mu.Lock()
	defer mu.Unlock()
	if img, ok := cache[name+"@"+strconv.Itoa(px)]; ok {
		return img
	}
	for _, dir := range iconDirs {
		p := filepath.Join(dir, name+".png")
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			continue
		}
		img := tk.NewPhoto(tk.File(p), tk.Width(px), tk.Height(px))
		cache[name+"@"+strconv.Itoa(px)] = img
		return img
	}
	return nil
}

// ico returns the options to hand a button so it shows the named icon. A
// missing file yields no image option at all rather than a nil one, so the
// button still works, just without artwork.
func ico(name string, px int) []tk.Opt {
	if img := Icon(name, px); img != nil {
		return []tk.Opt{tk.Image(img)}
	}
	return nil
}

// icoPx is the toolbar icon size in pixels. The artwork in resources/ is 256px,
// so it is scaled down; Tk's -width/-height on a photo image do the scaling,
// and -subsample is not accepted at creation time.
const icoPx = 32
