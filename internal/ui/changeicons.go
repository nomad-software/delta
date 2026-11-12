package ui

import (
	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtgui"

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
var changeIcons = map[byte]*qtgui.QIcon{}

// changeIcon returns the circle of what happened to a file, by its code, or a
// blank of the same size for a code with no colour, so that a row without one
// lines up with those with one.
func changeIcon(code byte) *qtgui.QIcon {
	if icon := changeIcons[code]; icon != nil {
		return icon
	}
	icon := paintedIcon(func(p *qtgui.QPainter, x, y, side float64, mode qtgui.QIconMode) {
		paintDot(p, x, y, side, code, mode)
	})
	changeIcons[code] = icon
	return icon
}

// paintDot draws the circle of the code in a square of the side at x, y: an
// outline in its colour, changeOutline wide at the size it is made for, and inside
// it a pale wash of the colour.
func paintDot(p *qtgui.QPainter, x, y, side float64, code byte, mode qtgui.QIconMode) {
	name, ok := changeColours[code]
	if !ok {
		return
	}
	p.Save()
	defer p.Restore()
	p.SetRenderHint(qtgui.QPainterAntialiasing)
	colour := dotColour(name, mode)
	stroke := side * changeOutline / changeDotSize
	p.SetPen(qtgui.NewQPenWithBrush(qtgui.NewQBrushWithColor(colour), stroke))
	wash := colour
	wash.A = alpha(changeFill)
	p.SetBrushColor(wash)
	p.DrawEllipseF(qtcore.QRectF{X: x + stroke/2, Y: y + stroke/2, Width: side - stroke, Height: side - stroke})
}

// dotColour returns the colour a circle is drawn in, in the mode: its own, on
// a selected row too, so that what happened to a file shows wherever it is, or
// that of disabled text where it is disabled, as the toolbar's icons are.
func dotColour(name string, mode qtgui.QIconMode) qtgui.QColor {
	if mode == qtgui.QIconDisabled {
		return iconColour(mode)
	}
	return colour(name)
}
