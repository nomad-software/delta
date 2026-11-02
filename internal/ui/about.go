package ui

import (
	"runtime/debug"
	"strings"

	qt "github.com/mappu/miqt/qt6"
)

// version returns the version Go recorded from git when the program was built.
func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		v, _, _ := strings.Cut(info.Main.Version, "+")
		return v
	}
	return "dev"
}

// newAboutDialog returns the about box: the icon, what the program is, and the
// licences of what it is built with.
func newAboutDialog(parent *qt.QWidget, icon *qt.QIcon) *qt.QDialog {
	d := qt.NewQDialog(parent)
	d.SetWindowTitle("About Delta")
	d.SetMinimumWidth(560)

	picture := qt.NewQLabel2()
	picture.SetPixmap(icon.PixmapWithExtent(96))
	picture.SetAlignment(qt.AlignTop)

	title := qt.NewQLabel3("Delta")
	titleFont := qt.NewQFont5(title.Font())
	defer titleFont.Delete()
	titleFont.SetBold(true)
	titleFont.SetPointSizeF(titleFont.PointSizeF() * 1.5)
	title.SetFont(titleFont)

	versionLabel := qt.NewQLabel3("Version " + version())
	versionLabel.SetTextFormat(qt.PlainText)

	summary := linkLabel("<p>A graphical user interface for git.</p>" +
		"<p>Copyright © 2026 Gary Willoughby. Released under the " +
		`<a href="https://www.gnu.org/licenses/gpl-3.0.html">GNU General Public License version 3</a>.</p>`)

	text := qt.NewQVBoxLayout2()
	text.AddWidget(title.QWidget)
	text.AddWidget(versionLabel.QWidget)
	text.AddWidget(summary.QWidget)
	text.AddStretch()

	top := qt.NewQHBoxLayout2()
	top.SetSpacing(20)
	top.AddWidget(picture.QWidget)
	top.AddLayout2(text.QLayout, 1)

	qtVersion := qt.QLibraryInfo_Version()
	notices := linkLabel("<p><b>Third party licences</b></p>" +
		"<p>This program uses Qt " + qtVersion.ToString() + ", under the terms of the " +
		`<a href="https://www.gnu.org/licenses/lgpl-3.0.html">GNU Lesser General Public License version 3</a>. ` +
		"Qt is linked dynamically, so it can be replaced with another build of the library. " +
		`Qt and its source code are available from <a href="https://www.qt.io">www.qt.io</a>. ` +
		"Copyright © The Qt Company Ltd. and other contributors.</p>" +
		`<p>The Go bindings for Qt are <a href="https://github.com/mappu/miqt">miqt</a>, under the MIT licence.</p>`)
	frame := qt.NewQFrame2()
	frame.SetFrameShape(qt.QFrame__StyledPanel)
	frame.SetFrameShadow(qt.QFrame__Sunken)
	frameLayout := qt.NewQVBoxLayout(frame.QWidget)
	frameLayout.AddWidget(notices.QWidget)

	buttons := qt.NewQDialogButtonBox4(qt.QDialogButtonBox__Ok)
	buttons.OnAccepted(d.Accept)

	layout := qt.NewQVBoxLayout(d.QWidget)
	layout.SetContentsMargins(20, 20, 20, 16)
	layout.SetSpacing(16)
	layout.AddLayout(top.QLayout)
	layout.AddWidget(frame.QWidget)
	layout.AddWidget(buttons.QWidget)

	d.OnFinished(func(int) { d.DeleteLater() })
	return d
}

// linkLabel returns a label of wrapped rich text whose links open in a browser.
func linkLabel(text string) *qt.QLabel {
	label := qt.NewQLabel3(text)
	label.SetTextFormat(qt.RichText)
	label.SetWordWrap(true)
	label.SetOpenExternalLinks(true)
	label.SetTextInteractionFlags(qt.TextBrowserInteraction)
	return label
}
