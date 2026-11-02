package ui

import (
	qt "github.com/mappu/miqt/qt6"

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
		desktopStyle = qt.QApplication_Style().Name()
	}
	var palette *qt.QPalette
	switch theme {
	case settings.ThemeLight:
		qt.QApplication_SetStyleWithStyle("Fusion")
		palette = newPalette(lightPalette)
	case settings.ThemeDark:
		qt.QApplication_SetStyleWithStyle("Fusion")
		palette = newPalette(darkPalette)
	default:
		qt.QApplication_SetStyleWithStyle(desktopStyle)
		palette = qt.NewQPalette()
	}
	defer palette.Delete()
	qt.QApplication_SetPalette(palette)
}

// paletteColours are the colours of a palette, in general and when disabled.
// Each sets every role Fusion draws with, as what is left out would come from
// the desktop's theme.
type paletteColours struct {
	normal, disabled map[qt.QPalette__ColorRole]string
}

var lightPalette = paletteColours{
	normal: map[qt.QPalette__ColorRole]string{
		qt.QPalette__Window:          "#f0f0f0",
		qt.QPalette__WindowText:      "#202020",
		qt.QPalette__Base:            "#ffffff",
		qt.QPalette__AlternateBase:   "#f6f6f6",
		qt.QPalette__Text:            "#202020",
		qt.QPalette__PlaceholderText: "#7f7f7f",
		qt.QPalette__Button:          "#ececec",
		qt.QPalette__ButtonText:      "#202020",
		qt.QPalette__BrightText:      "#ffffff",
		qt.QPalette__ToolTipBase:     "#fffbe0",
		qt.QPalette__ToolTipText:     "#202020",
		qt.QPalette__Highlight:       "#2f7fc1",
		qt.QPalette__HighlightedText: "#ffffff",
		qt.QPalette__Link:            "#0a5cc2",
		qt.QPalette__LinkVisited:     "#6b2fa0",
		qt.QPalette__Light:           "#ffffff",
		qt.QPalette__Midlight:        "#e2e2e2",
		qt.QPalette__Mid:             "#b5b5b5",
		qt.QPalette__Dark:            "#9c9c9c",
		qt.QPalette__Shadow:          "#707070",
	},
	disabled: map[qt.QPalette__ColorRole]string{
		qt.QPalette__WindowText:      "#a2a2a2",
		qt.QPalette__Text:            "#a2a2a2",
		qt.QPalette__ButtonText:      "#a2a2a2",
		qt.QPalette__Highlight:       "#959595",
		qt.QPalette__HighlightedText: "#ffffff",
	},
}

var darkPalette = paletteColours{
	normal: map[qt.QPalette__ColorRole]string{
		qt.QPalette__Window:          "#2b2b2b",
		qt.QPalette__WindowText:      "#e4e4e4",
		qt.QPalette__Base:            "#1f1f1f",
		qt.QPalette__AlternateBase:   "#272727",
		qt.QPalette__Text:            "#e4e4e4",
		qt.QPalette__PlaceholderText: "#8a8a8a",
		qt.QPalette__Button:          "#363636",
		qt.QPalette__ButtonText:      "#e4e4e4",
		qt.QPalette__BrightText:      "#ff6a6a",
		qt.QPalette__ToolTipBase:     "#363636",
		qt.QPalette__ToolTipText:     "#e4e4e4",
		qt.QPalette__Highlight:       "#2d6cb0",
		qt.QPalette__HighlightedText: "#ffffff",
		qt.QPalette__Link:            "#70acff",
		qt.QPalette__LinkVisited:     "#b591ff",
		qt.QPalette__Light:           "#4b4b4b",
		qt.QPalette__Midlight:        "#3f3f3f",
		qt.QPalette__Mid:             "#272727",
		qt.QPalette__Dark:            "#1b1b1b",
		qt.QPalette__Shadow:          "#111111",
	},
	disabled: map[qt.QPalette__ColorRole]string{
		qt.QPalette__WindowText:      "#7c7c7c",
		qt.QPalette__Text:            "#7c7c7c",
		qt.QPalette__ButtonText:      "#7c7c7c",
		qt.QPalette__Highlight:       "#4c4c4c",
		qt.QPalette__HighlightedText: "#b2b2b2",
	},
}

// newPalette returns a palette of the colours.
func newPalette(colours paletteColours) *qt.QPalette {
	palette := qt.NewQPalette()
	for role, name := range colours.normal {
		colour := qt.NewQColor6(name)
		palette.SetColor2(role, colour)
		colour.Delete()
	}
	for role, name := range colours.disabled {
		colour := qt.NewQColor6(name)
		palette.SetColor(qt.QPalette__Disabled, role, colour)
		colour.Delete()
	}
	return palette
}
