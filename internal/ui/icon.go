package ui

import qt "github.com/mappu/miqt/qt6"

// appIconSize is the size the program's icon is drawn at.
const appIconSize = 128

// newAppIcon draws the program's icon: on a green tile, a side branch that
// leaves a main line and merges back into it.
func newAppIcon() *qt.QIcon {
	pixmap := qt.NewQPixmap2(appIconSize, appIconSize)
	defer pixmap.Delete()
	clear := qt.NewQColor11(0, 0, 0, 0)
	defer clear.Delete()
	pixmap.FillWithFillColor(clear)

	p := qt.NewQPainter2(pixmap.QPaintDevice)
	defer p.Delete()
	p.SetRenderHint(qt.QPainter__Antialiasing)

	p.SetPenWithStyle(qt.NoPen)
	p.SetBrush(qt.NewQBrush3(qt.NewQColor6("#2e8b57")))
	p.DrawRoundedRect2(4, 4, 120, 120, 24, 24)

	white := qt.NewQBrush3(qt.NewQColor6("#ffffff"))
	line := qt.NewQPen4(white, 9)
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
	return qt.NewQIcon2(pixmap)
}
