package ui

import (
	"math"
	"strings"

	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtgui"
	"github.com/nomad-software/qute/qtwidgets"

	"github.com/nomad-software/delta/internal/git"
	"github.com/nomad-software/delta/internal/settings"
)

// pageMargin is the space around the changes and the history.
const pageMargin = 8

// The handles between panes are paneHandleWidth pixels thick, which also serves
// as the gap between the panes, with a grip of gripDots dots of gripDotSize
// pixels, gripDotGap apart.
const (
	paneHandleWidth = 10
	gripDots        = 5
	gripDotSize     = 2
	gripDotGap      = 2
)

// paneSplitter is a splitter whose handles are a gap between the panes with a
// row of dots at its centre, rather than what the style draws, which on some is
// coloured.
type paneSplitter struct{ qtwidgets.QSplitterBase }

// newPaneSplitter returns a splitter whose handles are a gap with a grip.
func newPaneSplitter(orientation qtcore.Orientation) *qtwidgets.QSplitter {
	s := &paneSplitter{}
	qtwidgets.NewQSplitterWithOrientationWith(s, orientation, nil)
	s.SetHandleWidth(paneHandleWidth)
	return s.QSplitter
}

// CreateHandle makes each handle of the splitter one that paints its grip.
func (s *paneSplitter) CreateHandle() *qtwidgets.QSplitterHandle {
	return qtwidgets.NewQSplitterHandleWith(&gripHandle{}, s.Orientation(), s)
}

// gripHandle is a handle between panes that is painted as its grip only.
type gripHandle struct{ qtwidgets.QSplitterHandleBase }

// PaintEvent draws the dots at the centre of a handle, in a colour half way
// between the window and its text, which shows in any theme.
func (h *gripHandle) PaintEvent(*qtgui.QPaintEvent) {
	palette := h.Palette()
	colour := mix(palette.Color(qtgui.QPaletteWindowText), palette.Color(qtgui.QPaletteWindow), 50)
	painter := qtgui.NewQPainterOn(h)
	defer painter.End()
	span := gripDots*gripDotSize + (gripDots-1)*gripDotGap
	for i := range gripDots {
		along := i * (gripDotSize + gripDotGap)
		x, y := (h.Width()-span)/2+along, (h.Height()-gripDotSize)/2
		if h.Orientation() == qtcore.Horizontal {
			x, y = (h.Width()-gripDotSize)/2, (h.Height()-span)/2+along
		}
		painter.FillRect(qtcore.QRect{X: x, Y: y, Width: gripDotSize, Height: gripDotSize}, colour)
	}
}

// mix returns the percentage of the colour mixed into the background.
func mix(colour, background qtgui.QColor, percent int) qtgui.QColor {
	channel := func(c, b uint8) uint8 { return uint8((int(c)*percent + int(b)*(100-percent)) / 100) }
	return qtgui.RGB(channel(colour.R, background.R), channel(colour.G, background.G), channel(colour.B, background.B))
}

// colour returns the colour of a name of the form #rrggbb, or else an invalid
// one, as Qt gives for a name it does not know.
func colour(name string) qtgui.QColor {
	c, err := qtgui.ParseColor(name)
	if err != nil {
		return qtgui.QColor{}
	}
	return c
}

// alpha returns the opacity of the fraction, out of 255, as Qt rounds it where it
// paints.
func alpha(fraction float64) uint8 {
	return uint8(math.Round(fraction * 255))
}

// lighter returns the colour made lighter by the factor, in percent, as Qt's
// QColor::lighter does for a colour it does not take past white: its value, and
// so each channel, is scaled in Qt's sixteen bits a channel, then rounded back
// to eight.
func lighter(c qtgui.QColor, factor int) qtgui.QColor {
	channel := func(v uint8) uint8 {
		wide := uint32(v) * 257 * uint32(factor) / 100
		return uint8(min((wide-(wide>>8)+0x80)>>8, 255))
	}
	return qtgui.QColor{R: channel(c.R), G: channel(c.G), B: channel(c.B), A: c.A}
}

// labelled returns the widget with a label above it.
func labelled(label *qtwidgets.QLabel, widget qtwidgets.QWidgetLike) *qtwidgets.QWidget {
	pane := qtwidgets.NewQWidget(nil)
	layout := qtwidgets.NewQVBoxLayout(pane)
	layout.SetContentsMargins(qtcore.QMargins{})
	layout.SetSpacing(4)
	layout.AddWidget(label)
	layout.AddWidgetFull(widget, 1, 0)
	return pane
}

// setPanes sizes the panes of a splitter.
func setPanes(s *qtwidgets.QSplitter, panes settings.Panes) {
	s.SetSizes(panes[:])
}

// panes returns the sizes of the panes of a splitter.
func panes(s *qtwidgets.QSplitter) settings.Panes {
	sizes := s.Sizes()
	return settings.Panes{sizes[0], sizes[1]}
}

// describe returns the text of a change in a list: its path, after the one it
// was renamed or copied from. What happened to it is its icon (changeIcon).
func describe(c git.Change) string {
	if c.OldPath != "" {
		return c.OldPath + " → " + c.Path
	}
	return c.Path
}

// newChangeItem returns the row of a list showing a change: its circle and its
// path. The staged and unstaged lists' rows have no tooltip (the user's choice).
func newChangeItem(c git.Change) *qtwidgets.QListWidgetItem {
	item := qtwidgets.NewQListWidgetItemWithText(describe(c), nil)
	item.SetIcon(changeIcon(c.Code))
	return item
}

// setChangeIconSize gives a list's icons the size of the circles of changes.
func setChangeIconSize(list *qtwidgets.QListWidget) {
	list.SetIconSize(qtcore.QSize{Width: changeDotSize, Height: changeDotSize})
}

// explain returns what the code of a change means.
func explain(c git.Change) string {
	switch c.Code {
	case 'M':
		return "Modified"
	case 'A':
		return "Added"
	case 'D':
		return "Deleted"
	case 'R':
		return "Renamed"
	case 'C':
		return "Copied"
	case 'T':
		return "Type changed"
	case git.Untracked:
		return "Untracked"
	case git.Conflict:
		return "In conflict: stage it once it is resolved"
	}
	return ""
}

// upperFirst returns the text with its first letter a capital.
func upperFirst(text string) string {
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}

// heading returns a label for a section.
func heading(text string) *qtwidgets.QLabel {
	label := qtwidgets.NewQLabelWithText(text, nil)
	font := label.Font()
	font.SetBold(true)
	label.SetFont(font)
	return label
}

// fallbackCodeFontSize is the size of the code's font in points when the system's
// has none to go by.
const fallbackCodeFontSize = 10

// codeFont returns a new font of the family and size in points, each the system's
// fixed width font's if it is not given.
func codeFont(family string, size int) *qtgui.QFont {
	font := qtgui.QFontDatabaseGetSystemFont(qtgui.QFontDatabaseFixedFont)
	if family != "" {
		font.SetFamily(family)
	}
	switch {
	case size > 0:
		font.SetPointSize(size)
	case font.PointSize() < settings.MinCodeFontSize:
		// A font set in pixels has no size in points.
		font.SetPointSize(fallbackCodeFontSize)
	}
	return font
}

// fitHeight sizes a dialog to the width, or the least it can be, and to the
// height its contents need at that width, as apemail's are. The height Qt
// would give it is that of its text wrapped narrower, which leaves a gap.
func fitHeight(d *qtwidgets.QDialog, width int) {
	width = max(width, fittedSize(d, width).Width)
	d.Resize(width, fittedSize(d, width).Height)
}

// fittedSize returns the least width the dialog's contents can have, and the
// height they need at the width.
func fittedSize(d *qtwidgets.QDialog, width int) qtcore.QSize {
	layout := d.Layout()
	layout.Activate()
	least := layout.MinimumSize()
	least.Height = max(layout.AsQLayoutItem().HeightForWidth(width), least.Height)
	return least
}

// keepFitted keeps a dialog with nothing to fill more height at the height its
// contents need at its width, whatever its width, so that making it taller
// leaves no gap under them: it is as tall as its notes need once wrapped, and
// widens and narrows freely. It is called whenever the dialog is resized.
func keepFitted(d *qtwidgets.QDialog) {
	height := fittedSize(d, d.Width()).Height
	if d.MinimumHeight() != height || d.MaximumHeight() != height {
		d.SetFixedHeight(height)
	}
}

// setSizePolicy sets how a widget takes up room across and down, keeping the
// kind of control it is, as QWidget::setSizePolicy does given the two.
func setSizePolicy(widget qtwidgets.QWidgetLike, horizontal, vertical qtwidgets.QSizePolicyPolicy) {
	w := widget.AsQWidget()
	policy := w.SizePolicy()
	policy.SetHorizontalPolicy(horizontal)
	policy.SetVerticalPolicy(vertical)
	w.SetSizePolicy(policy)
}

// margins returns margins of the same width on every side.
func margins(width int) qtcore.QMargins {
	return qtcore.QMargins{Left: width, Top: width, Right: width, Bottom: width}
}
