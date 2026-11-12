package tui

import (
	"context"
	"image"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/aklkbqx/wol/internal/art"
	"github.com/aklkbqx/wol/internal/store"
	"github.com/aklkbqx/wol/internal/termimg"
)

// Picture sizes. Kitty pictures are drawn at kittyCellW × kittyCellH pixels
// per cell so they stay sharp on high-density screens; half-block pictures
// have one pixel across and two down per cell.
const (
	kittyCellW     = 16
	kittyCellH     = 32
	kittyFirstID   = 16
	kittyLastID    = 255
	artBlockCap    = 96 // cached half-block pictures before the cache resets
	artPhotoCap    = 16 // decoded photos kept in memory
	kittyLargeCap  = 8  // stage-sized pictures kept in the terminal
	kittyLargeCell = 16 // pictures of at least this many cells count as large
	photoLoadLimit = 5 * time.Second
)

// artKey names one drawn picture.
type artKey struct {
	what       byte // 's' room scene, 'p' photo, 'i' list icon, 'l' logo
	kind       art.Kind
	light      art.Light
	photo      string
	cols, rows int
	bg         color.RGBA
}

type kittyImage struct {
	id   uint8
	used uint64
}

type photoEntry struct {
	img     *image.RGBA
	failed  bool
	loading bool
}

// artCache draws pictures once and keeps the terminal output for each. It
// is owned by the Bubble Tea goroutine: render fills it, Update drains it.
type artCache struct {
	mode    termimg.Mode
	blocks  map[artKey][]string
	kitty   map[artKey]kittyImage
	clock   uint64
	pending strings.Builder // kitty transmissions not yet written
	photos  map[string]*photoEntry
	toLoad  []string
}

func newArtCache(mode termimg.Mode) *artCache {
	return &artCache{
		mode:   mode,
		blocks: make(map[artKey][]string),
		kitty:  make(map[artKey]kittyImage),
		photos: make(map[string]*photoEntry),
	}
}

// photoLoadedMsg carries a decoded machine photo back to the model.
type photoLoadedMsg struct {
	path string
	img  *image.RGBA
}

func (m *WakeModel) artOn() bool {
	return m.art != nil && m.art.mode != termimg.None && m.theme.Colors && !m.theme.ASCII
}

func (m *WakeModel) artKitty() bool {
	return m.artOn() && m.art.mode == termimg.Kitty
}

// lines returns the terminal cells for a picture cols × rows cells big,
// drawing it with draw (given the pixel size) the first time it is needed.
func (c *artCache) lines(key artKey, draw func(w, h int) *image.RGBA) []string {
	switch c.mode {
	case termimg.Blocks:
		if lines, ok := c.blocks[key]; ok {
			return lines
		}
		if len(c.blocks) >= artBlockCap {
			clear(c.blocks)
		}
		lines := termimg.BlockLines(draw(key.cols, key.rows*2))
		c.blocks[key] = lines
		return lines
	case termimg.Kitty:
		c.clock++
		if img, ok := c.kitty[key]; ok {
			img.used = c.clock
			c.kitty[key] = img
			return c.placeholder(key, img.id)
		}
		data, err := termimg.EncodePNG(draw(key.cols*kittyCellW, key.rows*kittyCellH))
		if err != nil {
			return nil
		}
		if key.cols*key.rows >= kittyLargeCell {
			c.trimLarge()
		}
		id := c.freeID()
		c.kitty[key] = kittyImage{id: id, used: c.clock}
		c.pending.WriteString(termimg.Transmit(id, data, key.cols, key.rows))
		return c.placeholder(key, id)
	}
	return nil
}

func (c *artCache) placeholder(key artKey, id uint8) []string {
	// Placeholder cells are cheap to build and depend only on the id and
	// size, so they share the block cache under a distinct key.
	pk := key
	pk.what |= 0x80
	pk.light = art.Light(id)
	if lines, ok := c.blocks[pk]; ok {
		return lines
	}
	if len(c.blocks) >= artBlockCap {
		clear(c.blocks)
	}
	lines := termimg.PlaceholderLines(id, key.cols, key.rows)
	c.blocks[pk] = lines
	return lines
}

// freeID returns an unused kitty image id, deleting the least recently
// used picture when every id is taken.
func (c *artCache) freeID() uint8 {
	taken := make(map[uint8]bool, len(c.kitty))
	for _, img := range c.kitty {
		taken[img.id] = true
	}
	for id := kittyFirstID; id <= kittyLastID; id++ {
		if !taken[uint8(id)] {
			return uint8(id)
		}
	}
	var oldest artKey
	var oldestUse uint64
	first := true
	for key, img := range c.kitty {
		if first || img.used < oldestUse {
			oldest, oldestUse, first = key, img.used, false
		}
	}
	id := c.kitty[oldest].id
	delete(c.kitty, oldest)
	c.pending.WriteString(termimg.Delete(id))
	return id
}

// trimLarge deletes the least recently used stage pictures so that, with
// the one about to be sent, at most kittyLargeCap stay in the terminal.
// Each is up to a few megabytes there, and every state and window size
// draws a new one.
func (c *artCache) trimLarge() {
	for {
		count := 0
		var oldest artKey
		var oldestUse uint64
		for key, img := range c.kitty {
			if key.cols*key.rows < kittyLargeCell {
				continue
			}
			if count == 0 || img.used < oldestUse {
				oldest, oldestUse = key, img.used
			}
			count++
		}
		if count < kittyLargeCap {
			return
		}
		c.pending.WriteString(termimg.Delete(c.kitty[oldest].id))
		delete(c.kitty, oldest)
	}
}

// photo returns a decoded photo, or nil while it loads or when it cannot
// be read. A first request queues the load.
func (c *artCache) photo(path string) (*image.RGBA, bool) {
	entry, ok := c.photos[path]
	if !ok {
		if len(c.photos) >= artPhotoCap {
			for p, e := range c.photos {
				if !e.loading {
					delete(c.photos, p)
				}
			}
		}
		c.photos[path] = &photoEntry{loading: true}
		c.toLoad = append(c.toLoad, path)
		return nil, false
	}
	return entry.img, entry.failed
}

// flush returns the commands that write pending pictures and load queued
// photos.
func (c *artCache) flush(ctx context.Context) []tea.Cmd {
	var cmds []tea.Cmd
	if c.pending.Len() > 0 {
		cmds = append(cmds, tea.Raw(c.pending.String()))
		c.pending.Reset()
	}
	for _, path := range c.toLoad {
		cmds = append(cmds, loadPhotoCmd(ctx, path))
	}
	c.toLoad = c.toLoad[:0]
	return cmds
}

func loadPhotoCmd(parent context.Context, path string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, photoLoadLimit)
		defer cancel()
		img, err := art.LoadPhoto(ctx, path)
		if err != nil {
			return photoLoadedMsg{path: path}
		}
		return photoLoadedMsg{path: path, img: img}
	}
}

func (c *artCache) photoLoaded(msg photoLoadedMsg) {
	entry, ok := c.photos[msg.path]
	if !ok {
		entry = &photoEntry{}
		c.photos[msg.path] = entry
	}
	entry.loading = false
	entry.img = msg.img
	entry.failed = msg.img == nil
}

// release returns the escape sequences that free every kitty picture.
func (c *artCache) release() string {
	if c == nil || c.mode != termimg.Kitty {
		return ""
	}
	var sb strings.Builder
	for _, img := range c.kitty {
		sb.WriteString(termimg.Delete(img.id))
	}
	clear(c.kitty)
	return sb.String()
}

// artLight is what the picture shows for a machine's state.
func (m *WakeModel) artLight(device store.Device, now time.Time, pulse bool) art.Light {
	if m.waking && device.ID == m.actionTargetID {
		if pulse && m.motion.Enabled && now.Unix()%2 == 1 {
			return art.LightWakingDim
		}
		return art.LightWaking
	}
	switch m.deviceState(device) {
	case "ONLINE":
		return art.LightOn
	case "OFFLINE":
		return art.LightOff
	case "DISABLED", "BLOCKED", "FAILED":
		return art.LightFault
	default:
		return art.LightUnknown
	}
}

// artKind is the machine type to draw. With identifiers hidden, the name
// is not used for a guess.
func (m *WakeModel) artKind(device store.Device) (art.Kind, bool) {
	if m.redactIdentifiers {
		if k, ok := art.ParseKind(device.DeviceType); ok {
			return k, true
		}
		return art.Desktop, false
	}
	return art.Resolve(device.DeviceType, device.Name)
}

// stagePicture returns the stage picture for a machine: its photo when one
// is set and readable, otherwise the room scene. note describes the photo
// when it could not be shown.
func (m *WakeModel) stagePicture(device store.Device, cols, rows int, now time.Time) (lines []string, photo bool, note string) {
	bg := art.RGBA(m.theme.Palette.Ink)
	light := m.artLight(device, now, true)
	kind, _ := m.artKind(device)
	if path := strings.TrimSpace(device.ImagePath); path != "" && !m.redactIdentifiers {
		img, failed := m.art.photo(path)
		switch {
		case img != nil:
			key := artKey{what: 'p', light: light, photo: path, cols: cols, rows: rows, bg: bg}
			return m.art.lines(key, func(w, h int) *image.RGBA {
				return art.Shade(art.Fit(img, w, h, bg), light)
			}), true, ""
		case failed:
			note = "picture could not be read"
		default:
			note = "loading picture"
		}
	}
	key := artKey{what: 's', kind: kind, light: light, cols: cols, rows: rows, bg: bg}
	return m.art.lines(key, func(w, h int) *image.RGBA {
		return art.Scene(kind, light, w, h, bg)
	}), false, note
}

// listIcon returns the 2 × 1 cell icon for a list row (kitty only).
func (m *WakeModel) listIcon(device store.Device, bg color.Color) string {
	kind, _ := m.artKind(device)
	light := m.artLight(device, time.Time{}, false)
	key := artKey{what: 'i', kind: kind, light: light, cols: 2, rows: 1, bg: art.RGBA(bg)}
	lines := m.art.lines(key, func(w, h int) *image.RGBA {
		return art.Icon(kind, light, w, h, key.bg)
	})
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}

// logoMark returns the 2 × 1 cell power logo (kitty only).
func (m *WakeModel) logoMark(bg color.Color) string {
	key := artKey{what: 'l', cols: 2, rows: 1, bg: art.RGBA(bg)}
	lines := m.art.lines(key, func(w, h int) *image.RGBA {
		return art.Logo(w, h, key.bg)
	})
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}

// withArt runs after every update: it draws the coming frame once so any
// new kitty picture is sent before or with it, and starts photo loads.
func (m *WakeModel) withArt(cmd tea.Cmd) tea.Cmd {
	if !m.artOn() {
		return cmd
	}
	if m.phase == phaseReady && !m.opening && m.spotlightEligible(m.width, m.height) {
		m.renderSpotlight(m.width, m.height)
	}
	extra := m.art.flush(m.baseContext())
	if len(extra) == 0 {
		return cmd
	}
	return tea.Batch(append([]tea.Cmd{cmd}, extra...)...)
}
