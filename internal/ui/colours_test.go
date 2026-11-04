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
