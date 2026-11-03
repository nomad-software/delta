package ui

import (
	qt "github.com/mappu/miqt/qt6"

	"github.com/nomad-software/delta/internal/appicon"
)

// macIconSize and macIconMargin are the size of the macOS app's icon and the
// clear margin of Apple's icon grid.
const (
	macIconSize   = 1024
	macIconMargin = 100
)

// newAppIcon returns the program's icon, from the picture built into it.
func newAppIcon() *qt.QIcon {
	pixmap := appIconPixmap()
	defer pixmap.Delete()
	return qt.NewQIcon2(pixmap)
}

// appIconPixmap returns a new pixmap of the program's icon.
func appIconPixmap() *qt.QPixmap {
	pixmap := qt.NewQPixmap()
	pixmap.LoadFromDataWithData(appicon.PNG)
	return pixmap
}

// drawMacIcon draws the program's icon for the macOS app: at the size, within
// the clear margin, smoothly scaled.
func drawMacIcon(size, margin int) *qt.QPixmap {
	icon := appIconPixmap()
	defer icon.Delete()
	pixmap := qt.NewQPixmap2(size, size)
	transparent := qt.NewQColor11(0, 0, 0, 0)
	defer transparent.Delete()
	pixmap.FillWithFillColor(transparent)
	p := qt.NewQPainter2(pixmap.QPaintDevice)
	defer p.Delete()
	p.SetRenderHint(qt.QPainter__SmoothPixmapTransform)
	target := qt.NewQRect4(margin, margin, size-2*margin, size-2*margin)
	defer target.Delete()
	p.DrawPixmap10(target, icon)
	p.End()
	return pixmap
}
