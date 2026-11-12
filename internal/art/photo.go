package art

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif" // GIF photos
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Photo limits keep a bad file from using a lot of memory.
const (
	MaxPhotoBytes  = 20 << 20 // 20 MB on disk
	MaxPhotoPixels = 40e6     // 40 megapixels decoded
	photoKeep      = 640      // longest side kept in memory after loading
)

// ErrPhotoFormat means the file is not a PNG, JPEG or GIF image.
var ErrPhotoFormat = errors.New("use a PNG, JPEG or GIF file")

// ExpandPath turns "~/x" into an absolute path under the home directory.
func ExpandPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	if path != "" {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}
	return path
}

// CheckPhoto confirms that path is a readable image within the limits
// without decoding the pixels.
func CheckPhoto(path string) error {
	f, err := openPhoto(path)
	if err != nil {
		return err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(bufio.NewReader(io.LimitReader(f, MaxPhotoBytes)))
	if err != nil {
		return ErrPhotoFormat
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || float64(cfg.Width)*float64(cfg.Height) > MaxPhotoPixels {
		return fmt.Errorf("image is larger than %d megapixels", int(MaxPhotoPixels/1e6))
	}
	return nil
}

// LoadPhoto reads and decodes a machine photo, reduced so its longest side
// is at most 640 pixels.
func LoadPhoto(ctx context.Context, path string) (*image.RGBA, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := CheckPhoto(path); err != nil {
		return nil, err
	}
	data, err := readLimited(path)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrPhotoFormat
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > photoKeep || h > photoKeep {
		if w >= h {
			w, h = photoKeep, max(1, h*photoKeep/b.Dx())
		} else {
			w, h = max(1, w*photoKeep/b.Dy()), photoKeep
		}
	}
	return Resize(img, w, h), nil
}

// openPhoto opens a regular file no larger than MaxPhotoBytes.
func openPhoto(path string) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errors.New("file not found")
		}
		return nil, errors.New("file cannot be read")
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, errors.New("is not a regular file")
	}
	if info.Size() > MaxPhotoBytes {
		f.Close()
		return nil, fmt.Errorf("file is larger than %d MB", MaxPhotoBytes>>20)
	}
	return f, nil
}

func readLimited(path string) ([]byte, error) {
	f, err := openPhoto(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxPhotoBytes+1))
	if err != nil || len(data) > MaxPhotoBytes {
		return nil, errors.New("file cannot be read")
	}
	return data, nil
}

// Resize scales src to exactly w × h pixels by averaging the source area
// under each target pixel.
func Resize(src image.Image, w, h int) *image.RGBA {
	w, h = max(1, w), max(1, h)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	b := src.Bounds()
	sw, sh := float64(b.Dx()), float64(b.Dy())
	for y := 0; y < h; y++ {
		y0 := b.Min.Y + int(float64(y)*sh/float64(h))
		y1 := max(y0+1, b.Min.Y+int(float64(y+1)*sh/float64(h)))
		for x := 0; x < w; x++ {
			x0 := b.Min.X + int(float64(x)*sw/float64(w))
			x1 := max(x0+1, b.Min.X+int(float64(x+1)*sw/float64(w)))
			var r, g, bl, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					// RGBA is premultiplied, so transparent areas average
					// as black, which matches the dark stage.
					cr, cg, cb, _ := src.At(sx, sy).RGBA()
					r, g, bl, n = r+uint64(cr>>8), g+uint64(cg>>8), bl+uint64(cb>>8), n+1
				}
			}
			i := dst.PixOffset(x, y)
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = uint8(r/n), uint8(g/n), uint8(bl/n), 255
		}
	}
	return dst
}

// Fit places src inside a w × h picture on bg, keeping its proportions and
// rounding the corners like the room picture.
func Fit(src image.Image, w, h int, bg color.RGBA) *image.RGBA {
	b := src.Bounds()
	fw, fh := w, h
	if b.Dx()*h > b.Dy()*w {
		fh = max(1, b.Dy()*w/b.Dx())
	} else {
		fw = max(1, b.Dx()*h/b.Dy())
	}
	scaled := Resize(src, fw, fh)
	c := newCanvas(w, h, float64(w), float64(h), bg)
	ox, oy := (w-fw)/2, (h-fh)/2
	r := float64(min(fw, fh)) * 0.06
	for y := 0; y < fh; y++ {
		for x := 0; x < fw; x++ {
			if !insideRound(float64(x)+0.5, float64(y)+0.5, 0, 0, float64(fw), float64(fh), r) {
				continue
			}
			i := scaled.PixOffset(x, y)
			j := c.img.PixOffset(ox+x, oy+y)
			copy(c.img.Pix[j:j+4], scaled.Pix[i:i+4])
		}
	}
	return c.img
}

// Shade dims a photo to match the machine state: full colour when awake,
// darker and greyer when asleep.
func Shade(img *image.RGBA, light Light) *image.RGBA {
	var bright, sat float64
	tint, tintA := ink, 0.0
	switch light {
	case LightOn:
		return img
	case LightWaking:
		bright, sat, tint, tintA = 0.85, 0.85, amber, 0.06
	case LightWakingDim:
		bright, sat, tint, tintA = 0.6, 0.6, amber, 0.04
	case LightFault:
		bright, sat, tint, tintA = 0.45, 0.3, rose, 0.12
	case LightOff:
		bright, sat = 0.42, 0.35
	default:
		bright, sat = 0.55, 0.5
	}
	out := image.NewRGBA(img.Bounds())
	for i := 0; i+3 < len(img.Pix); i += 4 {
		r, g, b := float64(img.Pix[i]), float64(img.Pix[i+1]), float64(img.Pix[i+2])
		l := 0.2126*r + 0.7152*g + 0.0722*b
		r, g, b = l+(r-l)*sat, l+(g-l)*sat, l+(b-l)*sat
		r, g, b = r*bright, g*bright, b*bright
		r = r*(1-tintA) + float64(tint.R)*tintA
		g = g*(1-tintA) + float64(tint.G)*tintA
		b = b*(1-tintA) + float64(tint.B)*tintA
		out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = clamp8(r), clamp8(g), clamp8(b), 255
	}
	return out
}

func clamp8(v float64) uint8 {
	switch {
	case v <= 0:
		return 0
	case v >= 255:
		return 255
	default:
		return uint8(v + 0.5)
	}
}
