package ui

import (
	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtgui"
)

// iconPainter draws an icon in a square of the side at x, y, in the mode.
type iconPainter func(p *qtgui.QPainter, x, y, side float64, mode qtgui.QIconMode)

// paintedIcon returns an icon that paint draws each time it is shown, so that it
// can follow the palette as it then is.
func paintedIcon(paint iconPainter) *qtgui.QIcon {
	return qtgui.NewQIconWithIconEngine(newPaintedEngine(paint))
}

// paintedEngine is the engine of a painted icon.
type paintedEngine struct {
	qtgui.QIconEngineBase
	paint iconPainter
}

// newPaintedEngine returns the engine of a painted icon. Qt asks it for a copy of
// itself where an icon is changed.
func newPaintedEngine(paint iconPainter) *qtgui.QIconEngine {
	return qtgui.NewQIconEngineWith(&paintedEngine{paint: paint})
}

// Paint draws the icon in the rectangle.
func (e *paintedEngine) Paint(p *qtgui.QPainter, rect qtcore.QRect, mode qtgui.QIconMode, _ qtgui.QIconState) {
	e.paint(p, float64(rect.X), float64(rect.Y), float64(min(rect.Width, rect.Height)), mode)
}

// Pixmap draws the icon on a pixmap that is transparent, as Qt's own pixmap of
// an engine is not cleared before it is painted.
func (e *paintedEngine) Pixmap(size qtcore.QSize, mode qtgui.QIconMode, _ qtgui.QIconState) *qtgui.QPixmap {
	image := qtgui.NewQImageWithSize(size, qtgui.QImageFormat_ARGB32_Premultiplied)
	image.Fill(0)
	p := qtgui.NewQPainterOn(image)
	e.paint(p, 0, 0, float64(min(size.Width, size.Height)), mode)
	p.End()
	return qtgui.QPixmapFromImage(image)
}

// Clone returns a copy of the engine.
func (e *paintedEngine) Clone() *qtgui.QIconEngine {
	return newPaintedEngine(e.paint)
}
