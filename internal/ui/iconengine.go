package ui

import qt "github.com/mappu/miqt/qt6"

// iconPainter draws an icon in a square of the side at x, y, in the mode.
type iconPainter func(p *qt.QPainter, x, y, side float64, mode qt.QIcon__Mode)

// paintedIcon returns an icon that paint draws each time it is shown, so that it
// can follow the palette as it then is.
func paintedIcon(paint iconPainter) *qt.QIcon {
	return qt.NewQIcon5(newPaintedEngine(paint))
}

// newPaintedEngine returns the engine of a painted icon. Qt asks it for a copy of
// itself where an icon is changed.
func newPaintedEngine(paint iconPainter) *qt.QIconEngine {
	e := qt.NewQIconEngine()
	e.OnPaint(func(p *qt.QPainter, rect *qt.QRect, mode qt.QIcon__Mode, _ qt.QIcon__State) {
		paint(p, float64(rect.X()), float64(rect.Y()), float64(min(rect.Width(), rect.Height())), mode)
	})
	// Qt's own pixmap of an engine is not cleared before it is painted, so the
	// icon is drawn on one that is transparent. miqt hands Qt the pixmap
	// returned, which Qt copies once this returns: it is kept until the next is
	// asked for, as otherwise the garbage collector could free it first.
	var last *qt.QPixmap
	e.OnPixmap(func(_ func(*qt.QSize, qt.QIcon__Mode, qt.QIcon__State) *qt.QPixmap, size *qt.QSize, mode qt.QIcon__Mode, _ qt.QIcon__State) *qt.QPixmap {
		image := qt.NewQImage3(size.Width(), size.Height(), qt.QImage__Format_ARGB32_Premultiplied)
		defer image.Delete()
		image.Fill2(qt.Transparent)
		p := qt.NewQPainter2(image.QPaintDevice)
		paint(p, 0, 0, float64(min(size.Width(), size.Height())), mode)
		p.End()
		p.Delete()
		last = qt.QPixmap_FromImage(image)
		return last
	})
	e.OnClone(func() *qt.QIconEngine { return newPaintedEngine(paint) })
	return e
}
