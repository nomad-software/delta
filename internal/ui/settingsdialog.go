package ui

import (
	"fmt"

	qt "github.com/mappu/miqt/qt6"

	"github.com/nomad-software/delta/internal/settings"
)

// The columns of the table of colours.
const (
	colColourName = iota
	colColourSwatch
	colourColumns
)

// colourTableHeight is the least height of the table of colours.
const colourTableHeight = 260

// settingsDialog edits the theme, the font of the code, the history, and the
// colours the program gives itself. As apemail's, each section is a frame with
// its name, a note on what it is for where it has one, and its fields; the
// colours are in a table in theirs.
type settingsDialog struct {
	*qt.QDialog
	tabs        *qt.QTabWidget
	theme       *qt.QComboBox
	codeFamily  *qt.QFontComboBox
	codeSize    *qt.QSpinBox
	codeDefault *qt.QPushButton
	// historyPage is how many commits of the history are read at a time.
	historyPage *qt.QSpinBox
	// defaultFamily and defaultSize are the system's fixed width font, as the
	// fields show it, which is what is meant when nothing else is chosen.
	defaultFamily        string
	defaultSize          int
	added, removed, hunk *colourButton
	// changes are the colours of the circles of changed files, in the order of
	// changeNames and ChangeColours.Fields, and progress that of the progress
	// bar's segment.
	changes  []*colourButton
	progress *colourButton
	// colours is the table of the colours.
	colours *qt.QTableWidget
	// sections are the frames of the sections, by title.
	sections map[string]*qt.QGroupBox
	box      *qt.QDialogButtonBox
}

// colourButton shows a colour, and chooses another when clicked.
type colourButton struct {
	*qt.QPushButton
	colour string
}

func newColourButton(title string) *colourButton {
	b := &colourButton{QPushButton: qt.NewQPushButton2()}
	b.SetMinimumWidth(140)
	b.OnClicked(func() {
		initial := qt.NewQColor6(b.colour)
		defer initial.Delete()
		// A cancelled dialog gives an invalid colour.
		if chosen := qt.QColorDialog_GetColor3(initial, b.QWidget, title); chosen.IsValid() {
			b.set(chosen.Name())
		}
	})
	return b
}

// set shows the colour, named on the button in whichever of Solarized's darkest
// and lightest can be read on it.
func (b *colourButton) set(colour string) {
	b.colour = colour
	c := qt.NewQColor6(colour)
	defer c.Delete()
	text := settings.Base3
	if c.Red()*299+c.Green()*587+c.Blue()*114 > 128000 {
		text = settings.Base03
	}
	b.SetText(colour)
	// With a border of its own, the button is drawn in the colour as it is,
	// rather than shaded by the style.
	b.SetStyleSheet(fmt.Sprintf("QPushButton { background-color: %s; color: %s; padding: 6px; border: 1px solid %s; border-radius: 3px; }", colour, text, text))
}

// settingsDialogWidth is the narrowest the dialog is, so that its notes have
// room.
const settingsDialogWidth = 560

// changeNames are what can happen to a file, as the dialog names them, in the
// order of ChangeColours.Fields.
var changeNames = []string{"Modified", "Added", "Deleted", "Renamed", "Copied", "Type changed", "In conflict", "Untracked"}

// newSettingsDialog returns the dialog showing the settings. accepted is called
// with what was chosen, on top of current, if it is accepted.
func newSettingsDialog(parent *qt.QWidget, current settings.Settings, accepted func(settings.Settings)) *settingsDialog {
	d := &settingsDialog{QDialog: qt.NewQDialog(parent), sections: map[string]*qt.QGroupBox{}}
	d.SetWindowTitle("Settings")
	d.SetMinimumWidth(settingsDialogWidth)

	d.theme = qt.NewQComboBox2()
	for _, t := range themeChoices {
		d.theme.AddItem(t.label)
	}
	d.codeFamily = qt.NewQFontComboBox2()
	d.codeSize = qt.NewQSpinBox2()
	d.codeSize.SetRange(settings.MinCodeFontSize, settings.MaxCodeFontSize)
	d.codeSize.SetSuffix(" pt")
	system := codeFont("", 0)
	defer system.Delete()
	d.codeFamily.SetCurrentFont(system)
	d.defaultFamily = d.codeFamily.CurrentText()
	d.defaultSize = system.PointSize()
	d.codeDefault = qt.NewQPushButton3("Default")
	d.codeDefault.SetToolTip("The system's fixed width font")
	d.codeDefault.OnClicked(func() { d.showFont("", 0) })
	codeRow := qt.NewQHBoxLayout2()
	codeRow.AddWidget2(d.codeFamily.QWidget, 1)
	codeRow.AddWidget(d.codeSize.QWidget)
	codeRow.AddWidget(d.codeDefault.QWidget)
	d.historyPage = qt.NewQSpinBox2()
	d.historyPage.SetRange(settings.MinHistoryPageSize, settings.MaxHistoryPageSize)
	d.historyPage.SetSingleStep(100)
	d.historyPage.SetSuffix(" commits")

	d.tabs = qt.NewQTabWidget2()
	page := qt.NewQWidget2()
	d.tabs.AddTab(page, "User interface")
	look := qt.NewQVBoxLayout(page)
	appearance := d.section(look, "Appearance", "")
	appearance.AddRow3("Theme:", d.theme.QWidget)
	appearance.AddRow4("Code font:", codeRow.QLayout)
	history := d.section(look, "History", "The history is read this many commits at a time.")
	history.AddRow3("Commits:", d.historyPage.QWidget)

	d.colours = qt.NewQTableWidget3(0, colourColumns)
	d.colours.SetMinimumHeight(colourTableHeight)
	d.colours.SetHorizontalHeaderLabels([]string{"Name", "Colour"})
	d.colours.VerticalHeader().SetVisible(false)
	d.colours.VerticalHeader().SetSectionResizeMode(qt.QHeaderView__ResizeToContents)
	d.colours.SetSelectionMode(qt.QAbstractItemView__NoSelection)
	d.colours.SetEditTriggers(qt.QAbstractItemView__NoEditTriggers)
	d.colours.SetFocusPolicy(qt.NoFocus)
	d.colours.SetHorizontalScrollBarPolicy(qt.ScrollBarAlwaysOff)
	d.colours.HorizontalHeader().SetSectionResizeMode2(colColourName, qt.QHeaderView__Stretch)
	d.colours.HorizontalHeader().SetSectionResizeMode2(colColourSwatch, qt.QHeaderView__ResizeToContents)
	d.addColourSection("Diff")
	d.added = d.addColour("Added lines")
	d.removed = d.addColour("Removed lines")
	d.hunk = d.addColour("Hunk headers")
	d.addColourSection("Changed files")
	for _, name := range changeNames {
		d.changes = append(d.changes, d.addColour(name))
	}
	d.addColourSection("Progress bar")
	d.progress = d.addColour("Progress")

	useDefaults := qt.NewQPushButton3("Use Default Colours")
	useDefaults.OnClicked(func() { d.showColours(settings.Defaults()) })
	defaultsRow := qt.NewQHBoxLayout2()
	defaultsRow.AddStretch()
	defaultsRow.AddWidget(useDefaults.QWidget)
	colours := d.section(look, "Colours", "")
	colours.AddRowWithWidget(d.colours.QWidget)
	colours.AddRowWithLayout(defaultsRow.QLayout)
	// The table takes what room the page has.
	look.SetStretchFactor(d.sections["Colours"].QWidget, 1)

	d.show(current)

	d.box = qt.NewQDialogButtonBox5(qt.QDialogButtonBox__Ok|qt.QDialogButtonBox__Cancel|qt.QDialogButtonBox__RestoreDefaults, qt.Horizontal)
	d.box.Button(qt.QDialogButtonBox__RestoreDefaults).OnClicked(func() { d.show(settings.Defaults()) })
	d.box.OnRejected(d.Reject)
	d.box.OnAccepted(func() {
		d.Accept()
		accepted(d.chosen(current))
	})

	layout := qt.NewQVBoxLayout(d.QWidget)
	layout.AddWidget2(d.tabs.QWidget, 1)
	layout.AddWidget(d.box.QWidget)
	// Its notes wrap onto more lines than Qt allows for, so it is given the
	// height its contents need at its width, and no less, as the about box is;
	// else the frames overlap what is in them.
	fitHeight(d.QDialog, settingsDialogWidth)
	d.SetMinimumHeight(d.Height())
	d.OnFinished(func(int) { d.DeleteLater() })
	return d
}

// section adds a frame with the title to the layout, with the note, if there is
// one, above its fields, and returns the form they go in. Its fields grow to its
// width, as some styles keep them narrow.
func (d *settingsDialog) section(layout *qt.QVBoxLayout, title, note string) *qt.QFormLayout {
	box := qt.NewQGroupBox3(title)
	form := qt.NewQFormLayout(box.QWidget)
	form.SetFieldGrowthPolicy(qt.QFormLayout__AllNonFixedFieldsGrow)
	if note != "" {
		label := qt.NewQLabel3(note)
		label.SetWordWrap(true)
		form.AddRowWithWidget(label.QWidget)
	}
	layout.AddWidget(box.QWidget)
	d.sections[title] = box
	return form
}

// addColourSection adds a bold row as wide as the table, heading those after it.
func (d *settingsDialog) addColourSection(title string) {
	item := qt.NewQTableWidgetItem2(title)
	item.SetFlags(qt.ItemIsEnabled)
	font := qt.NewQFont5(item.Font())
	defer font.Delete()
	font.SetBold(true)
	item.SetFont(font)
	row := d.colours.RowCount()
	d.colours.InsertRow(row)
	d.colours.SetItem(row, colColourName, item)
	d.colours.SetSpan(row, colColourName, 1, colourColumns)
}

// addColour adds a row with the name of a colour and the button to choose it,
// which stands a little clear of the lines of its cell.
func (d *settingsDialog) addColour(name string) *colourButton {
	button := newColourButton(name)
	item := qt.NewQTableWidgetItem2(name)
	item.SetFlags(qt.ItemIsEnabled)
	cell := qt.NewQWidget2()
	layout := qt.NewQHBoxLayout(cell)
	layout.SetContentsMargins(6, 3, 6, 3)
	layout.AddWidget(button.QWidget)
	row := d.colours.RowCount()
	d.colours.InsertRow(row)
	d.colours.SetItem(row, colColourName, item)
	d.colours.SetCellWidget(row, colColourSwatch, cell)
	return button
}

// show puts the settings in the dialog.
func (d *settingsDialog) show(s settings.Settings) {
	for i, t := range themeChoices {
		if t.value == s.Theme {
			d.theme.SetCurrentIndex(i)
		}
	}
	d.showFont(s.CodeFont, s.CodeFontSize)
	d.historyPage.SetValue(s.HistoryPageSize)
	d.showColours(s)
}

// showColours puts the settings' colours in the dialog.
func (d *settingsDialog) showColours(s settings.Settings) {
	d.added.set(s.AddedColour)
	d.removed.set(s.RemovedColour)
	d.hunk.set(s.HunkColour)
	for i, colour := range s.ChangeColours.Fields() {
		d.changes[i].set(*colour)
	}
	d.progress.set(s.ProgressColour)
}

// showFont puts a font in the dialog, each part the system's if it is not given.
func (d *settingsDialog) showFont(family string, size int) {
	font := codeFont(family, size)
	defer font.Delete()
	d.codeFamily.SetCurrentFont(font)
	d.codeSize.SetValue(font.PointSize())
}

// chosen returns the settings with what is in the dialog.
func (d *settingsDialog) chosen(s settings.Settings) settings.Settings {
	s.Theme = themeChoices[d.theme.CurrentIndex()].value
	// What is still the system's goes on following the system.
	s.CodeFont, s.CodeFontSize = "", 0
	if family := d.codeFamily.CurrentText(); family != d.defaultFamily {
		s.CodeFont = family
	}
	if size := d.codeSize.Value(); size != d.defaultSize {
		s.CodeFontSize = size
	}
	s.HistoryPageSize = d.historyPage.Value()
	s.AddedColour = d.added.colour
	s.RemovedColour = d.removed.colour
	s.HunkColour = d.hunk.colour
	for i, colour := range s.ChangeColours.Fields() {
		*colour = d.changes[i].colour
	}
	s.ProgressColour = d.progress.colour
	return s
}
