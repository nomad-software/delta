package ui

import (
	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtgui"
)

// The toolbar's icons are drawn in code in one style, as apemail's are drawn
// in SVG: outlines on a grid of iconGrid, iconStroke wide with round ends and
// corners, in the colour of the text, so that they follow the theme.
const (
	iconGrid   = 24
	iconStroke = 1.5
	// toolIconSize is the size of the toolbar's icons, as apemail's.
	toolIconSize = 32
	// toolButtonWidth is the least width of a toolbar's button, so that one
	// with a short name is not thinner than the rest.
	toolButtonWidth = 80
)

// iconShape adds an icon's lines to a path on the grid.
type iconShape func(p iconPath)

// The icons of the toolbar.
var (
	// openShape is an open folder.
	openShape iconShape = func(p iconPath) {
		p.moveTo(20, 10)
		p.lineTo(20, 8)
		p.quadTo(20, 6.5, 18.5, 6.5)
		p.lineTo(11.5, 6.5)
		p.lineTo(9.5, 4)
		p.lineTo(4.5, 4)
		p.quadTo(3, 4, 3, 5.5)
		p.lineTo(3, 18.5)
		p.quadTo(3, 20, 4.5, 20)
		p.lineTo(18, 20)
		p.quadTo(19.2, 20, 19.6, 18.9)
		p.lineTo(21.8, 11.7)
		p.quadTo(22.2, 10, 20.5, 10)
		p.lineTo(9, 10)
		p.quadTo(7.8, 10, 7.3, 11)
		p.lineTo(3.4, 19.4)
	}
	// newBranchShape is a branch leaving a line, with a plus.
	newBranchShape iconShape = func(p iconPath) {
		p.moveTo(6, 3)
		p.lineTo(6, 15.5)
		circle(p, 6, 18, 2.5)
		circle(p, 17, 6, 2.5)
		p.moveTo(17, 8.5)
		p.quadTo(17, 18, 8.5, 18)
		p.moveTo(19, 16)
		p.lineTo(19, 22)
		p.moveTo(16, 19)
		p.lineTo(22, 19)
	}
	// checkoutShape is two arrows, going each way, for changing branches.
	checkoutShape iconShape = func(p iconPath) {
		p.moveTo(4, 8)
		p.lineTo(19, 8)
		p.moveTo(15.5, 4.5)
		p.lineTo(19, 8)
		p.lineTo(15.5, 11.5)
		p.moveTo(20, 16)
		p.lineTo(5, 16)
		p.moveTo(8.5, 12.5)
		p.lineTo(5, 16)
		p.lineTo(8.5, 19.5)
	}
	// finishShape is a flag on its pole, for a branch finished with.
	finishShape iconShape = func(p iconPath) {
		p.moveTo(6, 21)
		p.lineTo(6, 3.5)
		p.moveTo(6, 4)
		p.lineTo(18.5, 4)
		p.lineTo(15, 8.5)
		p.lineTo(18.5, 13)
		p.lineTo(6, 13)
	}
	// rescanShape is an arrow going round, clockwise, from the left to the
	// bottom.
	rescanShape iconShape = func(p iconPath) {
		p.moveTo(4.5, 12)
		p.arcTo(4.5, 4.5, 15, 15, 180, -270)
		p.lineTo(10, 19.5)
		p.moveTo(12.5, 17)
		p.lineTo(10, 19.5)
		p.lineTo(12.5, 22)
	}
	// pullShape is an arrow down into a tray.
	pullShape iconShape = func(p iconPath) {
		p.moveTo(12, 3)
		p.lineTo(12, 12)
		p.moveTo(8.5, 8.5)
		p.lineTo(12, 12)
		p.lineTo(15.5, 8.5)
		tray(p)
	}
	// pushShape is an arrow up out of a tray.
	pushShape iconShape = func(p iconPath) {
		p.moveTo(12, 12)
		p.lineTo(12, 3)
		p.moveTo(8.5, 6.5)
		p.lineTo(12, 3)
		p.lineTo(15.5, 6.5)
		tray(p)
	}
	// historyShape is a clock.
	historyShape iconShape = func(p iconPath) {
		circle(p, 12, 12, 8.5)
		p.moveTo(12, 7)
		p.lineTo(12, 12)
		p.lineTo(15.5, 14)
	}
)

// iconPath is the path of an icon's lines, added to by points on the grid.
type iconPath struct{ *qtgui.QPainterPath }

// moveTo starts a line at x, y.
func (p iconPath) moveTo(x, y float64) { p.MoveTo(qtcore.QPointF{X: x, Y: y}) }

// lineTo draws a line to x, y.
func (p iconPath) lineTo(x, y float64) { p.LineTo(qtcore.QPointF{X: x, Y: y}) }

// quadTo draws a curve to x, y, pulled towards cx, cy.
func (p iconPath) quadTo(cx, cy, x, y float64) {
	p.QuadTo(qtcore.QPointF{X: cx, Y: cy}, qtcore.QPointF{X: x, Y: y})
}

// arcTo draws an arc of the ellipse in the rectangle at x, y, from the angle
// start, in degrees, through length.
func (p iconPath) arcTo(x, y, width, height, start, length float64) {
	p.ArcTo(qtcore.QRectF{X: x, Y: y, Width: width, Height: height}, start, length)
}

// circle adds a circle of the radius at x, y.
func circle(p iconPath, x, y, r float64) {
	p.AddEllipse(qtcore.QRectF{X: x - r, Y: y - r, Width: 2 * r, Height: 2 * r})
}

// tray adds the tray that pulling and pushing go into and out of.
func tray(p iconPath) {
	p.moveTo(3, 14)
	p.lineTo(7.5, 14)
	p.lineTo(9, 16.5)
	p.lineTo(15, 16.5)
	p.lineTo(16.5, 14)
	p.lineTo(21, 14)
	p.moveTo(3, 14)
	p.lineTo(3, 18.5)
	p.arcTo(3, 16.5, 4, 4, 180, 90)
	p.lineTo(19, 20.5)
	p.arcTo(17, 16.5, 4, 4, 270, 90)
	p.lineTo(21, 14)
}

// toolIcon returns an icon of the shape. It is drawn each time it is shown, in
// the colours of the palette as it then is: those of the text, or of disabled
// text where it is disabled.
func toolIcon(shape iconShape) *qtgui.QIcon {
	return paintedIcon(func(p *qtgui.QPainter, x, y, side float64, mode qtgui.QIconMode) {
		paintShape(p, x, y, side, shape, mode)
	})
}

// paintShape draws the shape in a square of the side at x, y, in the colour of
// the mode.
func paintShape(p *qtgui.QPainter, x, y, side float64, shape iconShape, mode qtgui.QIconMode) {
	p.Save()
	defer p.Restore()
	p.SetRenderHint(qtgui.QPainterAntialiasing)
	p.Translate(x, y)
	p.Scale(side/iconGrid, side/iconGrid)
	brush := qtgui.NewQBrushWithColor(iconColour(mode))
	p.SetPen(qtgui.NewQPenWithBrushFull(brush, iconStroke, qtcore.SolidLine, qtcore.RoundCap, qtcore.RoundJoin))
	p.SetBrushStyle(qtcore.NoBrush)
	path := iconPath{qtgui.NewQPainterPath()}
	shape(path)
	p.DrawPath(path.QPainterPath)
}

// iconColour returns the colour that an icon is drawn in, in the mode: that of
// the text, or of disabled text.
func iconColour(mode qtgui.QIconMode) qtgui.QColor {
	group := qtgui.QPaletteActive
	if mode == qtgui.QIconDisabled {
		group = qtgui.QPaletteDisabled
	}
	return qtgui.QGuiApplicationPalette().GroupColor(group, qtgui.QPaletteWindowText)
}
