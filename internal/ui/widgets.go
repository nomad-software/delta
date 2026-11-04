package ui

import (
	"strings"

	qt "github.com/mappu/miqt/qt6"

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

// newPaneSplitter returns a splitter whose handles are a gap between the panes
// with a row of dots at its centre, rather than what the style draws, which on
// some is coloured. A handle's painting can only be replaced on one made here.
func newPaneSplitter(orientation qt.Orientation) *qt.QSplitter {
	s := qt.NewQSplitter3(orientation)
	s.SetHandleWidth(paneHandleWidth)
	s.OnCreateHandle(func(func() *qt.QSplitterHandle) *qt.QSplitterHandle {
		handle := qt.NewQSplitterHandle(s.Orientation(), s)
		handle.OnPaintEvent(func(func(*qt.QPaintEvent), *qt.QPaintEvent) { paintGrip(handle) })
		return handle
	})
	return s
}

// paintGrip draws the dots at the centre of a handle, in a colour half way
// between the window and its text, which shows in any theme.
func paintGrip(handle *qt.QSplitterHandle) {
	palette := handle.Palette()
	colour := mix(palette.ColorWithCr(qt.QPalette__WindowText), palette.ColorWithCr(qt.QPalette__Window), 50)
	defer colour.Delete()
	painter := qt.NewQPainter2(handle.QPaintDevice)
	defer painter.Delete()
	span := gripDots*gripDotSize + (gripDots-1)*gripDotGap
	for i := range gripDots {
		along := i * (gripDotSize + gripDotGap)
		x, y := (handle.Width()-span)/2+along, (handle.Height()-gripDotSize)/2
		if handle.Orientation() == qt.Horizontal {
			x, y = (handle.Width()-gripDotSize)/2, (handle.Height()-span)/2+along
		}
		painter.FillRect5(x, y, gripDotSize, gripDotSize, colour)
	}
	painter.End()
}

// mix returns the percentage of the colour mixed into the background.
func mix(colour, background *qt.QColor, percent int) *qt.QColor {
	channel := func(c, b int) int { return (c*percent + b*(100-percent)) / 100 }
	return qt.NewQColor3(
		channel(colour.Red(), background.Red()),
		channel(colour.Green(), background.Green()),
		channel(colour.Blue(), background.Blue()),
	)
}

// labelled returns the widget with a label above it.
func labelled(label *qt.QLabel, widget *qt.QWidget) *qt.QWidget {
	pane := qt.NewQWidget2()
	layout := qt.NewQVBoxLayout(pane)
	layout.SetContentsMargins(0, 0, 0, 0)
	layout.SetSpacing(4)
	layout.AddWidget(label.QWidget)
	layout.AddWidget2(widget, 1)
	return pane
}

// setPanes sizes the panes of a splitter.
func setPanes(s *qt.QSplitter, panes settings.Panes) {
	s.SetSizes(panes[:])
}

// panes returns the sizes of the panes of a splitter.
func panes(s *qt.QSplitter) settings.Panes {
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
func newChangeItem(c git.Change) *qt.QListWidgetItem {
	item := qt.NewQListWidgetItem2(describe(c))
	item.SetIcon(changeIcon(c.Code))
	return item
}

// setChangeIconSize gives a list's icons the size of the circles of changes.
func setChangeIconSize(list *qt.QListWidget) {
	size := qt.NewQSize2(changeDotSize, changeDotSize)
	defer size.Delete()
	list.SetIconSize(size)
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
func heading(text string) *qt.QLabel {
	label := qt.NewQLabel3(text)
	font := qt.NewQFont5(label.Font())
	defer font.Delete()
	font.SetBold(true)
	label.SetFont(font)
	return label
}

// fallbackCodeFontSize is the size of the code's font in points when the system's
// has none to go by.
const fallbackCodeFontSize = 10

// codeFont returns a new font of the family and size in points, each the system's
// fixed width font's if it is not given.
func codeFont(family string, size int) *qt.QFont {
	font := qt.NewQFont5(qt.QFontDatabase_SystemFont(qt.QFontDatabase__FixedFont))
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
func fitHeight(d *qt.QDialog, width int) {
	layout := d.Layout()
	layout.Activate()
	// The size is miqt's to free, by its finalizer, as Qt gives it by value.
	least := layout.MinimumSize()
	width = max(width, least.Width())
	d.Resize(width, max(layout.HeightForWidth(width), least.Height()))
}
