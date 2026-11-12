package main

import (
	"bytes"
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/goregular"
)

var (
	colBar       = color.RGBA{0x1d, 0x1d, 0x21, 0xff}
	colBarLine   = color.RGBA{0x34, 0x34, 0x3a, 0xff}
	colArea      = color.RGBA{0x2b, 0x2b, 0x30, 0xff}
	colButton    = color.RGBA{0x35, 0x35, 0x3c, 0xff}
	colHover     = color.RGBA{0x45, 0x45, 0x4e, 0xff}
	colPressed   = color.RGBA{0x28, 0x28, 0x2e, 0xff}
	colActive    = color.RGBA{0x2f, 0x6b, 0xd8, 0xff}
	colActiveHov = color.RGBA{0x3c, 0x7a, 0xe8, 0xff}
	colText      = color.RGBA{0xea, 0xea, 0xee, 0xff}
	colDim       = color.RGBA{0x9a, 0x9a, 0xa4, 0xff}
	colDisabled  = color.RGBA{0x66, 0x66, 0x6e, 0xff}
	colBorder    = color.RGBA{0x70, 0x70, 0x78, 0xff}
	colError     = color.RGBA{0xe0, 0x5a, 0x4a, 0xff}
	colTooltip   = color.RGBA{0x0e, 0x0e, 0x10, 0xf0}
)

type WidgetKind int

const (
	KindButton WidgetKind = iota
	KindLabel
	KindSlider
)

type Icon int

const (
	IconNone Icon = iota
	IconFirst
	IconPrev
	IconPlay
	IconPause
	IconNext
	IconLast
)

// Widget is a single element of the immediate-mode UI. The widget list is
// rebuilt from the current state for every Update and Draw.
type Widget struct {
	ID       string
	Rect     image.Rectangle
	Kind     WidgetKind
	Label    string
	Icon     Icon
	Active   bool
	Disabled bool
	Repeat   bool // button fires repeatedly while held
	Tooltip  string
	Color    color.Color // label color
	Center   bool        // center label text
	Flat     bool        // button without background unless hovered
}

var fontSource *text.GoTextFaceSource

func init() {
	s, err := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	if err != nil {
		panic(err)
	}
	fontSource = s
}

type uiFonts struct {
	scale float64
	face  *text.GoTextFace
	big   *text.GoTextFace
}

func (f *uiFonts) get(scale float64) *uiFonts {
	if f.face == nil || f.scale != scale {
		f.scale = scale
		f.face = &text.GoTextFace{Source: fontSource, Size: 13.5 * scale}
		f.big = &text.GoTextFace{Source: fontSource, Size: 17 * scale}
	}
	return f
}

func textWidth(s string, face text.Face) int {
	w, _ := text.Measure(s, face, 0)
	return int(math.Ceil(w))
}

func drawText(dst *ebiten.Image, s string, face *text.GoTextFace, x, y float64, clr color.Color) {
	op := &text.DrawOptions{}
	op.GeoM.Translate(math.Round(x), math.Round(y))
	op.ColorScale.ScaleWithColor(clr)
	text.Draw(dst, s, face, op)
}

// drawTextIn draws text vertically centered in r.
func drawTextIn(dst *ebiten.Image, s string, face *text.GoTextFace, r image.Rectangle, center bool, clr color.Color) {
	m := face.Metrics()
	h := m.HAscent + m.HDescent
	y := float64(r.Min.Y) + (float64(r.Dy())-h)/2
	x := float64(r.Min.X)
	if center {
		x += (float64(r.Dx()) - float64(textWidth(s, face))) / 2
	}
	drawText(dst, s, face, x, y, clr)
}

func fillRect(dst *ebiten.Image, r image.Rectangle, clr color.Color) {
	vector.FillRect(dst, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), clr, false)
}

func fillRoundRect(dst *ebiten.Image, r image.Rectangle, radius float32, clr color.Color) {
	x, y, w, h := float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy())
	if radius*2 > h {
		radius = h / 2
	}
	var p vector.Path
	p.MoveTo(x+radius, y)
	p.LineTo(x+w-radius, y)
	p.Arc(x+w-radius, y+radius, radius, -math.Pi/2, 0, vector.Clockwise)
	p.LineTo(x+w, y+h-radius)
	p.Arc(x+w-radius, y+h-radius, radius, 0, math.Pi/2, vector.Clockwise)
	p.LineTo(x+radius, y+h)
	p.Arc(x+radius, y+h-radius, radius, math.Pi/2, math.Pi, vector.Clockwise)
	p.LineTo(x, y+radius)
	p.Arc(x+radius, y+radius, radius, math.Pi, 3*math.Pi/2, vector.Clockwise)
	p.Close()
	op := &vector.DrawPathOptions{AntiAlias: true}
	op.ColorScale.ScaleWithColor(clr)
	vector.FillPath(dst, &p, nil, op)
}

func fillPoly(dst *ebiten.Image, pts [][2]float32, clr color.Color) {
	var p vector.Path
	p.MoveTo(pts[0][0], pts[0][1])
	for _, pt := range pts[1:] {
		p.LineTo(pt[0], pt[1])
	}
	p.Close()
	op := &vector.DrawPathOptions{AntiAlias: true}
	op.ColorScale.ScaleWithColor(clr)
	vector.FillPath(dst, &p, nil, op)
}

func drawIcon(dst *ebiten.Image, icon Icon, r image.Rectangle, scale float64, clr color.Color) {
	cx := float32(r.Min.X) + float32(r.Dx())/2
	cy := float32(r.Min.Y) + float32(r.Dy())/2
	s := float32(scale)
	h := 6 * s // half height of the triangles
	w := 9 * s
	bar := 2.5 * s
	tri := func(x0 float32, right bool) {
		if right {
			fillPoly(dst, [][2]float32{{x0, cy - h}, {x0 + w, cy}, {x0, cy + h}}, clr)
		} else {
			fillPoly(dst, [][2]float32{{x0 + w, cy - h}, {x0, cy}, {x0 + w, cy + h}}, clr)
		}
	}
	rect := func(x0, y0, ww, hh float32) {
		vector.FillRect(dst, x0, y0, ww, hh, clr, true)
	}
	switch icon {
	case IconPlay:
		tri(cx-w/2+1*s, true)
	case IconPause:
		rect(cx-5*s, cy-h, 3.5*s, 2*h)
		rect(cx+1.5*s, cy-h, 3.5*s, 2*h)
	case IconPrev:
		rect(cx-w/2-1*s, cy-h, bar, 2*h)
		tri(cx-w/2+bar, false)
	case IconNext:
		tri(cx-w/2-bar+1*s, true)
		rect(cx+w/2-bar+1*s, cy-h, bar, 2*h)
	case IconFirst:
		rect(cx-w-1*s, cy-h, bar, 2*h)
		tri(cx-w+bar-1*s, false)
		tri(cx-1*s, false)
	case IconLast:
		tri(cx-w+1*s, true)
		tri(cx+1*s, true)
		rect(cx+w-bar+1*s, cy-h, bar, 2*h)
	}
}

// ellipsize shortens s with "…" so that it fits into maxW pixels.
func ellipsize(s string, face text.Face, maxW int) string {
	if textWidth(s, face) <= maxW {
		return s
	}
	rs := []rune(s)
	for len(rs) > 1 && textWidth(string(rs)+"…", face) > maxW {
		rs = rs[:len(rs)-1]
	}
	return string(rs) + "…"
}
