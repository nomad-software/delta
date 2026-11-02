package ui

import (
	qt "github.com/mappu/miqt/qt6"

	"github.com/nomad-software/merge-monkey/internal/diff"
)

// How much of a colour goes into the background of a line, in percent. The
// colours are mixed into the palette's background rather than used as they are,
// so that one colour reads well, under the palette's text, in light and dark
// themes alike.
const (
	changeTint = 30
	hunkTint   = 14
	metaTint   = 6
)

// diffView shows a diff read-only, with whole lines tinted by what they are. A
// block's background is used, as only that reaches the full width of the view;
// a syntax highlighter can only colour behind the text.
type diffView struct {
	*qt.QPlainTextEdit
	added, removed string
	// text is what is shown, and painted says what colours it is drawn in.
	text    string
	kinds   []diff.Kind
	painted string
}

func newDiffView(added, removed string) *diffView {
	v := &diffView{QPlainTextEdit: qt.NewQPlainTextEdit2(), added: added, removed: removed}
	v.SetReadOnly(true)
	v.SetUndoRedoEnabled(false)
	v.SetLineWrapMode(qt.QPlainTextEdit__NoWrap)
	font := qt.QFontDatabase_SystemFont(qt.QFontDatabase__FixedFont)
	v.SetFont(font)
	v.SetTabStopDistance(float64(4 * v.FontMetrics().HorizontalAdvance(" ")))
	// The tints are worked out from the palette, which the theme can change.
	v.OnChangeEvent(func(super func(*qt.QEvent), event *qt.QEvent) {
		super(event)
		if t := event.Type(); t == qt.QEvent__PaletteChange || t == qt.QEvent__ApplicationPaletteChange {
			v.recolour()
		}
	})
	return v
}

// show shows the diff. Showing the same text again leaves the view as it is,
// scrolled where it was, as happens on every rescan.
func (v *diffView) show(text string) {
	text = diff.Clean(text)
	if text == v.text {
		return
	}
	v.text = text
	v.kinds = diff.Classify(text)
	v.SetPlainText(text)
	v.paint()
}

// setColours changes the colours of added and removed lines.
func (v *diffView) setColours(added, removed string) {
	v.added, v.removed = added, removed
	v.recolour()
}

// recolour draws the lines again if their colours have changed.
func (v *diffView) recolour() {
	if v.painted == v.colourKey() {
		return
	}
	// Formats are only ever added to, so old ones are cleared by starting
	// again. The scroll position would be lost with them.
	scroll := v.VerticalScrollBar().Value()
	v.SetPlainText(v.text)
	v.paint()
	v.VerticalScrollBar().SetValue(scroll)
}

// colourKey names what the tints are made from.
func (v *diffView) colourKey() string {
	p := qt.QGuiApplication_Palette()
	return v.added + v.removed + p.ColorWithCr(qt.QPalette__Base).Name() + p.ColorWithCr(qt.QPalette__Text).Name()
}

// paint gives each line that is not unchanged its background.
func (v *diffView) paint() {
	v.painted = v.colourKey()
	p := qt.QGuiApplication_Palette()
	base, text := p.ColorWithCr(qt.QPalette__Base), p.ColorWithCr(qt.QPalette__Text)
	formats := map[diff.Kind]*qt.QTextBlockFormat{
		diff.Added:   tintFormat(mix(qt.NewQColor6(v.added), base, changeTint)),
		diff.Removed: tintFormat(mix(qt.NewQColor6(v.removed), base, changeTint)),
		diff.Hunk:    tintFormat(mix(text, base, hunkTint)),
		diff.Meta:    tintFormat(mix(text, base, metaTint)),
	}
	doc := v.Document()
	cursor := qt.NewQTextCursor2(doc)
	defer cursor.Delete()
	cursor.BeginEditBlock()
	defer cursor.EndEditBlock()
	block := doc.Begin()
	for _, kind := range v.kinds {
		if !block.IsValid() {
			break
		}
		if format := formats[kind]; format != nil {
			cursor.SetPosition(block.Position())
			cursor.SetBlockFormat(format)
		}
		block = block.Next()
	}
}

// tintFormat returns the format of a block with the background.
func tintFormat(colour *qt.QColor) *qt.QTextBlockFormat {
	format := qt.NewQTextBlockFormat()
	format.SetBackground(qt.NewQBrush3(colour))
	return format
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
