package ui

import (
	qt "github.com/mappu/miqt/qt6"

	"github.com/nomad-software/delta/internal/git"
	"github.com/nomad-software/delta/internal/settings"
)

// changeDotSize is how wide the circle of a changed file is, as reghead's dots
// of its matches are.
const changeDotSize = 12

// changeOutline is how wide the outline of a circle is, in pixels at
// changeDotSize.
const changeOutline = 2

// changeFill is how much of its colour the inside of a circle has, as the wash
// of apemail's folders has.
const changeFill = 0.2

// changeColours are the colours of what happened to a file, by its code, so
// that each is told at a glance, from the settings: by default Solarized's
// accents, which read on light and dark alike, so that each is the same in
// every theme, and its grey for what git does not track. A circle is drawn in
// them each time it is shown, so that one changed shows once the lists are
// painted again.
var changeColours = changeCodes(settings.DefaultChangeColours)

// changeCodes returns the colours by the codes of what happened.
func changeCodes(c settings.ChangeColours) map[byte]string {
	return map[byte]string{
		'M':           c.Modified,
		'A':           c.Added,
		'D':           c.Deleted,
		'R':           c.Renamed,
		'C':           c.Copied,
		'T':           c.TypeChanged,
		git.Untracked: c.Untracked,
		git.Conflict:  c.Conflict,
	}
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
	icon := paintedIcon(func(p *qt.QPainter, x, y, side float64, mode qt.QIcon__Mode) {
		paintDot(p, x, y, side, code, mode)
	})
	changeIcons[code] = icon
	return icon
}

// paintDot draws the circle of the code in a square of the side at x, y: an
// outline in its colour, changeOutline wide at the size it is made for, and inside
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
	stroke := side * changeOutline / changeDotSize
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
// that of disabled text where it is disabled, as the toolbar's icons are.
func dotColour(name string, mode qt.QIcon__Mode) *qt.QColor {
	if mode == qt.QIcon__Disabled {
		return iconColour(mode)
	}
	return qt.NewQColor6(name)
}
