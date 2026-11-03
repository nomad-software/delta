package ui

import (
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"

	qt "github.com/mappu/miqt/qt6"
)

// The about box's title is the name at titleScale of the normal size, with the
// version beside it at versionScale and versionOpacity, as apemail's is.
const (
	titleScale     = 1.5
	versionScale   = 0.8
	versionOpacity = 0.5
)

// version returns the version Go recorded from git when the program was built,
// as the release it is or follows.
func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return release(info.Main.Version)
	}
	return "dev"
}

// release returns the release a version Go recorded is, or follows. Between
// tags Go makes a version of the commit whose patch is one past the tag before
// it, as v1.2.4-0.20261027184954-00c19a9187bb follows v1.2.3.
func release(version string) string {
	version, _, _ = strings.Cut(version, "+")
	version, pre, _ := strings.Cut(version, "-")
	if !strings.HasPrefix(pre, "0.") {
		return version
	}
	dot := strings.LastIndex(version, ".") + 1
	patch, err := strconv.Atoi(version[dot:])
	if err != nil || patch == 0 {
		return version
	}
	return version[:dot] + strconv.Itoa(patch-1)
}

// newAboutTitle returns the name of the program with its version to the right
// of it, smaller and half transparent, on one baseline, and the version's label.
func newAboutTitle() (*qt.QHBoxLayout, *qt.QLabel) {
	name := qt.NewQLabel3("Delta")
	font := qt.NewQFont5(name.Font())
	defer font.Delete()
	font.SetBold(true)
	font.SetPointSizeF(font.PointSizeF() * titleScale)
	name.SetFont(font)

	label := qt.NewQLabel3(version())
	label.SetTextFormat(qt.PlainText)
	small := qt.NewQFont5(label.Font())
	defer small.Delete()
	small.SetPointSizeF(small.PointSizeF() * versionScale)
	label.SetFont(small)
	// Both sit on the bottom, so the smaller is raised by what its letters
	// hang below the line less than the name's do.
	nameMetrics, labelMetrics := name.FontMetrics(), label.FontMetrics()
	label.SetContentsMargins(0, 0, 0, nameMetrics.Descent()-labelMetrics.Descent())
	runtime.KeepAlive(nameMetrics)
	runtime.KeepAlive(labelMetrics)
	faded := qt.NewQGraphicsOpacityEffect()
	faded.SetOpacity(versionOpacity)
	label.SetGraphicsEffect(faded.QGraphicsEffect)

	title := qt.NewQHBoxLayout2()
	title.AddWidget3(name.QWidget, 0, qt.AlignBottom)
	title.AddWidget3(label.QWidget, 0, qt.AlignBottom)
	title.AddStretch()
	return title, label
}

// newAboutDialog returns the about box: the icon, what the program is, and the
// licences of what it is built with.
func newAboutDialog(parent *qt.QWidget, icon *qt.QIcon) *qt.QDialog {
	d := qt.NewQDialog(parent)
	d.SetWindowTitle("About Delta")
	d.SetMinimumWidth(aboutWidth)

	picture := qt.NewQLabel2()
	picture.SetPixmap(icon.PixmapWithExtent(96))
	picture.SetAlignment(qt.AlignTop)

	title, _ := newAboutTitle()

	summary := linkLabel("<p>A graphical user interface for git.</p>" +
		"<p>Copyright © 2026 Gary Willoughby. Released under the " +
		`<a href="https://www.gnu.org/licenses/gpl-3.0.html">GNU General Public License version 3</a>.</p>`)

	text := qt.NewQVBoxLayout2()
	text.SetSpacing(12)
	text.AddLayout(title.QLayout)
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

	fitHeight(d, aboutWidth)
	d.OnFinished(func(int) { d.DeleteLater() })
	return d
}

// aboutWidth is the narrowest the about box is.
const aboutWidth = 560

// fitHeight sizes a dialog to the width, or the least it can be, and to the
// height its contents need at that width, as apemail's are. The height Qt
// would give it is that of its text wrapped narrower, which leaves a gap.
func fitHeight(d *qt.QDialog, width int) {
	layout := d.Layout()
	layout.Activate()
	// The size is miqt's to free, by its finalizer, as Qt gives it by value.
	least := layout.MinimumSize()
	width = max(width, least.Width())
	d.Resize(width, max(layout.HeightForWidth(width), least.Height()))
	runtime.KeepAlive(least)
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
