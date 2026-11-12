package ui

import (
	"github.com/nomad-software/qute/qtgui"

	"github.com/nomad-software/delta/internal/appicon"
)

// newAppIcon returns the program's icon, from the picture built into it.
func newAppIcon() *qtgui.QIcon {
	pixmap := qtgui.NewQPixmap()
	pixmap.LoadFromData(appicon.PNG)
	return qtgui.NewQIconWithPixmap(pixmap)
}
