package ui

import (
	"fmt"

	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtwidgets"

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
	*qtwidgets.QDialog
	tabs        *qtwidgets.QTabWidget
	theme       *qtwidgets.QComboBox
	codeFamily  *qtwidgets.QFontComboBox
	codeSize    *qtwidgets.QSpinBox
	codeDefault *qtwidgets.QPushButton
	// historyPage is how many commits of the history are read at a time, and
	// repositoryLimit how many repositories opened are remembered.
	historyPage, repositoryLimit *qtwidgets.QSpinBox
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
	colours *qtwidgets.QTableWidget
	// sections are the frames of the sections, by title.
	sections map[string]*qtwidgets.QGroupBox
	box      *qtwidgets.QDialogButtonBox
}

// colourButton shows a colour, and chooses another when clicked.
type colourButton struct {
	*qtwidgets.QPushButton
	colour string
}

func newColourButton(title string) *colourButton {
	b := &colourButton{QPushButton: qtwidgets.NewQPushButton(nil)}
	b.SetMinimumWidth(140)
	b.OnClicked(func(bool) {
		// A cancelled dialog gives an invalid colour.
		if chosen := qtwidgets.QColorDialogGetColorFull(colour(b.colour), b, title, 0); chosen.IsValid() {
			b.set(chosen.Name())
		}
	})
	return b
}

// set shows the colour, named on the button in whichever of Solarized's darkest
// and lightest can be read on it.
func (b *colourButton) set(name string) {
	b.colour = name
	c := colour(name)
	text := settings.Base3
	if int(c.R)*299+int(c.G)*587+int(c.B)*114 > 128000 {
		text = settings.Base03
	}
	b.SetText(name)
	// With a border of its own, the button is drawn in the colour as it is,
	// rather than shaded by the style.
	b.SetStyleSheet(fmt.Sprintf("QPushButton { background-color: %s; color: %s; padding: 6px; border: 1px solid %s; border-radius: 3px; }", name, text, text))
}

// settingsDialogWidth is the narrowest the dialog is, so that its notes have
// room.
const settingsDialogWidth = 560

// changeNames are what can happen to a file, as the dialog names them, in the
// order of ChangeColours.Fields.
var changeNames = []string{"Modified", "Added", "Deleted", "Renamed", "Copied", "Type changed", "In conflict", "Untracked"}

// newSettingsDialog returns the dialog showing the settings. accepted is called
// with what was chosen, on top of current, if it is accepted.
func newSettingsDialog(parent qtwidgets.QWidgetLike, current settings.Settings, accepted func(settings.Settings)) *settingsDialog {
	d := &settingsDialog{QDialog: qtwidgets.NewQDialog(parent), sections: map[string]*qtwidgets.QGroupBox{}}
	d.SetWindowTitle("Settings")
	d.SetMinimumWidth(settingsDialogWidth)

	d.theme = qtwidgets.NewQComboBox(nil)
	for _, t := range themeChoices {
		d.theme.AddItem(t.label)
	}
	d.codeFamily = qtwidgets.NewQFontComboBox(nil)
	d.codeSize = qtwidgets.NewQSpinBox(nil)
	d.codeSize.SetRange(settings.MinCodeFontSize, settings.MaxCodeFontSize)
	d.codeSize.SetSuffix(" pt")
	system := codeFont("", 0)
	d.codeFamily.SetCurrentFont(system)
	d.defaultFamily = d.codeFamily.CurrentText()
	d.defaultSize = system.PointSize()
	d.codeDefault = qtwidgets.NewQPushButtonWithText("Default", nil)
	d.codeDefault.SetToolTip("The system's fixed width font")
	d.codeDefault.OnClicked(func(bool) { d.showFont("", 0) })
	codeRow := qtwidgets.NewQHBoxLayout(nil)
	codeRow.AddWidgetFull(d.codeFamily, 1, 0)
	codeRow.AddWidget(d.codeSize)
	codeRow.AddWidget(d.codeDefault)
	d.historyPage = qtwidgets.NewQSpinBox(nil)
	d.historyPage.SetRange(settings.MinHistoryPageSize, settings.MaxHistoryPageSize)
	d.historyPage.SetSingleStep(100)
	d.historyPage.SetSuffix(" commits")
	d.repositoryLimit = qtwidgets.NewQSpinBox(nil)
	d.repositoryLimit.SetRange(settings.MinRepositoryLimit, settings.MaxRepositoryLimit)
	d.repositoryLimit.SetSuffix(" repositories")

	// General comes first, as apemail's does, its sections at the top.
	d.tabs = qtwidgets.NewQTabWidget(nil)
	general := d.page("General")
	repositories := d.section(general, "Repositories", "The repositories opened are remembered, up to this many.")
	repositories.AddRowText("Remembered:", d.repositoryLimit)
	history := d.section(general, "History", "The history is read this many commits at a time.")
	history.AddRowText("Commits:", d.historyPage)
	general.AddStretch()

	look := d.page("User interface")
	appearance := d.section(look, "Appearance", "")
	appearance.AddRowText("Theme:", d.theme)
	appearance.AddRowTextLayout("Code font:", codeRow)

	d.colours = qtwidgets.NewQTableWidgetWithRows(0, colourColumns, nil)
	d.colours.SetMinimumHeight(colourTableHeight)
	d.colours.SetHorizontalHeaderLabels([]string{"Name", "Colour"})
	d.colours.VerticalHeader().SetVisible(false)
	d.colours.VerticalHeader().SetSectionResizeModeAll(qtwidgets.QHeaderViewResizeToContents)
	d.colours.SetSelectionMode(qtwidgets.QAbstractItemViewNoSelection)
	d.colours.SetEditTriggers(qtwidgets.QAbstractItemViewNoEditTriggers)
	d.colours.SetFocusPolicy(qtcore.NoFocus)
	d.colours.SetHorizontalScrollBarPolicy(qtcore.ScrollBarAlwaysOff)
	d.colours.HorizontalHeader().SetSectionResizeMode(colColourName, qtwidgets.QHeaderViewStretch)
	d.colours.HorizontalHeader().SetSectionResizeMode(colColourSwatch, qtwidgets.QHeaderViewResizeToContents)
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

	useDefaults := qtwidgets.NewQPushButtonWithText("Use Default Colours", nil)
	useDefaults.OnClicked(func(bool) { d.showColours(settings.Defaults()) })
	defaultsRow := qtwidgets.NewQHBoxLayout(nil)
	defaultsRow.AddStretch()
	defaultsRow.AddWidget(useDefaults)
	colours := d.section(look, "Colours", "")
	colours.AddRowWidget(d.colours)
	colours.AddRowSpanLayout(defaultsRow)
	// The table takes what room the page has.
	look.SetStretchFactor(d.sections["Colours"], 1)

	d.show(current)

	d.box = qtwidgets.NewQDialogButtonBoxWithButtonsOriented(qtwidgets.QDialogButtonBoxOk|qtwidgets.QDialogButtonBoxCancel|qtwidgets.QDialogButtonBoxRestoreDefaults, qtcore.Horizontal, nil)
	d.box.Button(qtwidgets.QDialogButtonBoxRestoreDefaults).OnClicked(func(bool) { d.show(settings.Defaults()) })
	d.box.OnRejected(d.Reject)
	d.box.OnAccepted(func() {
		d.Accept()
		accepted(d.chosen(current))
	})

	layout := qtwidgets.NewQVBoxLayout(d)
	layout.AddWidgetFull(d.tabs, 1, 0)
	layout.AddWidget(d.box)
	// Its notes wrap onto more lines than Qt allows for, so it is given the
	// height its contents need at its width, and no less, as the about box is;
	// else the frames overlap what is in them.
	fitHeight(d.QDialog, settingsDialogWidth)
	d.SetMinimumHeight(d.Height())
	d.OnFinished(func(int) { d.DeleteLater() })
	return d
}

// page adds a tab, and returns the layout its sections go in.
func (d *settingsDialog) page(title string) *qtwidgets.QVBoxLayout {
	w := qtwidgets.NewQWidget(nil)
	d.tabs.AddTab(w, title)
	return qtwidgets.NewQVBoxLayout(w)
}

// section adds a frame with the title to the layout, with the note, if there is
// one, above its fields, and returns the form they go in. Its fields grow to its
// width, as some styles keep them narrow.
func (d *settingsDialog) section(layout *qtwidgets.QVBoxLayout, title, note string) *qtwidgets.QFormLayout {
	box := qtwidgets.NewQGroupBoxWithTitle(title, nil)
	form := qtwidgets.NewQFormLayout(box)
	form.SetFieldGrowthPolicy(qtwidgets.QFormLayoutAllNonFixedFieldsGrow)
	if note != "" {
		label := qtwidgets.NewQLabelWithText(note, nil)
		label.SetWordWrap(true)
		form.AddRowWidget(label)
	}
	layout.AddWidget(box)
	d.sections[title] = box
	return form
}

// addColourSection adds a bold row as wide as the table, heading those after it.
func (d *settingsDialog) addColourSection(title string) {
	item := qtwidgets.NewQTableWidgetItemWithText(title)
	font := item.Font()
	font.SetBold(true)
	item.SetFont(font)
	row := d.addColourRow(item)
	d.colours.SetSpan(row, colColourName, 1, colourColumns)
}

// addColourRow adds a row to the table of colours, named by the item, which
// can't be edited, and returns its number.
func (d *settingsDialog) addColourRow(item *qtwidgets.QTableWidgetItem) int {
	item.SetFlags(qtcore.ItemIsEnabled)
	row := d.colours.RowCount()
	d.colours.InsertRow(row)
	d.colours.SetItem(row, colColourName, item)
	return row
}

// addColour adds a row with the name of a colour and the button to choose it,
// which stands a little clear of the lines of its cell.
func (d *settingsDialog) addColour(name string) *colourButton {
	button := newColourButton(name)
	cell := qtwidgets.NewQWidget(nil)
	layout := qtwidgets.NewQHBoxLayout(cell)
	layout.SetContentsMargins(qtcore.QMargins{Left: 6, Top: 3, Right: 6, Bottom: 3})
	layout.AddWidget(button)
	row := d.addColourRow(qtwidgets.NewQTableWidgetItemWithText(name))
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
	d.repositoryLimit.SetValue(s.RepositoryLimit)
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
	s.RepositoryLimit = d.repositoryLimit.Value()
	s.AddedColour = d.added.colour
	s.RemovedColour = d.removed.colour
	s.HunkColour = d.hunk.colour
	for i, colour := range s.ChangeColours.Fields() {
		*colour = d.changes[i].colour
	}
	s.ProgressColour = d.progress.colour
	return s
}
