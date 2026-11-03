package ui

import (
	"testing"

	qt "github.com/mappu/miqt/qt6"

	"github.com/nomad-software/delta/internal/git"
	"github.com/nomad-software/delta/internal/settings"
)

// TestChangeIcons checks that each code is a circle with an outline in its
// colour and a pale wash of it inside, the same colour in either theme, and
// that a code with no colour is a blank of the same size.
func TestChangeIcons(t *testing.T) {
	on(func() {
		defer applyTheme("")
		for _, theme := range []string{settings.ThemeLight, settings.ThemeDark} {
			applyTheme(theme)
			for code, want := range changeColours {
				// A selected row's circle keeps its colour.
				mode := qt.QIcon__Normal
				if code == 'M' {
					mode = qt.QIcon__Selected
				}
				image := changeIcon(code).Pixmap7(changeDotSize, changeDotSize, mode).ToImage()
				// The edge, at the middle of the left, is the colour itself,
				// and the middle a wash of it.
				edge := image.PixelColor(0, changeDotSize/2)
				middle := image.PixelColor(changeDotSize/2, changeDotSize/2)
				if edge.Alpha() < 128 || middle.Alpha() == 0 || middle.Alpha() > 128 {
					t.Errorf("%s %c: edge alpha %d, middle alpha %d", theme, code, edge.Alpha(), middle.Alpha())
				}
				colour := qt.NewQColor6(want)
				if d := edge.Hue() - colour.Hue(); colour.Saturation() > 0 && (d > 8 || d < -8) {
					t.Errorf("%s %c: edge hue %d, not %d", theme, code, edge.Hue(), colour.Hue())
				}
				colour.Delete()
			}
		}
		blank := changeIcon(0).Pixmap2(changeDotSize, changeDotSize).ToImage()
		for x := range changeDotSize {
			if blank.PixelColor(x, changeDotSize/2).Alpha() != 0 {
				t.Fatal("the blank is not blank")
			}
		}
		// Every code a change can have has its colour.
		for _, code := range []byte{'A', 'D', 'M', 'R', 'C', 'T', git.Untracked, git.Conflict} {
			if _, ok := changeColours[code]; !ok {
				t.Errorf("%c has no colour", code)
			}
		}
	})
}
