package ui

import (
	"github.com/nomad-software/qute/qtgui"
	"github.com/nomad-software/qute/qtwidgets"

	"github.com/nomad-software/delta/internal/settings"
)

// themeChoices are the themes offered, by how the settings name them.
var themeChoices = []struct{ value, label string }{
	{"", "Follow the system"},
	{settings.ThemeLight, "Light"},
	{settings.ThemeDark, "Dark"},
}

// desktopStyle is the style the program was started with, which following the
// system returns to.
var desktopStyle string

// applyTheme puts a theme into effect. The light and dark themes are Qt's Fusion
// style, which honours any palette on any desktop, with a palette of their own.
// Following the system is the desktop's style with an empty palette, which Qt
// fills in and keeps up to date as the desktop's theme changes.
func applyTheme(theme string) {
	if desktopStyle == "" {
		desktopStyle = qtwidgets.QApplicationStyle().Name()
	}
	var palette *qtgui.QPalette
	switch theme {
	case settings.ThemeLight:
		qtwidgets.QApplicationSetStyleName("Fusion")
		palette = newPalette(lightPalette)
	case settings.ThemeDark:
		qtwidgets.QApplicationSetStyleName("Fusion")
		palette = newPalette(darkPalette)
	default:
		qtwidgets.QApplicationSetStyleName(desktopStyle)
		palette = qtgui.NewQPalette()
	}
	qtwidgets.QApplicationSetPalette(palette)
}

// paletteColours are the colours of a palette, in general and when disabled.
// Each sets every role Fusion draws with, as what is left out would come from
// the desktop's theme.
type paletteColours struct {
	normal, disabled map[qtgui.QPaletteColorRole]string
}

var lightPalette = paletteColours{
	normal: map[qtgui.QPaletteColorRole]string{
		qtgui.QPaletteWindow:          "#f0f0f0",
		qtgui.QPaletteWindowText:      "#202020",
		qtgui.QPaletteBase:            "#ffffff",
		qtgui.QPaletteAlternateBase:   "#f6f6f6",
		qtgui.QPaletteText:            "#202020",
		qtgui.QPalettePlaceholderText: "#7f7f7f",
		qtgui.QPaletteButton:          "#ececec",
		qtgui.QPaletteButtonText:      "#202020",
		qtgui.QPaletteBrightText:      "#ffffff",
		qtgui.QPaletteToolTipBase:     "#fffbe0",
		qtgui.QPaletteToolTipText:     "#202020",
		qtgui.QPaletteHighlight:       "#2f7fc1",
		qtgui.QPaletteHighlightedText: "#ffffff",
		qtgui.QPaletteLink:            "#0a5cc2",
		qtgui.QPaletteLinkVisited:     "#6b2fa0",
		qtgui.QPaletteLight:           "#ffffff",
		qtgui.QPaletteMidlight:        "#e2e2e2",
		qtgui.QPaletteMid:             "#b5b5b5",
		qtgui.QPaletteDark:            "#9c9c9c",
		qtgui.QPaletteShadow:          "#707070",
	},
	disabled: map[qtgui.QPaletteColorRole]string{
		qtgui.QPaletteWindowText:      "#a2a2a2",
		qtgui.QPaletteText:            "#a2a2a2",
		qtgui.QPaletteButtonText:      "#a2a2a2",
		qtgui.QPaletteHighlight:       "#959595",
		qtgui.QPaletteHighlightedText: "#ffffff",
	},
}

var darkPalette = paletteColours{
	normal: map[qtgui.QPaletteColorRole]string{
		qtgui.QPaletteWindow:          "#2b2b2b",
		qtgui.QPaletteWindowText:      "#e4e4e4",
		qtgui.QPaletteBase:            "#1f1f1f",
		qtgui.QPaletteAlternateBase:   "#272727",
		qtgui.QPaletteText:            "#e4e4e4",
		qtgui.QPalettePlaceholderText: "#8a8a8a",
		qtgui.QPaletteButton:          "#363636",
		qtgui.QPaletteButtonText:      "#e4e4e4",
		qtgui.QPaletteBrightText:      "#ff6a6a",
		qtgui.QPaletteToolTipBase:     "#363636",
		qtgui.QPaletteToolTipText:     "#e4e4e4",
		qtgui.QPaletteHighlight:       "#2d6cb0",
		qtgui.QPaletteHighlightedText: "#ffffff",
		qtgui.QPaletteLink:            "#70acff",
		qtgui.QPaletteLinkVisited:     "#b591ff",
		qtgui.QPaletteLight:           "#4b4b4b",
		qtgui.QPaletteMidlight:        "#3f3f3f",
		qtgui.QPaletteMid:             "#272727",
		qtgui.QPaletteDark:            "#1b1b1b",
		qtgui.QPaletteShadow:          "#111111",
	},
	disabled: map[qtgui.QPaletteColorRole]string{
		qtgui.QPaletteWindowText:      "#7c7c7c",
		qtgui.QPaletteText:            "#7c7c7c",
		qtgui.QPaletteButtonText:      "#7c7c7c",
		qtgui.QPaletteHighlight:       "#4c4c4c",
		qtgui.QPaletteHighlightedText: "#b2b2b2",
	},
}

// newPalette returns a palette of the colours.
func newPalette(colours paletteColours) *qtgui.QPalette {
	palette := qtgui.NewQPalette()
	for role, name := range colours.normal {
		palette.SetColor(role, colour(name))
	}
	for role, name := range colours.disabled {
		palette.SetGroupColor(qtgui.QPaletteDisabled, role, colour(name))
	}
	return palette
}
