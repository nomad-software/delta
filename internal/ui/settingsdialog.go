package ui

import (
	"fmt"

	qt "github.com/mappu/miqt/qt6"

	"github.com/nomad-software/merge-monkey/internal/settings"
)

// The columns of the table of colours.
const (
	colColourName = iota
	colColourSwatch
	colourColumns
)

// colourTableHeight is the least height of the table of colours.
const colourTableHeight = 170

// fallbackCodeFontSize is the size of the code's font in points when the system's
// has none to go by.
const fallbackCodeFontSize = 10

// settingsDialog edits the theme, the font of the code and the colours of the
// diff.
type settingsDialog struct {
	*qt.QDialog
	tabs        *qt.QTabWidget
	theme       *qt.QComboBox
	codeFamily  *qt.QFontComboBox
	codeSize    *qt.QSpinBox
	codeDefault *qt.QPushButton
	// defaultFamily and defaultSize are the system's fixed width font, as the
	// fields show it, which is what is meant when nothing else is chosen.
	defaultFamily        string
	defaultSize          int
	added, removed, hunk *colourButton
	// colours is the table of the colours.
	colours *qt.QTableWidget
	box     *qt.QDialogButtonBox
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

// set shows the colour, named on the button in whichever of black and white can
// be read on it.
func (b *colourButton) set(colour string) {
	b.colour = colour
	c := qt.NewQColor6(colour)
	defer c.Delete()
	text := "#ffffff"
	if c.Red()*299+c.Green()*587+c.Blue()*114 > 128000 {
		text = "#000000"
	}
	b.SetText(colour)
	b.SetStyleSheet(fmt.Sprintf("QPushButton { background-color: %s; color: %s; padding: 6px; }", colour, text))
}

// newSettingsDialog returns the dialog showing the settings. accepted is called
// with what was chosen, on top of current, if it is accepted.
func newSettingsDialog(parent *qt.QWidget, current settings.Settings, accepted func(settings.Settings)) *settingsDialog {
	d := &settingsDialog{QDialog: qt.NewQDialog(parent)}
	d.SetWindowTitle("Settings")
	d.SetMinimumWidth(460)

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

	useDefaults := qt.NewQPushButton3("Use Default Colours")
	useDefaults.OnClicked(func() {
		d.added.set(settings.DefaultAdded)
		d.removed.set(settings.DefaultRemoved)
		d.hunk.set(settings.DefaultHunk)
	})
	defaultsRow := qt.NewQHBoxLayout2()
	defaultsRow.AddStretch()
	defaultsRow.AddWidget(useDefaults.QWidget)

	form := qt.NewQFormLayout2()
	form.AddRow3("Theme", d.theme.QWidget)
	form.AddRow4("Code font", codeRow.QLayout)

	page := qt.NewQWidget2()
	pageLayout := qt.NewQVBoxLayout(page)
	pageLayout.SetContentsMargins(20, 20, 20, 20)
	pageLayout.AddWidget(heading("Appearance").QWidget)
	pageLayout.AddLayout(form.QLayout)
	pageLayout.AddWidget(heading("Colours").QWidget)
	pageLayout.AddWidget2(d.colours.QWidget, 1)
	pageLayout.AddLayout(defaultsRow.QLayout)

	d.show(current)

	d.box = qt.NewQDialogButtonBox5(qt.QDialogButtonBox__Ok|qt.QDialogButtonBox__Cancel|qt.QDialogButtonBox__RestoreDefaults, qt.Horizontal)
	d.box.Button(qt.QDialogButtonBox__RestoreDefaults).OnClicked(func() { d.show(settings.Defaults()) })
	d.box.OnRejected(d.Reject)
	d.box.OnAccepted(func() {
		d.Accept()
		accepted(d.chosen(current))
	})

	d.tabs = qt.NewQTabWidget2()
	d.tabs.AddTab(page, "User interface")

	layout := qt.NewQVBoxLayout(d.QWidget)
	layout.AddWidget2(d.tabs.QWidget, 1)
	layout.AddWidget(d.box.QWidget)
	d.OnFinished(func(int) { d.DeleteLater() })
	return d
}

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

// heading returns a label for a section.
func heading(text string) *qt.QLabel {
	label := qt.NewQLabel3(text)
	font := qt.NewQFont5(label.Font())
	defer font.Delete()
	font.SetBold(true)
	label.SetFont(font)
	return label
}

// show puts the settings in the dialog.
func (d *settingsDialog) show(s settings.Settings) {
	for i, t := range themeChoices {
		if t.value == s.Theme {
			d.theme.SetCurrentIndex(i)
		}
	}
	d.showFont(s.CodeFont, s.CodeFontSize)
	d.added.set(s.AddedColour)
	d.removed.set(s.RemovedColour)
	d.hunk.set(s.HunkColour)
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
	s.AddedColour = d.added.colour
	s.RemovedColour = d.removed.colour
	s.HunkColour = d.hunk.colour
	return s
}
