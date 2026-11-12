package ui

import (
	"slices"
	"testing"

	"github.com/nomad-software/delta/internal/settings"
	"github.com/nomad-software/qute/qtwidgets"
)

func TestSettingsDialog(t *testing.T) {
	on(func() {
		current := settings.Defaults()
		current.Theme = settings.ThemeDark
		current.AddedColour = "#123456"
		var got settings.Settings
		d := newSettingsDialog(nil, func() settings.Settings { return current }, func(s settings.Settings) { got = s })
		defer d.Delete()
		// General, with History and Repositories, then User interface, with
		// Appearance and Colours.
		general, look := d.tabs.Widget(0), d.tabs.Widget(1)
		if d.tabs.Count() != 2 || d.tabs.TabText(0) != "General" || d.tabs.TabText(1) != "User interface" {
			t.Fatalf("tabs: %d, %q, %q", d.tabs.Count(), d.tabs.TabText(0), d.tabs.TabText(1))
		}
		if !general.IsAncestorOf(d.sections["History"].AsQWidget()) || !general.IsAncestorOf(d.sections["Repositories"].AsQWidget()) ||
			!look.IsAncestorOf(d.sections["Appearance"].AsQWidget()) || !look.IsAncestorOf(d.sections["Colours"].AsQWidget()) {
			t.Error("a section is in the wrong tab")
		}
		if d.theme.CurrentText() != "Dark" || d.theme.Count() != 3 || d.added.colour != "#123456" {
			t.Errorf("shown as %q, %s", d.theme.CurrentText(), d.added.colour)
		}
		if d.historyPage.Value() != settings.DefaultHistoryPageSize {
			t.Errorf("history page %d", d.historyPage.Value())
		}
		d.theme.SetCurrentIndex(1)
		d.removed.set("#abcdef")
		d.historyPage.SetValue(1200)
		d.box.Button(qtwidgets.QDialogButtonBoxOk).Click()
		if got.Theme != settings.ThemeLight || got.RemovedColour != "#abcdef" || got.HistoryPageSize != 1200 {
			t.Errorf("chosen %+v", got)
		}
		d.box.Button(qtwidgets.QDialogButtonBoxRestoreDefaults).Click()
		if d.theme.CurrentIndex() != 0 || d.added.colour != settings.DefaultAdded {
			t.Errorf("defaults %q %s", d.theme.CurrentText(), d.added.colour)
		}
	})
}

func TestCodeFontSetting(t *testing.T) {
	on(func() {
		system := codeFont("", 0)
		defer system.Delete()
		var got settings.Settings
		d := newSettingsDialog(nil, settings.Defaults, func(s settings.Settings) { got = s })
		defer d.Delete()

		// Left alone, it follows the system.
		if d.codeSize.Value() != system.PointSize() {
			t.Errorf("size shown %d, not %d", d.codeSize.Value(), system.PointSize())
		}
		if s := d.chosen(settings.Defaults()); s.CodeFont != "" || s.CodeFontSize != 0 {
			t.Errorf("unchanged gave %q, %d", s.CodeFont, s.CodeFontSize)
		}

		// Any font can be chosen, not only those of a fixed width.
		if d.codeFamily.FontFilters() != qtwidgets.QFontComboBoxAllFonts {
			t.Errorf("fonts are limited by %v", d.codeFamily.FontFilters())
		}

		// A size and another family are kept.
		d.codeSize.SetValue(system.PointSize() + 3)
		other := -1
		for i := range d.codeFamily.Count() {
			if d.codeFamily.ItemText(i) != d.defaultFamily {
				other = i
				break
			}
		}
		family := ""
		if other >= 0 {
			d.codeFamily.SetCurrentIndex(other)
			family = d.codeFamily.CurrentText()
		}
		d.box.Button(qtwidgets.QDialogButtonBoxOk).Click()
		if got.CodeFontSize != system.PointSize()+3 || got.CodeFont != family {
			t.Errorf("chose %q, %d", got.CodeFont, got.CodeFontSize)
		}

		// The button beside them goes back to the system's.
		d.codeDefault.Click()
		if s := d.chosen(settings.Defaults()); s.CodeFont != "" || s.CodeFontSize != 0 {
			t.Errorf("default button gave %q, %d", s.CodeFont, s.CodeFontSize)
		}

		// Restoring the defaults shows the system's again.
		d.codeSize.SetValue(system.PointSize() + 1)
		d.box.Button(qtwidgets.QDialogButtonBoxRestoreDefaults).Click()
		if s := d.chosen(settings.Defaults()); s.CodeFont != "" || s.CodeFontSize != 0 {
			t.Errorf("defaults gave %q, %d", s.CodeFont, s.CodeFontSize)
		}
	})
}

// As apemail's, each section is a frame with its name, a note, and its fields,
// labelled; the colours are in a table in theirs, every colour the program
// gives itself, and each is chosen.
func TestSettingsSections(t *testing.T) {
	on(func() {
		var got settings.Settings
		d := newSettingsDialog(nil, settings.Defaults, func(s settings.Settings) { got = s })
		defer d.Delete()
		// note is the section's note, or empty if it has none.
		in := func(title, note string, widgets ...*qtwidgets.QWidget) {
			t.Helper()
			box := d.sections[title]
			if box == nil || box.Title() != title {
				t.Errorf("no frame %q", title)
				return
			}
			for _, w := range widgets {
				if !box.IsAncestorOf(w) {
					t.Errorf("%q does not hold %q", title, w.ObjectName())
				}
			}
			got := ""
			for _, child := range box.Children() {
				if child.Inherits("QLabel") {
					if label := qtwidgets.QLabelCast(child); label.WordWrap() {
						got = label.Text()
					}
				}
			}
			if got != note {
				t.Errorf("%q's note is %q", title, got)
			}
		}
		in("Appearance", "", d.theme.AsQWidget(), d.codeFamily.AsQWidget())
		in("History", "The history is read this many commits at a time.", d.historyPage.AsQWidget())
		in("Repositories", "The repositories opened are remembered, up to this many.", d.repositoryLimit.AsQWidget())
		if page := d.tabs.Widget(0).Layout(); page.IndexOf(d.sections["Repositories"].AsQWidget()) >= page.IndexOf(d.sections["History"].AsQWidget()) {
			t.Error("Repositories is not above History")
		}
		in("Colours", "", d.colours.AsQWidget())
		if len(d.sections) != 4 {
			t.Errorf("%d frames", len(d.sections))
		}
		form := qtwidgets.QFormLayoutCast(d.sections["History"].Layout())
		if label := qtwidgets.QLabelCast(form.LabelForField(d.historyPage.AsQWidget())); label.Text() != "Commits:" {
			t.Errorf("labelled %q", label.Text())
		}

		// The colours are in a table, in sections headed by bold rows across it.
		if d.colours.ColumnCount() != 2 {
			t.Fatalf("table has %d columns", d.colours.ColumnCount())
		}
		var names []string
		for row := range d.colours.RowCount() {
			item := d.colours.Item(row, colColourName)
			names = append(names, item.Text())
			if heading := d.colours.CellWidget(row, colColourSwatch) == nil; heading != item.Font().Bold() || heading != (d.colours.ColumnSpan(row, colColourName) == 2) {
				t.Errorf("row %q: heading %v, bold %v", item.Text(), heading, item.Font().Bold())
			}
		}
		want := []string{"Diff", "Added lines", "Removed lines", "Hunk headers", "Changed files"}
		want = append(want, changeNames...)
		want = append(want, "Progress bar", "Progress")
		if !slices.Equal(names, want) {
			t.Errorf("rows %q", names)
		}
		if !d.colours.CellWidget(3, colColourSwatch).IsAncestorOf(d.hunk.AsQWidget()) {
			t.Error("the button is not in the table")
		}

		// Each is chosen, and the defaults put back.
		d.changes[2].set("#000003")
		d.progress.set("#000009")
		d.box.Button(qtwidgets.QDialogButtonBoxOk).Click()
		if got.ChangeColours.Deleted != "#000003" || got.ProgressColour != "#000009" {
			t.Errorf("chose %+v, %s", got.ChangeColours, got.ProgressColour)
		}
		d.showColours(settings.Defaults())
		if s := d.chosen(settings.Settings{}); s.ChangeColours != settings.DefaultChangeColours || s.ProgressColour != settings.DefaultProgressColour {
			t.Errorf("defaults %+v", s)
		}
	})
}

// A colour button shows its colour as it is: under Fusion, a style sheet's
// background alone was shaded paler by the style.
func TestColourButtonIsItsColour(t *testing.T) {
	on(func() {
		defer applyTheme("")
		for _, theme := range []string{settings.ThemeLight, settings.ThemeDark} {
			applyTheme(theme)
			b := newColourButton("Test")
			b.set(settings.Red)
			b.Resize(140, 32)
			image := b.Grab().ToImage()
			if got := image.PixelColor(10, image.Height()/2).Name(); got != settings.Red {
				t.Errorf("%s: shows %s", theme, got)
			}
			b.Delete()
		}
	})
}

// The settings dialog is tall enough for what is in it: its notes wrap onto
// more lines than Qt allowed for, and Use Default Colours lay over the table.
func TestSettingsDialogFits(t *testing.T) {
	on(func() {
		d := newSettingsDialog(nil, settings.Defaults, func(settings.Settings) {})
		defer d.Delete()
		// The colours are on the second tab.
		d.tabs.SetCurrentIndex(1)
		d.Show()
		var button *qtwidgets.QPushButton
		for _, b := range d.sections["Colours"].Children() {
			if b.Inherits("QPushButton") && qtwidgets.QPushButtonCast(b).Text() == "Use Default Colours" {
				button = qtwidgets.QPushButtonCast(b)
			}
		}
		if button == nil {
			t.Fatal("no Use Default Colours")
		}
		frame := d.sections["Colours"]
		_, top := mapPoint(button.AsQWidget(), frame.AsQWidget(), 0, 0)
		_, bottom := mapPoint(d.colours.AsQWidget(), frame.AsQWidget(), 0, d.colours.Height())
		if top < bottom || top+button.Height() > frame.Height() {
			t.Errorf("the button is at %d to %d, the table ends at %d, the frame at %d", top, top+button.Height(), bottom, frame.Height())
		}
		d.Close()
	})
}
