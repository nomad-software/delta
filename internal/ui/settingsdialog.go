package ui

import (
	"fmt"

	qt "github.com/mappu/miqt/qt6"

	"github.com/nomad-software/merge-monkey/internal/settings"
)

// settingsDialog edits the theme and the colours of the diff.
type settingsDialog struct {
	*qt.QDialog
	tabs           *qt.QTabWidget
	theme          *qt.QComboBox
	added, removed *colourButton
	box            *qt.QDialogButtonBox
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
	d.added = newColourButton("Added Lines")
	d.removed = newColourButton("Removed Lines")

	useDefaults := qt.NewQPushButton3("Use Default Colours")
	useDefaults.OnClicked(func() {
		d.added.set(settings.DefaultAdded)
		d.removed.set(settings.DefaultRemoved)
	})
	defaultsRow := qt.NewQHBoxLayout2()
	defaultsRow.AddStretch()
	defaultsRow.AddWidget(useDefaults.QWidget)

	note := qt.NewQLabel3("Lines of a diff are tinted with these colours, mixed into the background so that the text stays readable in any theme.")
	note.SetWordWrap(true)

	page := qt.NewQWidget2()
	form := qt.NewQFormLayout(page)
	form.SetContentsMargins(20, 20, 20, 20)
	form.AddRowWithWidget(heading("Appearance").QWidget)
	form.AddRow3("Theme", d.theme.QWidget)
	form.AddRowWithWidget(heading("Diff colours").QWidget)
	form.AddRow3("Added lines", d.added.QWidget)
	form.AddRow3("Removed lines", d.removed.QWidget)
	form.AddRowWithWidget(note.QWidget)
	form.AddRowWithLayout(defaultsRow.QLayout)

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
	d.added.set(s.AddedColour)
	d.removed.set(s.RemovedColour)
}

// chosen returns the settings with what is in the dialog.
func (d *settingsDialog) chosen(s settings.Settings) settings.Settings {
	s.Theme = themeChoices[d.theme.CurrentIndex()].value
	s.AddedColour = d.added.colour
	s.RemovedColour = d.removed.colour
	return s
}
