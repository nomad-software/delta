package ui

import (
	"math"
	"testing"

	"github.com/nomad-software/qute/qtcore"

	"github.com/nomad-software/delta/internal/git"
	"github.com/nomad-software/delta/internal/settings"
	"github.com/nomad-software/qute/qtgui"
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
				mode := qtgui.QIconNormal
				if code == 'M' {
					mode = qtgui.QIconSelected
				}
				image := changeIcon(code).PixmapFull(qtcore.QSize{Width: changeDotSize, Height: changeDotSize}, mode, qtgui.QIconOff).ToImage()
				// The edge, at the middle of the left, is the colour itself,
				// and the middle a wash of it.
				edge := image.PixelColor(0, changeDotSize/2)
				// The outline is two pixels wide: the second in is it too.
				inner := image.PixelColor(1, changeDotSize/2)
				middle := image.PixelColor(changeDotSize/2, changeDotSize/2)
				if edge.A < 128 || inner.A < 192 || middle.A == 0 || middle.A > 128 {
					t.Errorf("%s %c: edge alpha %d, inner %d, middle %d", theme, code, edge.A, inner.A, middle.A)
				}
				c := colour(want)
				if d := hue(edge) - hue(c); saturated(c) && (d > 8 || d < -8) {
					t.Errorf("%s %c: edge hue %d, not %d", theme, code, hue(edge), hue(c))
				}
			}
		}
		blank := changeIcon(0).Pixmap(qtcore.QSize{Width: changeDotSize, Height: changeDotSize}).ToImage()
		for x := range changeDotSize {
			if blank.PixelColor(x, changeDotSize/2).A != 0 {
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

// saturated says whether the colour has a hue, as a grey has none.
func saturated(c qtgui.QColor) bool {
	return c.R != c.G || c.G != c.B
}

// hue returns the hue of the colour in degrees, from 0 to 359, as Qt gives it.
func hue(c qtgui.QColor) int {
	r, g, b := float64(c.R), float64(c.G), float64(c.B)
	high, low := max(r, g, b), min(r, g, b)
	delta := high - low
	if delta == 0 {
		return -1
	}
	var h float64
	switch high {
	case r:
		h = (g - b) / delta
	case g:
		h = 2 + (b-r)/delta
	default:
		h = 4 + (r-g)/delta
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return int(math.Round(h)) % 360
}
