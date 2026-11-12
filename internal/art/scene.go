package art

import (
	"image"
	"image/color"
)

// Light is what the machine's power light and screen show.
type Light int

const (
	LightUnknown   Light = iota // not checked yet
	LightOff                    // asleep: screen dark, light off
	LightOn                     // awake: screen lit, green light
	LightWaking                 // wake sent: amber light, bright pulse
	LightWakingDim              // wake sent: amber light, quiet pulse
	LightFault                  // disabled or cannot be woken
)

// Website palette (Night Room).
var (
	ink        = hex(0x080C14)
	wall       = hex(0x131A2D)
	floor      = hex(0x0C1120)
	frame      = hex(0x222B45)
	glass      = hex(0x0B1020)
	moon       = hex(0xC6CEDE)
	city       = hex(0x18203A)
	deskTop    = hex(0x26314F)
	deskLeg    = hex(0x1C2440)
	body       = hex(0x1E2742)
	bodyEdge   = hex(0x2A3556)
	bay        = hex(0x141B2F)
	screenOff  = hex(0x070A12)
	screenOn   = hex(0x0F1A33)
	codeBlue   = hex(0x4A8FC7)
	amber      = hex(0xF5A524)
	green      = hex(0x22C55E)
	rose       = hex(0xFB7185)
	ledOff     = hex(0x2A3350)
	ledUnknown = hex(0x5A6585)
	leaf       = hex(0x1D3A2E)
	leafLight  = hex(0x24493A)
	pot        = hex(0x2A2436)
	iconBody   = hex(0x8E99B3)
)

// SceneAspect is the width-to-height ratio of the room picture.
const SceneAspect = 1.5

// Scene draws the night room with the machine in it, w × h pixels, on bg.
// The machine fills most of the picture so it still reads at 30 × 20
// pixels; the window, moon and plant set the scene as on the website.
func Scene(kind Kind, light Light, w, h int, bg color.RGBA) *image.RGBA {
	c := newCanvas(w, h, 300, 200, bg)
	if w < smallPicture {
		c.t = smallTones
	}
	c.round(0, 0, 300, 200, 12, wall)
	c.round(0, 160, 300, 40, 12, floor)
	c.rect(0, 160, 300, 20, floor)

	// Window with the moon and a sleeping city.
	c.round(16, 16, 82, 74, 3, frame)
	for _, p := range [][2]float64{{21, 21}, {59, 21}, {21, 55}, {59, 55}} {
		c.rect(p[0], p[1], 34, 30, glass)
	}
	c.circle(80, 35, c.atLeast(8, 1.2), moon, 1)
	for _, b := range [][4]float64{{22, 72, 7, 13}, {31, 66, 7, 19}, {40, 75, 8, 10}, {60, 70, 7, 15}, {69, 64, 8, 21}, {79, 73, 7, 12}} {
		c.rect(b[0], b[1], b[2], b[3], city)
	}
	for _, l := range [][2]float64{{24, 76}, {33, 70}, {71, 68}, {81, 77}} {
		c.rectA(l[0], l[1], 2.4, 2.4, amber, 0.85)
	}

	// Plant in the corner.
	c.ellipse(26, 120, 5.5, 19, leafLight)
	c.ellipse(19, 128, 4.6, 14, leaf)
	c.ellipse(33, 128, 4.6, 14, leaf)
	c.round(18, 140, 16, 26, 2, pot)

	switch kind {
	case Server:
		drawRack(c, light)
	case Console:
		drawMedia(c, light)
	default:
		drawDesk(c)
		switch kind {
		case Laptop:
			drawLaptop(c, light)
		case NAS:
			drawNAS(c, light)
		case Mini:
			drawMonitor(c, 128, 40, 104, 74, light)
			c.round(240, 110, 44, 18, 3, c.t.bodyEdge)
			c.rect(246, 118, 20, 2, c.t.body)
			drawLED(c, 275, 119, 3.2, light)
		default:
			drawMonitor(c, 122, 30, 120, 86, light)
			c.round(250, 62, 34, 66, 3, c.t.body)
			for _, y := range []float64{70, 76, 82} {
				c.rect(256, y, 22, 2, c.t.bodyEdge)
			}
			drawLED(c, 267, 112, 4, light)
		}
	}
	return c.img
}

// atLeast returns r, or the radius that keeps a dot minPx pixels wide when
// the picture is small.
func (c *canvas) atLeast(r, minPx float64) float64 {
	if need := minPx / 2 / c.sx; need > r {
		return need
	}
	return r
}

func drawDesk(c *canvas) {
	c.rect(116, 134, 5, 34, c.t.deskLeg)
	c.rect(282, 134, 5, 34, c.t.deskLeg)
	c.round(108, 128, 186, 6, 1.5, c.t.deskTop)
}

// drawMonitor draws a monitor whose frame starts at (x, y), w × h, standing
// on the desk top at y=128.
func drawMonitor(c *canvas, x, y, w, h float64, light Light) {
	c.round(x, y, w, h, 4, c.t.body)
	drawScreen(c, x+5, y+5, w-10, h-10, light)
	cx := x + w/2
	c.rect(cx-5, y+h, 10, 128-(y+h)-3, c.t.body)
	c.round(cx-18, 124, 36, 4, 1.5, c.t.body)
}

// drawScreen shows code on a lit screen and nothing on a dark one.
func drawScreen(c *canvas, x, y, w, h float64, light Light) {
	if light != LightOn {
		c.rect(x, y, w, h, screenOff)
		return
	}
	c.rect(x, y, w, h, screenOn)
	lineH := h * 0.08
	step := h * 0.17
	widths := []float64{0.48, 0.66, 0.55, 0.4}
	for i, f := range widths {
		col, a := codeBlue, 1-float64(i)*0.15
		if i == len(widths)-1 {
			col, a = amber, 1
		}
		c.rectA(x+w*0.1, y+h*0.17+float64(i)*step, w*f, lineH, col, a)
	}
}

func drawLaptop(c *canvas, light Light) {
	c.round(140, 50, 112, 72, 4, c.t.body)
	drawScreen(c, 146, 56, 100, 60, light)
	c.poly(c.t.bodyEdge, 128, 121, 264, 121, 272, 128, 120, 128)
	drawLED(c, 256, 124.5, 2.6, light)
}

func drawNAS(c *canvas, light Light) {
	c.round(150, 46, 92, 82, 5, c.t.body)
	for i := 0; i < 4; i++ {
		x := 160 + float64(i)*19
		c.round(x, 56, 14, 48, 2, c.t.bay)
		col := ledOff
		if light == LightOn {
			col = green
		}
		c.circle(x+7, 96, c.atLeast(2, 0.8), col, 1)
	}
	drawLED(c, 196, 116, 4, light)
}

func drawRack(c *canvas, light Light) {
	c.round(140, 14, 104, 154, 4, c.t.body)
	c.rect(146, 20, 92, 142, c.t.bay)
	for i := 0; i < 6; i++ {
		y := 25 + float64(i)*22.5
		c.round(151, y, 82, 18, 2, c.t.deskTop)
		c.rect(158, y+6, 36, 2, c.t.bodyEdge)
		c.rect(158, y+10.5, 26, 2, c.t.bodyEdge)
		if i == 0 {
			drawLED(c, 222, y+9, 3.6, light)
			continue
		}
		col := ledOff
		if light == LightOn {
			col = green
		}
		c.circle(222, y+9, c.atLeast(2, 0.8), col, 1)
	}
}

func drawMedia(c *canvas, light Light) {
	c.rect(130, 156, 5, 12, c.t.deskLeg)
	c.rect(274, 156, 5, 12, c.t.deskLeg)
	c.round(122, 146, 162, 10, 2, c.t.deskTop)
	c.round(118, 26, 170, 104, 4, c.t.body)
	drawScreen(c, 124, 32, 158, 92, light)
	c.rect(197, 130, 12, 16, c.t.body)
	c.round(236, 134, 40, 12, 2.5, c.t.bodyEdge)
	drawLED(c, 268, 140, 2.8, light)
}

// drawLED draws the power light with a glow that matches the state.
func drawLED(c *canvas, x, y, r float64, light Light) {
	r = c.atLeast(r, 1)
	switch light {
	case LightOn:
		c.circle(x, y, r*3.2, green, 0.14)
		c.circle(x, y, r*1.9, green, 0.28)
		c.circle(x, y, r, green, 1)
	case LightWaking:
		c.circle(x, y, r*4.2, amber, 0.16)
		c.circle(x, y, r*2.4, amber, 0.32)
		c.circle(x, y, r, amber, 1)
	case LightWakingDim:
		c.circle(x, y, r*2.2, amber, 0.18)
		c.circle(x, y, r, amber, 0.75)
	case LightFault:
		c.circle(x, y, r*2, rose, 0.2)
		c.circle(x, y, r, rose, 1)
	case LightOff:
		c.circle(x, y, r, ledOff, 1)
	default:
		c.circle(x, y, r, ledUnknown, 1)
	}
}

// stateColor is the colour a small icon uses to show the light.
func stateColor(light Light) (color.RGBA, bool) {
	switch light {
	case LightOn:
		return green, true
	case LightWaking, LightWakingDim:
		return amber, true
	case LightFault:
		return rose, true
	case LightOff:
		return ledOff, false
	default:
		return ledUnknown, false
	}
}

// Icon draws the machine type alone, w × h pixels, for list rows.
func Icon(kind Kind, light Light, w, h int, bg color.RGBA) *image.RGBA {
	c := newCanvas(w, h, 40, 40, bg)
	state, lit := stateColor(light)
	screen := screenOff
	if lit {
		screen = state
	}
	switch kind {
	case Laptop:
		c.round(8, 8, 24, 18, 2, iconBody)
		c.rect(10.5, 10.5, 19, 13, screen)
		c.poly(iconBody, 4, 27, 36, 27, 38.5, 31.5, 1.5, 31.5)
	case Server:
		c.round(10, 3, 20, 34, 2, iconBody)
		for i := 0; i < 4; i++ {
			y := 6 + float64(i)*7.6
			c.round(12.5, y, 15, 5.6, 1, bay)
			col := state
			if i > 0 && !lit {
				col = ledOff
			}
			c.circle(24.5, y+2.8, 1.3, col, 1)
		}
	case NAS:
		c.round(8, 6, 24, 29, 2.5, iconBody)
		for i := 0; i < 3; i++ {
			c.rect(11.5+float64(i)*6.2, 9.5, 4.2, 16, bay)
		}
		c.circle(20, 30, 2, state, 1)
	case Mini:
		c.round(5, 15, 30, 11, 2.5, iconBody)
		c.rect(8.5, 19.6, 13, 1.8, bay)
		c.circle(30, 20.5, 2, state, 1)
	case Console:
		c.round(3, 7, 34, 22, 2, iconBody)
		c.rect(5.5, 9.5, 29, 17, screen)
		c.rect(18, 29, 4, 3, iconBody)
		c.round(12, 31.5, 16, 2.5, 1, iconBody)
	default:
		c.round(5, 6, 30, 22, 2, iconBody)
		c.rect(7.5, 8.5, 25, 17, screen)
		c.rect(18, 28, 4, 4, iconBody)
		c.round(12, 31, 16, 2.5, 1, iconBody)
	}
	return c.img
}

// Logo draws the amber power mark used on the website, w × h pixels.
func Logo(w, h int, bg color.RGBA) *image.RGBA {
	c := newCanvas(w, h, 40, 40, bg)
	c.circle(20, 20, 18, amber, 1)
	mark := ink
	// Ring with a gap at the top, then the stem through the gap.
	c.shape(6, 6, 34, 34, mark, 1, func(x, y float64) bool {
		dx, dy := x-20, y-21
		d := dx*dx + dy*dy
		if d < 8.6*8.6 || d > 11.8*11.8 {
			return false
		}
		return !(dy < 0 && dx > dy*0.75 && dx < -dy*0.75)
	})
	c.round(18.4, 7, 3.2, 14, 1.6, mark)
	return c.img
}
