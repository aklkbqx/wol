package art

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGuessUsesWholeShortWords(t *testing.T) {
	cases := map[string]Kind{
		"nas-vault":   NAS,
		"MacBook-Air": Laptop,
		"render-node": Server,
		"lab-pi":      Mini,
		"pixel-ws":    Desktop, // "pi" only as a whole part
		"living-tv":   Console,
		"studio":      Desktop,
	}
	for name, want := range cases {
		if got := Guess(name); got != want {
			t.Errorf("Guess(%q) = %s, want %s", name, got, want)
		}
	}
}

func TestResolvePrefersStoredType(t *testing.T) {
	if k, manual := Resolve("server", "my-laptop"); k != Server || !manual {
		t.Fatalf("Resolve stored = %s %v", k, manual)
	}
	if k, manual := Resolve("unknown", "my-laptop"); k != Laptop || manual {
		t.Fatalf("Resolve guess = %s %v", k, manual)
	}
	if k, ok := ParseKind("Mini PC"); !ok || k != Mini {
		t.Fatalf("ParseKind(Mini PC) = %s %v", k, ok)
	}
}

func TestPicturesAreOpaqueAndSized(t *testing.T) {
	bg := color.RGBA{R: 8, G: 12, B: 20, A: 255}
	check := func(name string, img *image.RGBA, w, h int) {
		t.Helper()
		if img.Bounds().Dx() != w || img.Bounds().Dy() != h {
			t.Fatalf("%s is %v, want %dx%d", name, img.Bounds(), w, h)
		}
		for i := 3; i < len(img.Pix); i += 4 {
			if img.Pix[i] != 255 {
				t.Fatalf("%s has a transparent pixel", name)
			}
		}
	}
	for _, k := range Kinds() {
		for _, l := range []Light{LightUnknown, LightOff, LightOn, LightWaking, LightWakingDim, LightFault} {
			check("scene "+string(k), Scene(k, l, 30, 20, bg), 30, 20)
			check("icon "+string(k), Icon(k, l, 32, 32, bg), 32, 32)
		}
	}
	check("logo", Logo(32, 32, bg), 32, 32)
	check("tiny", Scene(Desktop, LightOn, 1, 1, bg), 1, 1)
}

func TestSceneShowsTheLight(t *testing.T) {
	bg := color.RGBA{A: 255}
	on := Scene(Desktop, LightOn, 300, 200, bg)
	off := Scene(Desktop, LightOff, 300, 200, bg)
	waking := Scene(Desktop, LightWaking, 300, 200, bg)
	// The tower's power light sits at (267, 112) in design units.
	if c := on.RGBAAt(267, 112); c.G < 150 || c.R > 100 {
		t.Fatalf("awake light = %v, want green", c)
	}
	if c := waking.RGBAAt(267, 112); c.R < 200 || c.B > 100 {
		t.Fatalf("waking light = %v, want amber", c)
	}
	if c := off.RGBAAt(267, 112); c.G > 80 {
		t.Fatalf("asleep light = %v, want dark", c)
	}
}

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestPhotoChecksAndLoads(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "machine.png")
	writePNG(t, good, 1600, 1200)
	if err := CheckPhoto(good); err != nil {
		t.Fatalf("CheckPhoto(good) = %v", err)
	}
	img, err := LoadPhoto(context.Background(), good)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 640 || b.Dy() != 480 {
		t.Fatalf("loaded photo is %v, want it reduced to 640x480", b)
	}

	text := filepath.Join(dir, "notes.txt")
	os.WriteFile(text, []byte("not a picture"), 0o600)
	if err := CheckPhoto(text); err != ErrPhotoFormat {
		t.Fatalf("CheckPhoto(text) = %v, want ErrPhotoFormat", err)
	}
	if err := CheckPhoto(filepath.Join(dir, "missing.png")); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("CheckPhoto(missing) = %v", err)
	}
	if err := CheckPhoto(dir); err == nil {
		t.Fatal("CheckPhoto(directory) should fail")
	}
	big := filepath.Join(dir, "big.png")
	f, _ := os.Create(big)
	f.Truncate(MaxPhotoBytes + 1)
	f.Close()
	if err := CheckPhoto(big); err == nil || !strings.Contains(err.Error(), "MB") {
		t.Fatalf("CheckPhoto(big) = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := LoadPhoto(ctx, good); err == nil {
		t.Fatal("LoadPhoto with a cancelled context should fail")
	}
}

func TestFitAndShade(t *testing.T) {
	bg := color.RGBA{R: 1, G: 2, B: 3, A: 255}
	src := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for i := range src.Pix {
		src.Pix[i] = 220
	}
	fit := Fit(src, 60, 30, bg)
	if c := fit.RGBAAt(2, 15); c != bg {
		t.Fatalf("letterbox = %v, want background", c)
	}
	if c := fit.RGBAAt(30, 15); c.R != 220 {
		t.Fatalf("centre = %v, want the photo", c)
	}
	if Shade(fit, LightOn) != fit {
		t.Fatal("an awake photo should be unchanged")
	}
	if c := Shade(fit, LightOff).RGBAAt(30, 15); c.R >= 220 {
		t.Fatalf("asleep photo = %v, want darker", c)
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	if got := ExpandPath(" ~/pictures/a.png "); got != filepath.Join(home, "pictures", "a.png") {
		t.Fatalf("ExpandPath = %q", got)
	}
	if got := ExpandPath(""); got != "" {
		t.Fatalf("ExpandPath(empty) = %q", got)
	}
}

func BenchmarkScene(b *testing.B) {
	bg := color.RGBA{A: 255}
	for b.Loop() {
		Scene(Desktop, LightOn, 384, 256, bg)
	}
}
