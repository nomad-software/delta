package ui

import (
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtgui"
	"github.com/nomad-software/qute/qtwidgets"
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
func newAboutTitle() (*qtwidgets.QHBoxLayout, *qtwidgets.QLabel) {
	name := qtwidgets.NewQLabelWithText("Delta", nil)
	font := name.Font()
	font.SetBold(true)
	font.SetPointSizeF(font.PointSizeF() * titleScale)
	name.SetFont(font)

	label := qtwidgets.NewQLabelWithText(version(), nil)
	label.SetTextFormat(qtcore.PlainText)
	small := label.Font()
	small.SetPointSizeF(small.PointSizeF() * versionScale)
	label.SetFont(small)
	// Both sit on the bottom, so the smaller is raised by what its letters
	// hang below the line less than the name's do.
	label.SetContentsMargins(qtcore.QMargins{Bottom: name.FontMetrics().Descent() - label.FontMetrics().Descent()})
	faded := qtwidgets.NewQGraphicsOpacityEffect(nil)
	faded.SetOpacity(versionOpacity)
	label.SetGraphicsEffect(faded)

	title := qtwidgets.NewQHBoxLayout(nil)
	title.AddWidgetFull(name, 0, qtcore.AlignBottom)
	title.AddWidgetFull(label, 0, qtcore.AlignBottom)
	title.AddStretch()
	return title, label
}

// newAboutDialog returns the about box: the icon, what the program is, and the
// licences of what it is built with.
func newAboutDialog(parent qtwidgets.QWidgetLike, icon *qtgui.QIcon) *qtwidgets.QDialog {
	d := qtwidgets.NewQDialog(parent)
	d.SetWindowTitle("About Delta")
	d.SetMinimumWidth(aboutWidth)

	picture := qtwidgets.NewQLabel(nil)
	picture.SetPixmap(icon.PixmapExtent(96))
	picture.SetAlignment(qtcore.AlignTop)

	title, _ := newAboutTitle()

	summary := linkLabel("<p>A graphical user interface for git.</p>" +
		"<p>Copyright © 2026 Gary Willoughby. Released under the " +
		`<a href="https://www.gnu.org/licenses/gpl-3.0.html">GNU General Public License version 3</a>.</p>`)

	text := qtwidgets.NewQVBoxLayout(nil)
	text.SetSpacing(12)
	text.AddLayout(title)
	text.AddWidget(summary)
	text.AddStretch()

	top := qtwidgets.NewQHBoxLayout(nil)
	top.SetSpacing(20)
	top.AddWidget(picture)
	top.AddLayoutFull(text, 1)

	qtVersion := qtcore.QLibraryInfoVersion()
	notices := linkLabel("<p><b>Third party licences</b></p>" +
		"<p>This program uses Qt " + qtVersion.ToString() + ", under the terms of the " +
		`<a href="https://www.gnu.org/licenses/lgpl-3.0.html">GNU Lesser General Public License version 3</a>. ` +
		"Qt is linked dynamically, so it can be replaced with another build of the library. " +
		`Qt and its source code are available from <a href="https://www.qt.io">www.qt.io</a>. ` +
		"Copyright © The Qt Company Ltd. and other contributors.</p>" +
		`<p>The Go bindings for Qt are <a href="https://github.com/nomad-software/qute">qute</a>, under the ` +
		`<a href="https://www.gnu.org/licenses/gpl-3.0.html">GNU General Public License version 3</a>.</p>`)
	frame := qtwidgets.NewQFrame(nil)
	frame.SetFrameShape(qtwidgets.QFrameStyledPanel)
	frame.SetFrameShadow(qtwidgets.QFrameSunken)
	frameLayout := qtwidgets.NewQVBoxLayout(frame)
	frameLayout.AddWidget(notices)

	buttons := qtwidgets.NewQDialogButtonBoxWithStandardButton(qtwidgets.QDialogButtonBoxOk, nil)
	buttons.OnAccepted(d.Accept)

	layout := qtwidgets.NewQVBoxLayout(d)
	layout.SetContentsMargins(qtcore.QMargins{Left: 20, Top: 20, Right: 20, Bottom: 16})
	layout.SetSpacing(16)
	layout.AddLayout(top)
	layout.AddWidget(frame)
	layout.AddWidget(buttons)

	fitHeight(d, aboutWidth)
	d.OnFinished(func(int) { d.DeleteLater() })
	return d
}

// aboutWidth is the narrowest the about box is.
const aboutWidth = 560

// linkLabel returns a label of wrapped rich text whose links open in a browser.
func linkLabel(text string) *qtwidgets.QLabel {
	label := qtwidgets.NewQLabelWithText(text, nil)
	label.SetTextFormat(qtcore.RichText)
	label.SetWordWrap(true)
	label.SetOpenExternalLinks(true)
	label.SetTextInteractionFlags(qtcore.TextBrowserInteraction)
	return label
}
