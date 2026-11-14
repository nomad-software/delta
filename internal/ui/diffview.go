package ui

import (
	"github.com/nomad-software/qute/qtgui"
	"github.com/nomad-software/qute/qtwidgets"

	"github.com/nomad-software/delta/internal/diff"
)

// tabWidth is how many spaces wide a tab is.
const tabWidth = 4

// diffView shows a diff read-only, with its text coloured by what each line is.
type diffView struct {
	*qtwidgets.QPlainTextEdit
	added, removed, hunk string
	// raw is the diff given, and parsed what is shown of it.
	raw    string
	parsed diff.Parsed
}

func newDiffView(added, removed, hunk string) *diffView {
	v := &diffView{QPlainTextEdit: qtwidgets.NewQPlainTextEdit(nil), added: added, removed: removed, hunk: hunk}
	v.SetReadOnly(true)
	v.SetUndoRedoEnabled(false)
	v.SetLineWrapMode(qtwidgets.QPlainTextEditNoWrap)
	return v
}

// show shows the diff, parsing it here. A diff read in the background is parsed
// there too, as diff.Parse touches nothing of Qt, and shown with showParsed,
// leaving the main thread only the widget to fill.
func (v *diffView) show(text string) {
	v.showParsed(text, diff.Parse(text))
}

// showParsed shows the diff, already parsed. Showing the same text again leaves
// the view as it is, scrolled where it was, as happens on every rescan.
func (v *diffView) showParsed(text string, parsed diff.Parsed) {
	if text == v.raw {
		return
	}
	v.raw = text
	v.parsed = parsed
	v.SetPlainText(v.parsed.Text)
	v.paint()
}

// setCodeFont changes the font, and with it how wide a tab is.
func (v *diffView) setCodeFont(font *qtgui.QFont) {
	v.SetFont(font)
	v.SetTabStopDistance(float64(tabWidth * v.FontMetrics().HorizontalAdvance(" ")))
}

// setColours changes the colours of added and removed lines and hunk ranges.
func (v *diffView) setColours(added, removed, hunk string) {
	if added == v.added && removed == v.removed && hunk == v.hunk {
		return
	}
	v.added, v.removed, v.hunk = added, removed, hunk
	// Formats are only ever added to, so old ones are cleared by starting again,
	// which would lose the scroll position.
	scroll, across := v.VerticalScrollBar().Value(), v.HorizontalScrollBar().Value()
	v.SetPlainText(v.parsed.Text)
	v.paint()
	v.VerticalScrollBar().SetValue(scroll)
	v.HorizontalScrollBar().SetValue(across)
}

// paint colours the text of each line by what it is: the whole of an added or
// removed line, and the @@ range of a hunk's heading, leaving any text after it
// as it is. Backgrounds are left alone, and what has no colour of its own
// follows the palette.
func (v *diffView) paint() {
	added, removed, hunk := colourFormat(v.added), colourFormat(v.removed), colourFormat(v.hunk)

	doc := v.Document()
	cursor := qtgui.NewQTextCursorWithTextDocument(doc)
	cursor.BeginEditBlock()
	defer cursor.EndEditBlock()
	// format applies to the part of the block up to its length.
	format := func(block *qtgui.QTextBlock, length int, f *qtgui.QTextCharFormat) {
		cursor.SetPosition(block.Position())
		cursor.SetPositionFull(block.Position()+length, qtgui.QTextCursorKeepAnchor)
		cursor.MergeCharFormat(f)
	}
	block := doc.Begin()
	for i, kind := range v.parsed.Kinds {
		if !block.IsValid() {
			break
		}
		switch kind {
		case diff.Added:
			format(block, block.Length()-1, added)
		case diff.Removed:
			format(block, block.Length()-1, removed)
		case diff.Hunk:
			// The range is plain ASCII, so its length is the same in Qt's terms.
			format(block, v.parsed.RangeEnds[i], hunk)
		}
		block = block.Next()
	}
}

// colourFormat returns a format of text in the colour.
func colourFormat(name string) *qtgui.QTextCharFormat {
	format := qtgui.NewQTextCharFormat()
	format.SetForeground(qtgui.NewQBrushWithColor(colour(name)))
	return format
}
