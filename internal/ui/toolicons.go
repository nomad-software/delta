package ui

import (
	"runtime"

	qt "github.com/mappu/miqt/qt6"
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
type iconShape func(p *qt.QPainterPath)

// The icons of the toolbar.
var (
	// newBranchShape is a branch leaving a line, with a plus.
	newBranchShape iconShape = func(p *qt.QPainterPath) {
		p.MoveTo2(6, 3)
		p.LineTo2(6, 15.5)
		circle(p, 6, 18, 2.5)
		circle(p, 17, 6, 2.5)
		p.MoveTo2(17, 8.5)
		p.QuadTo2(17, 18, 8.5, 18)
		p.MoveTo2(19, 16)
		p.LineTo2(19, 22)
		p.MoveTo2(16, 19)
		p.LineTo2(22, 19)
	}
	// checkoutShape is two arrows, going each way, for changing branches.
	checkoutShape iconShape = func(p *qt.QPainterPath) {
		p.MoveTo2(4, 8)
		p.LineTo2(19, 8)
		p.MoveTo2(15.5, 4.5)
		p.LineTo2(19, 8)
		p.LineTo2(15.5, 11.5)
		p.MoveTo2(20, 16)
		p.LineTo2(5, 16)
		p.MoveTo2(8.5, 12.5)
		p.LineTo2(5, 16)
		p.LineTo2(8.5, 19.5)
	}
	// deleteShape is a bin.
	deleteShape iconShape = func(p *qt.QPainterPath) {
		p.MoveTo2(4, 6.5)
		p.LineTo2(20, 6.5)
		p.MoveTo2(9.5, 6.5)
		p.LineTo2(9.5, 5)
		p.ArcTo2(9.5, 3.5, 3, 3, 180, -90)
		p.LineTo2(13, 3.5)
		p.ArcTo2(11.5, 3.5, 3, 3, 90, -90)
		p.LineTo2(14.5, 6.5)
		p.MoveTo2(6, 6.5)
		p.LineTo2(6.9, 18.6)
		p.QuadTo2(7.05, 20.5, 8.9, 20.5)
		p.LineTo2(15.1, 20.5)
		p.QuadTo2(16.95, 20.5, 17.1, 18.6)
		p.LineTo2(18, 6.5)
		p.MoveTo2(10, 10.5)
		p.LineTo2(10, 16.5)
		p.MoveTo2(14, 10.5)
		p.LineTo2(14, 16.5)
	}
	// rescanShape is an arrow going round, clockwise, from the left to the
	// bottom.
	rescanShape iconShape = func(p *qt.QPainterPath) {
		p.MoveTo2(4.5, 12)
		p.ArcTo2(4.5, 4.5, 15, 15, 180, -270)
		p.LineTo2(10, 19.5)
		p.MoveTo2(12.5, 17)
		p.LineTo2(10, 19.5)
		p.LineTo2(12.5, 22)
	}
	// pullShape is an arrow down into a tray.
	pullShape iconShape = func(p *qt.QPainterPath) {
		p.MoveTo2(12, 3)
		p.LineTo2(12, 12)
		p.MoveTo2(8.5, 8.5)
		p.LineTo2(12, 12)
		p.LineTo2(15.5, 8.5)
		tray(p)
	}
	// pushShape is an arrow up out of a tray.
	pushShape iconShape = func(p *qt.QPainterPath) {
		p.MoveTo2(12, 12)
		p.LineTo2(12, 3)
		p.MoveTo2(8.5, 6.5)
		p.LineTo2(12, 3)
		p.LineTo2(15.5, 6.5)
		tray(p)
	}
	// historyShape is a clock.
	historyShape iconShape = func(p *qt.QPainterPath) {
		circle(p, 12, 12, 8.5)
		p.MoveTo2(12, 7)
		p.LineTo2(12, 12)
		p.LineTo2(15.5, 14)
	}
)

// circle adds a circle of the radius at x, y.
func circle(p *qt.QPainterPath, x, y, r float64) {
	p.AddEllipse2(x-r, y-r, 2*r, 2*r)
}

// tray adds the tray that pulling and pushing go into and out of.
func tray(p *qt.QPainterPath) {
	p.MoveTo2(3, 14)
	p.LineTo2(7.5, 14)
	p.LineTo2(9, 16.5)
	p.LineTo2(15, 16.5)
	p.LineTo2(16.5, 14)
	p.LineTo2(21, 14)
	p.MoveTo2(3, 14)
	p.LineTo2(3, 18.5)
	p.ArcTo2(3, 16.5, 4, 4, 180, 90)
	p.LineTo2(19, 20.5)
	p.ArcTo2(17, 16.5, 4, 4, 270, 90)
	p.LineTo2(21, 14)
}

// toolIcon returns an icon of the shape. It is drawn each time it is shown, in
// the colours of the palette as it then is: those of the text, or of disabled
// text where it is disabled.
func toolIcon(shape iconShape) *qt.QIcon {
	return qt.NewQIcon5(newShapeEngine(shape))
}

// newShapeEngine returns the engine that draws the icon of the shape. Qt asks
// it for a copy of itself where an icon is changed.
func newShapeEngine(shape iconShape) *qt.QIconEngine {
	e := qt.NewQIconEngine()
	e.OnPaint(func(p *qt.QPainter, rect *qt.QRect, mode qt.QIcon__Mode, _ qt.QIcon__State) {
		paintShape(p, float64(rect.X()), float64(rect.Y()), float64(min(rect.Width(), rect.Height())), shape, mode)
	})
	// Qt's own pixmap of an engine is not cleared before it is painted, so
	// the icon is drawn on one that is transparent. miqt hands Qt the pixmap
	// returned, which Qt copies once this returns: it is kept until the next
	// is asked for, as otherwise the garbage collector could free it first.
	var last *qt.QPixmap
	e.OnPixmap(func(_ func(*qt.QSize, qt.QIcon__Mode, qt.QIcon__State) *qt.QPixmap, size *qt.QSize, mode qt.QIcon__Mode, _ qt.QIcon__State) *qt.QPixmap {
		image := qt.NewQImage3(size.Width(), size.Height(), qt.QImage__Format_ARGB32_Premultiplied)
		defer image.Delete()
		image.Fill2(qt.Transparent)
		p := qt.NewQPainter2(image.QPaintDevice)
		paintShape(p, 0, 0, float64(min(size.Width(), size.Height())), shape, mode)
		p.End()
		p.Delete()
		last = qt.QPixmap_FromImage(image)
		return last
	})
	e.OnClone(func() *qt.QIconEngine { return newShapeEngine(shape) })
	return e
}

// paintShape draws the shape in a square of the side at x, y, in the colour of
// the mode.
func paintShape(p *qt.QPainter, x, y, side float64, shape iconShape, mode qt.QIcon__Mode) {
	p.Save()
	defer p.Restore()
	p.SetRenderHint(qt.QPainter__Antialiasing)
	p.Translate2(x, y)
	p.Scale(side/iconGrid, side/iconGrid)
	colour := iconColour(mode)
	defer colour.Delete()
	brush := qt.NewQBrush3(colour)
	defer brush.Delete()
	pen := qt.NewQPen8(brush, iconStroke, qt.SolidLine, qt.RoundCap, qt.RoundJoin)
	defer pen.Delete()
	p.SetPenWithPen(pen)
	p.SetBrushWithStyle(qt.NoBrush)
	path := qt.NewQPainterPath()
	defer path.Delete()
	shape(path)
	p.DrawPath(path)
}

// iconColour returns a new colour that an icon is drawn in, in the mode: that of
// the text, or of disabled text.
func iconColour(mode qt.QIcon__Mode) *qt.QColor {
	// The palette is a copy, which the garbage collector may free once it is
	// no longer used, and the colour it gives is its own.
	palette := qt.QGuiApplication_Palette()
	defer runtime.KeepAlive(palette)
	group := qt.QPalette__Active
	if mode == qt.QIcon__Disabled {
		group = qt.QPalette__Disabled
	}
	return qt.NewQColor10(palette.Color(group, qt.QPalette__WindowText))
}
