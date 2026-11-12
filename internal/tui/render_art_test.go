package tui

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aklkbqx/wol/internal/termimg"
)

func artModel(mode termimg.Mode, w, h int) *WakeModel {
	m := spotlightModel()
	m.art = newArtCache(mode)
	m.width, m.height = w, h
	return m
}

func TestSpotlightPictureKeepsExactFrame(t *testing.T) {
	for _, mode := range []termimg.Mode{termimg.Blocks, termimg.Kitty} {
		for _, size := range [][2]int{{90, 24}, {100, 28}, {120, 36}, {160, 48}} {
			for sel := range 5 {
				m := artModel(mode, size[0], size[1])
				m.selected = sel
				assertExactFrame(t, m.render(), size[0], size[1])
			}
			m := artModel(mode, size[0], size[1])
			m.waking, m.actionTargetID, m.action = true, "b", "wake-wait"
			m.wakeSentAt = time.Now().Add(-5 * time.Second)
			assertExactFrame(t, m.render(), size[0], size[1])
		}
	}
}

func TestSpotlightKittySendsEachPictureOnce(t *testing.T) {
	m := artModel(termimg.Kitty, 120, 34)
	m.render()
	first := m.art.pending.String()
	if strings.Count(first, "a=T,") < 2 {
		t.Fatalf("first frame should send the scene, the icons and the logo")
	}
	if !strings.Contains(first, "q=2") {
		t.Fatal("transmissions must ask the terminal not to reply")
	}
	m.art.pending.Reset()
	m.render()
	if m.art.pending.Len() != 0 {
		t.Fatal("an unchanged frame must not send pictures again")
	}
	if cmds := m.art.flush(t.Context()); len(cmds) != 0 {
		t.Fatalf("nothing to flush, got %d commands", len(cmds))
	}
	if rel := m.art.release(); strings.Count(rel, "a=d,") != len(strings.Split(first, "a=T,"))-1 {
		t.Fatalf("release should delete every sent picture")
	}
}

func TestArtCacheReusesIDsWhenFull(t *testing.T) {
	c := newArtCache(termimg.Kitty)
	draw := func(w, h int) *image.RGBA { return image.NewRGBA(image.Rect(0, 0, w, h)) }
	for i := range kittyLastID - kittyFirstID + 2 {
		c.lines(artKey{what: 'i', cols: 1, rows: 1, photo: string(rune('a' + i))}, draw)
	}
	if len(c.kitty) != kittyLastID-kittyFirstID+1 {
		t.Fatalf("cache holds %d pictures, want %d", len(c.kitty), kittyLastID-kittyFirstID+1)
	}
	if !strings.Contains(c.pending.String(), "a=d,d=I,q=2,i=16") {
		t.Fatal("the least recently used picture should be deleted")
	}
}

func TestArtCacheKeepsFewStagePictures(t *testing.T) {
	c := newArtCache(termimg.Kitty)
	draw := func(w, h int) *image.RGBA { return image.NewRGBA(image.Rect(0, 0, w, h)) }
	for i := range 20 {
		c.lines(artKey{what: 's', cols: 12 + i, rows: 4}, draw)
		c.lines(artKey{what: 'i', cols: 2, rows: 1, photo: string(rune('a' + i))}, draw)
	}
	large, small := 0, 0
	for key := range c.kitty {
		if key.cols*key.rows >= kittyLargeCell {
			large++
		} else {
			small++
		}
	}
	if large != kittyLargeCap || small != 20 {
		t.Fatalf("kept %d stage pictures and %d icons, want %d and 20", large, small, kittyLargeCap)
	}
	if _, ok := c.kitty[artKey{what: 's', cols: 31, rows: 4}]; !ok {
		t.Fatal("the newest stage picture must stay")
	}
}

func writeTestPhoto(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "machine.png")
	img := image.NewRGBA(image.Rect(0, 0, 60, 40))
	for i := range img.Pix {
		img.Pix[i] = 180
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSpotlightLoadsThePhoto(t *testing.T) {
	m := artModel(termimg.Blocks, 120, 36)
	m.devices[1].ImagePath = writeTestPhoto(t)
	if plain := stripANSI(m.render()); !strings.Contains(plain, "loading picture") {
		t.Fatalf("first frame should say the picture is loading:\n%s", plain)
	}
	cmds := m.art.flush(t.Context())
	if len(cmds) != 1 {
		t.Fatalf("want one photo load, got %d commands", len(cmds))
	}
	msg, ok := cmds[0]().(photoLoadedMsg)
	if !ok || msg.img == nil {
		t.Fatalf("photo did not load: %#v", msg)
	}
	m.update(msg)
	view := m.render()
	assertExactFrame(t, view, 120, 36)
	if plain := stripANSI(view); !strings.Contains(plain, "your picture") || strings.Contains(plain, "loading picture") {
		t.Fatalf("loaded photo should be shown:\n%s", plain)
	}

	m.devices[1].ImagePath = filepath.Join(t.TempDir(), "missing.png")
	m.render()
	m.update(m.art.flush(t.Context())[0]())
	if plain := stripANSI(m.render()); !strings.Contains(plain, "picture could not be read") {
		t.Fatalf("a missing photo should be reported:\n%s", plain)
	}
}

func TestSpotlightHiddenIdentifiersSkipPhotoAndGuess(t *testing.T) {
	m := artModel(termimg.Blocks, 120, 36)
	m.devices[1].ImagePath = writeTestPhoto(t)
	m.redactIdentifiers = true
	plain := stripANSI(m.render())
	if len(m.art.toLoad) != 0 || len(m.art.photos) != 0 {
		t.Fatal("hidden identifiers must not load the photo")
	}
	for _, leak := range []string{"NAS", "your picture", "nas-vault", "198.51.100"} {
		if strings.Contains(plain, leak) {
			t.Fatalf("hidden view shows %q:\n%s", leak, plain)
		}
	}
}

func TestPicturesOffWithoutColour(t *testing.T) {
	m := artModel(termimg.Blocks, 120, 36)
	m.theme = NewTheme(false, false)
	view := m.render()
	if strings.Contains(view, "38;2;") || m.artOn() || len(m.art.blocks) > 0 || m.art.pending.Len() > 0 {
		t.Fatal("no-colour mode must not draw pictures")
	}
}

func TestFormTypeAndPicture(t *testing.T) {
	for in, want := range map[string]string{"": "unknown", "auto": "unknown", "NAS": "nas", "mini pc": "mini"} {
		if got, err := parseFormKind(in); err != nil || got != want {
			t.Errorf("parseFormKind(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := parseFormKind("toaster"); err == nil {
		t.Error("an unknown type should be rejected")
	}
	if formKind("unknown") != "" || formKind("server") != "server" {
		t.Error("formKind should show only known types")
	}
	path := writeTestPhoto(t)
	if got, err := parseFormPicture(" " + path + " "); err != nil || got != path {
		t.Errorf("parseFormPicture = %q, %v", got, err)
	}
	if got, err := parseFormPicture(""); err != nil || got != "" {
		t.Errorf("blank picture = %q, %v", got, err)
	}
	text := filepath.Join(t.TempDir(), "notes.txt")
	os.WriteFile(text, []byte("hello"), 0o600)
	_, err := parseFormPicture(text)
	if err == nil {
		t.Fatal("a text file should be rejected")
	}
	if strings.Contains(err.Error(), text) {
		t.Error("the error must not repeat the path")
	}
}

func BenchmarkRenderSpotlightBlocks(b *testing.B) {
	m := spotlightModel()
	m.art = newArtCache(termimg.Blocks)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.render()
	}
}

func BenchmarkRenderSpotlightKitty(b *testing.B) {
	m := spotlightModel()
	m.art = newArtCache(termimg.Kitty)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.render()
		m.art.pending.Reset()
	}
}
