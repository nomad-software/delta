// Package settings keeps what is remembered between runs in a JSON file under
// the system's folder for program configuration. It knows nothing of the UI.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nomad-software/delta/internal/atomicfile"
)

// Themes, besides following the system, which is "".
const (
	ThemeLight = "light"
	ThemeDark  = "dark"
)

// Default colours of a diff: lines added and removed, and the @@ range of a hunk's
// heading. They are Solarized's green, red and base0 grey, as the circles of
// changed files are.
const (
	DefaultAdded   = "#859900"
	DefaultRemoved = "#dc322f"
	DefaultHunk    = "#839496"
)

const leastWindowSize = 200

// How many commits of the history can be read at a time, and are by default.
const (
	MinHistoryPageSize     = 50
	MaxHistoryPageSize     = 10000
	DefaultHistoryPageSize = 500
)

// The sizes the code's font can be, in points.
const (
	MinCodeFontSize = 6
	MaxCodeFontSize = 72
)

// Panes are the sizes of the two panes of a splitter, in pixels.
type Panes [2]int

// Settings is what is saved.
type Settings struct {
	// Width and Height are the size of the window when it is not maximized.
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximized bool `json:"maximized"`

	// Sidebar is the sidebar and the rest of the window, Lists the unstaged
	// and staged lists, and Editor the diff and the message pane.
	Sidebar Panes `json:"sidebar"`
	Lists   Panes `json:"lists"`
	Editor  Panes `json:"editor"`
	// History is the commits and what is below them, and HistoryDetail the
	// files and the diff, in the history.
	History       Panes `json:"history"`
	HistoryDetail Panes `json:"historyDetail"`
	// RebaseWidth and RebaseHeight are the size of the interactive rebase
	// dialog, 0 until it has been closed once.
	RebaseWidth  int `json:"rebaseWidth,omitempty"`
	RebaseHeight int `json:"rebaseHeight,omitempty"`

	// HistoryPageSize is how many commits of the history are read at a time:
	// on showing it, and again on scrolling to the end of those read.
	HistoryPageSize int `json:"historyPageSize"`

	// ForcePush is whether the next push is forced, as the Force push checkbox
	// says.
	ForcePush bool `json:"forcePush,omitempty"`

	// Theme is ThemeLight, ThemeDark, or empty to follow the system.
	Theme string `json:"theme,omitempty"`

	// AddedColour and RemovedColour colour the text of a diff's lines, as #rrggbb.
	AddedColour   string `json:"addedColour"`
	RemovedColour string `json:"removedColour"`
	// HunkColour is the colour of the @@ range of a hunk's heading, as #rrggbb.
	HunkColour string `json:"hunkColour"`

	// CodeFont is the family of the font of diffs and the commit message, empty
	// for the system's fixed width font, and CodeFontSize its size in points,
	// 0 for that font's size.
	CodeFont     string `json:"codeFont,omitempty"`
	CodeFontSize int    `json:"codeFontSize,omitempty"`
}

// Defaults returns the settings of a first run.
func Defaults() Settings {
	return Settings{
		Width:         1100,
		Height:        760,
		Sidebar:       Panes{300, 800},
		Lists:         Panes{340, 340},
		Editor:        Panes{520, 180},
		History:       Panes{280, 420},
		HistoryDetail: Panes{280, 800},
		AddedColour:   DefaultAdded,
		RemovedColour: DefaultRemoved,
		HunkColour:    DefaultHunk,

		HistoryPageSize: DefaultHistoryPageSize,
	}
}

// Path returns where the settings are kept, making its folder.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "delta")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// Load reads the settings from the file. A missing or unreadable file gives the
// defaults, as does any unusable value in one.
func Load(path string) Settings {
	s := Defaults()
	if data, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(data, &s) != nil {
			s = Defaults()
		}
	}
	s.normalise()
	return s
}

var colourPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// normalise replaces each unusable value with its default.
func (s *Settings) normalise() {
	d := Defaults()
	if s.Width < leastWindowSize || s.Height < leastWindowSize {
		s.Width, s.Height = d.Width, d.Height
	}
	if s.RebaseWidth < leastWindowSize || s.RebaseHeight < leastWindowSize {
		s.RebaseWidth, s.RebaseHeight = 0, 0
	}
	s.Sidebar = s.Sidebar.or(d.Sidebar)
	s.Lists = s.Lists.or(d.Lists)
	s.Editor = s.Editor.or(d.Editor)
	s.History = s.History.or(d.History)
	s.HistoryDetail = s.HistoryDetail.or(d.HistoryDetail)
	if s.HistoryPageSize < MinHistoryPageSize || s.HistoryPageSize > MaxHistoryPageSize {
		s.HistoryPageSize = d.HistoryPageSize
	}
	if s.Theme != ThemeLight && s.Theme != ThemeDark {
		s.Theme = ""
	}
	s.CodeFont = strings.TrimSpace(s.CodeFont)
	if s.CodeFontSize < MinCodeFontSize || s.CodeFontSize > MaxCodeFontSize {
		s.CodeFontSize = 0
	}
	s.AddedColour = colourOr(s.AddedColour, d.AddedColour)
	s.RemovedColour = colourOr(s.RemovedColour, d.RemovedColour)
	s.HunkColour = colourOr(s.HunkColour, d.HunkColour)
}

// or returns the sizes if both panes have some, or else the fallback.
func (p Panes) or(fallback Panes) Panes {
	if p[0] <= 0 || p[1] <= 0 {
		return fallback
	}
	return p
}

// colourOr returns the colour if it is one, or else the fallback.
func colourOr(colour, fallback string) string {
	if !colourPattern.MatchString(colour) {
		return fallback
	}
	return colour
}

// Save writes the settings to the file, which only the user can read.
func (s Settings) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(path, append(data, '\n'), 0o600)
}
