package art

import (
	"image"
	"image/color"
	"math"
)

// canvas maps a fixed design space onto an opaque pixel image. Shapes are
// described in design units and filled by coverage (4×4 samples per pixel),
// so the same drawing stays smooth at 24 pixels or 480.
type canvas struct {
	img    *image.RGBA
	sx, sy float64
	t      tones
}

// tones are the furniture and machine colours. Small pictures use lighter
// ones so the machine still stands out from the wall once pixels blend.
type tones struct {
	body, bodyEdge, deskTop, deskLeg, bay color.RGBA
}

var (
	normalTones = tones{body: body, bodyEdge: bodyEdge, deskTop: deskTop, deskLeg: deskLeg, bay: bay}
	smallTones  = tones{body: hex(0x2C3962), bodyEdge: hex(0x3D4C7C), deskTop: hex(0x36446E), deskLeg: hex(0x28335C), bay: hex(0x161D33)}
)

// smallPicture is the width below which a picture uses smallTones.
const smallPicture = 96

const samples = 4

func newCanvas(w, h int, designW, designH float64, bg color.RGBA) *canvas {
	w, h = max(1, w), max(1, h)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	bg.A = 255
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = bg.R, bg.G, bg.B, 255
	}
	return &canvas{img: img, sx: float64(w) / designW, sy: float64(h) / designH, t: normalTones}
}

// shape fills the pixels whose samples fall inside test, within the design
// bounding box (x0, y0)–(x1, y1), blending col at the given opacity.
func (c *canvas) shape(x0, y0, x1, y1 float64, col color.RGBA, alpha float64, inside func(x, y float64) bool) {
	b := c.img.Bounds()
	px0 := max(b.Min.X, int(math.Floor(x0*c.sx)))
	py0 := max(b.Min.Y, int(math.Floor(y0*c.sy)))
	px1 := min(b.Max.X, int(math.Ceil(x1*c.sx)))
	py1 := min(b.Max.Y, int(math.Ceil(y1*c.sy)))
	for py := py0; py < py1; py++ {
		for px := px0; px < px1; px++ {
			hits := 0
			for sy := 0; sy < samples; sy++ {
				y := (float64(py) + (float64(sy)+0.5)/samples) / c.sy
				for sx := 0; sx < samples; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/samples) / c.sx
					if inside(x, y) {
						hits++
					}
				}
			}
			if hits == 0 {
				continue
			}
			c.blend(px, py, col, alpha*float64(hits)/(samples*samples))
		}
	}
}

func (c *canvas) blend(x, y int, col color.RGBA, a float64) {
	i := c.img.PixOffset(x, y)
	p := c.img.Pix[i : i+3 : i+3]
	p[0] = mix(p[0], col.R, a)
	p[1] = mix(p[1], col.G, a)
	p[2] = mix(p[2], col.B, a)
}

func mix(dst, src uint8, a float64) uint8 {
	return uint8(math.Round(float64(dst)*(1-a) + float64(src)*a))
}

func (c *canvas) rect(x, y, w, h float64, col color.RGBA) {
	c.rectA(x, y, w, h, col, 1)
}

func (c *canvas) rectA(x, y, w, h float64, col color.RGBA, alpha float64) {
	c.shape(x, y, x+w, y+h, col, alpha, func(px, py float64) bool {
		return px >= x && px < x+w && py >= y && py < y+h
	})
}

// round fills a rounded rectangle.
func (c *canvas) round(x, y, w, h, r float64, col color.RGBA) {
	r = math.Min(r, math.Min(w, h)/2)
	c.shape(x, y, x+w, y+h, col, 1, func(px, py float64) bool {
		return insideRound(px, py, x, y, w, h, r)
	})
}

func insideRound(px, py, x, y, w, h, r float64) bool {
	if px < x || px >= x+w || py < y || py >= y+h {
		return false
	}
	cx := math.Max(x+r, math.Min(px, x+w-r))
	cy := math.Max(y+r, math.Min(py, y+h-r))
	dx, dy := px-cx, py-cy
	return dx*dx+dy*dy <= r*r
}

func (c *canvas) circle(cx, cy, r float64, col color.RGBA, alpha float64) {
	c.shape(cx-r, cy-r, cx+r, cy+r, col, alpha, func(px, py float64) bool {
		dx, dy := px-cx, py-cy
		return dx*dx+dy*dy <= r*r
	})
}

func (c *canvas) ellipse(cx, cy, rx, ry float64, col color.RGBA) {
	c.shape(cx-rx, cy-ry, cx+rx, cy+ry, col, 1, func(px, py float64) bool {
		dx, dy := (px-cx)/rx, (py-cy)/ry
		return dx*dx+dy*dy <= 1
	})
}

// poly fills a polygon given as x, y pairs.
func (c *canvas) poly(col color.RGBA, pts ...float64) {
	x0, y0, x1, y1 := pts[0], pts[1], pts[0], pts[1]
	for i := 0; i+1 < len(pts); i += 2 {
		x0, x1 = math.Min(x0, pts[i]), math.Max(x1, pts[i])
		y0, y1 = math.Min(y0, pts[i+1]), math.Max(y1, pts[i+1])
	}
	c.shape(x0, y0, x1, y1, col, 1, func(px, py float64) bool {
		in := false
		n := len(pts) / 2
		for i, j := 0, n-1; i < n; j, i = i, i+1 {
			xi, yi, xj, yj := pts[2*i], pts[2*i+1], pts[2*j], pts[2*j+1]
			if (yi > py) != (yj > py) && px < (xj-xi)*(py-yi)/(yj-yi)+xi {
				in = !in
			}
		}
		return in
	})
}

func hex(v uint32) color.RGBA {
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}

// RGBA converts any colour to an opaque RGBA value.
func RGBA(c color.Color) color.RGBA {
	if c == nil {
		return ink
	}
	r, g, b, _ := c.RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 255}
}
