package ui

import (
	"runtime"

	qt "github.com/mappu/miqt/qt6"

	"github.com/nomad-software/delta/internal/git"
)

// changeDotSize is how wide the circle of a changed file is, as reghead's dots
// of its matches are.
const changeDotSize = 12

// changeFill is how much of its colour the inside of a circle has, as the wash
// of apemail's folders has.
const changeFill = 0.2

// changeColours are the colours of what happened to a file, so that each is
// told at a glance: Solarized's accents, which read on light and dark alike,
// so that each is the same in every theme, and its grey for what git does not
// track.
var changeColours = map[byte]string{
	'M':           "#268bd2", // blue
	'A':           "#859900", // green
	'D':           "#dc322f", // red
	'R':           "#6c71c4", // violet
	'C':           "#2aa198", // cyan
	'T':           "#b58900", // yellow
	git.Untracked: "#839496", // base0
	git.Conflict:  "#cb4b16", // orange
}

// changeIcons are the icons made so far, one for each code, as a list can have
// thousands of rows but only a few codes. Each is drawn in the palette as it is
// when it is shown, so that it is greyed where it is disabled.
var changeIcons = map[byte]*qt.QIcon{}

// changeIcon returns the circle of what happened to a file, by its code, or a
// blank of the same size for a code with no colour, so that a row without one
// lines up with those with one.
func changeIcon(code byte) *qt.QIcon {
	if icon := changeIcons[code]; icon != nil {
		return icon
	}
	icon := qt.NewQIcon5(newDotEngine(code))
	changeIcons[code] = icon
	return icon
}

// newDotEngine returns the engine that draws the circle of the code. Qt asks it
// for a copy of itself where an icon is changed.
func newDotEngine(code byte) *qt.QIconEngine {
	e := qt.NewQIconEngine()
	e.OnPaint(func(p *qt.QPainter, rect *qt.QRect, mode qt.QIcon__Mode, _ qt.QIcon__State) {
		paintDot(p, float64(rect.X()), float64(rect.Y()), float64(min(rect.Width(), rect.Height())), code, mode)
	})
	// As in newShapeEngine, the pixmap is drawn transparent, and kept until the
	// next is asked for, as Qt copies it only once this returns.
	var last *qt.QPixmap
	e.OnPixmap(func(_ func(*qt.QSize, qt.QIcon__Mode, qt.QIcon__State) *qt.QPixmap, size *qt.QSize, mode qt.QIcon__Mode, _ qt.QIcon__State) *qt.QPixmap {
		image := qt.NewQImage3(size.Width(), size.Height(), qt.QImage__Format_ARGB32_Premultiplied)
		defer image.Delete()
		image.Fill2(qt.Transparent)
		p := qt.NewQPainter2(image.QPaintDevice)
		paintDot(p, 0, 0, float64(min(size.Width(), size.Height())), code, mode)
		p.End()
		p.Delete()
		last = qt.QPixmap_FromImage(image)
		return last
	})
	e.OnClone(func() *qt.QIconEngine { return newDotEngine(code) })
	return e
}

// paintDot draws the circle of the code in a square of the side at x, y: an
// outline in its colour, a pixel wide at the size it is made for, and inside
// it a pale wash of the colour.
func paintDot(p *qt.QPainter, x, y, side float64, code byte, mode qt.QIcon__Mode) {
	name, ok := changeColours[code]
	if !ok {
		return
	}
	p.Save()
	defer p.Restore()
	p.SetRenderHint(qt.QPainter__Antialiasing)
	colour := dotColour(name, mode)
	defer colour.Delete()
	stroke := side / changeDotSize
	outline := qt.NewQBrush3(colour)
	defer outline.Delete()
	pen := qt.NewQPen4(outline, stroke)
	defer pen.Delete()
	wash := qt.NewQColor10(colour)
	defer wash.Delete()
	wash.SetAlphaF(changeFill)
	fill := qt.NewQBrush3(wash)
	defer fill.Delete()
	p.SetPenWithPen(pen)
	p.SetBrush(fill)
	bounds := qt.NewQRectF4(x+stroke/2, y+stroke/2, side-stroke, side-stroke)
	defer bounds.Delete()
	p.DrawEllipse(bounds)
}

// dotColour returns a new colour a circle is drawn in, in the mode: its own, on
// a selected row too, so that what happened to a file shows wherever it is, or
// that of disabled text where it is disabled.
func dotColour(name string, mode qt.QIcon__Mode) *qt.QColor {
	if mode == qt.QIcon__Disabled {
		palette := qt.QGuiApplication_Palette()
		defer runtime.KeepAlive(palette)
		return qt.NewQColor10(palette.Color(qt.QPalette__Disabled, qt.QPalette__WindowText))
	}
	return qt.NewQColor6(name)
}
