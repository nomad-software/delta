package ui

import (
	"slices"
	"testing"

	qt "github.com/mappu/miqt/qt6"

	"github.com/nomad-software/delta/internal/settings"
)

// solarized are Solarized's sixteen colours.
var solarized = []string{
	settings.Base03, settings.Base02, settings.Base01, settings.Base00, settings.Base0, settings.Base1, settings.Base2, settings.Base3,
	settings.Yellow, settings.Orange, settings.Red, settings.Magenta, settings.Violet, settings.Blue, settings.Cyan, settings.Green,
}

// Every colour the program gives itself is Solarized's by default: the graph's
// lanes, each accent once, the circles of changed files, the diffs' and the
// progress bar's. The themes keep their own palettes.
func TestColoursAreSolarized(t *testing.T) {
	lanes := graphColours
	if len(slices.Compact(slices.Sorted(slices.Values(lanes)))) != len(lanes) {
		t.Errorf("lanes %q", lanes)
	}
	if !slices.Contains(solarized, settings.DefaultProgressColour) {
		t.Errorf("the progress bar is %s", settings.DefaultProgressColour)
	}
	for _, colour := range lanes {
		if !slices.Contains(solarized, colour) {
			t.Errorf("a lane is %s", colour)
		}
	}
	for change, colour := range changeColours {
		if !slices.Contains(solarized, colour) {
			t.Errorf("%c is %s", change, colour)
		}
	}
	if !slices.Contains(solarized, settings.DefaultAdded) || !slices.Contains(solarized, settings.DefaultRemoved) || !slices.Contains(solarized, settings.DefaultHunk) {
		t.Errorf("diffs are %s, %s and %s", settings.DefaultAdded, settings.DefaultRemoved, settings.DefaultHunk)
	}
	for _, palette := range []paletteColours{lightPalette, darkPalette} {
		if slices.Contains(solarized, palette.normal[qt.QPalette__Window]) {
			t.Error("a theme was changed")
		}
	}
}

// The progress bar's segment is in its colour in any theme.
func TestProgressColour(t *testing.T) {
	on(func() {
		defer applyTheme("")
		for _, theme := range []string{settings.ThemeLight, settings.ThemeDark} {
			applyTheme(theme)
			p := newProgress(settings.Blue)
			p.Resize(400, progressHeight)
			p.phase = 0.5
			image := p.Grab().ToImage()
			left, right := p.chunk()
			if got := image.PixelColor((left+right)/2, progressHeight/2).Name(); got != settings.Blue {
				t.Errorf("%s: the segment is %s", theme, got)
			}
			p.Delete()
		}
	})
}

// Colours chosen in the settings are put into effect at once, and saved: the
// circles of changed files and the progress bar's.
func TestColoursApplied(t *testing.T) {
	defer on(func() { changeColours = changeCodes(settings.DefaultChangeColours) })
	w := newTestWindow(t, newRepo(t))
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })
	on(func() {
		s := w.config
		s.ChangeColours.Modified = "#010101"
		s.ProgressColour = "#030303"
		w.applySettings(s)
		if changeColours['M'] != "#010101" || w.progress.colour != "#030303" {
			t.Errorf("circle %s, progress %s", changeColours['M'], w.progress.colour)
		}
	})
	got := settings.Load(w.path)
	if got.ChangeColours.Modified != "#010101" || got.ProgressColour != "#030303" {
		t.Errorf("saved %+v", got)
	}
	// A new window starts with them.
	again := newTestWindowAt(t, "", w.path)
	on(func() {
		if changeColours['M'] != "#010101" || again.progress.colour != "#030303" {
			t.Error("a new window started without them")
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
		d := newSettingsDialog(nil, settings.Defaults(), func(settings.Settings) {})
		defer d.Delete()
		d.Show()
		var button *qt.QPushButton
		for _, b := range d.sections["Colours"].Children() {
			if b.Inherits("QPushButton") && qt.UnsafeNewQPushButton(b.UnsafePointer()).Text() == "Use Default Colours" {
				button = qt.UnsafeNewQPushButton(b.UnsafePointer())
			}
		}
		if button == nil {
			t.Fatal("no Use Default Colours")
		}
		frame := d.sections["Colours"]
		at := func(w *qt.QWidget, y int) int {
			point := qt.NewQPoint2(0, y)
			defer point.Delete()
			return w.MapTo2(frame.QWidget, point).Y()
		}
		top, bottom := at(button.QWidget, 0), at(d.colours.QWidget, d.colours.Height())
		if top < bottom || top+button.Height() > frame.Height() {
			t.Errorf("the button is at %d to %d, the table ends at %d, the frame at %d", top, top+button.Height(), bottom, frame.Height())
		}
		d.Close()
	})
}
