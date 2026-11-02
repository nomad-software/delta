package ui

import qt "github.com/mappu/miqt/qt6"

const (
	// appIconSize is the size the icon is designed at, and drawn at for the
	// window and the AppImage.
	appIconSize = 128
	// macIconSize and macIconMargin are the size of the macOS app's icon and
	// the clear margin of Apple's icon grid.
	macIconSize   = 1024
	macIconMargin = 100
)

// newAppIcon returns the program's icon.
func newAppIcon() *qt.QIcon {
	pixmap := drawIcon(appIconSize, 0)
	defer pixmap.Delete()
	return qt.NewQIcon2(pixmap)
}

// drawIcon draws the icon at a size, within a clear margin: on a green tile, a
// side branch that leaves a main line and merges back into it.
func drawIcon(size, margin int) *qt.QPixmap {
	pixmap := qt.NewQPixmap2(size, size)
	transparent := qt.NewQColor11(0, 0, 0, 0)
	defer transparent.Delete()
	pixmap.FillWithFillColor(transparent)

	p := qt.NewQPainter2(pixmap.QPaintDevice)
	defer p.Delete()
	p.SetRenderHint(qt.QPainter__Antialiasing)
	p.Translate2(float64(margin), float64(margin))
	scale := float64(size-2*margin) / appIconSize
	p.Scale(scale, scale)

	p.SetPenWithStyle(qt.NoPen)
	green := qt.NewQColor6("#2e8b57")
	defer green.Delete()
	tile := qt.NewQBrush3(green)
	defer tile.Delete()
	p.SetBrush(tile)
	p.DrawRoundedRect2(4, 4, 120, 120, 24, 24)

	whiteColour := qt.NewQColor6("#ffffff")
	defer whiteColour.Delete()
	white := qt.NewQBrush3(whiteColour)
	defer white.Delete()
	line := qt.NewQPen4(white, 9)
	defer line.Delete()
	p.SetPenWithPen(line)
	p.DrawLine2(44, 22, 44, 106)
	p.DrawLine2(44, 42, 84, 64)
	p.DrawLine2(84, 64, 44, 86)

	p.SetPenWithStyle(qt.NoPen)
	p.SetBrush(white)
	for _, c := range [][2]int{{44, 24}, {44, 104}, {84, 64}} {
		p.DrawEllipse2(c[0]-11, c[1]-11, 22, 22)
	}
	p.End()
	return pixmap
}
