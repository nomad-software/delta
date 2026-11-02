// Package settings keeps what is remembered between runs in a JSON file under
// the system's folder for program configuration. It knows nothing of the UI.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
)

// Themes, besides following the system, which is "".
const (
	ThemeLight = "light"
	ThemeDark  = "dark"
)

// Default colours of the lines of a diff.
const (
	DefaultAdded   = "#2ea043"
	DefaultRemoved = "#e5534b"
)

const leastWindowSize = 200

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

	// Theme is ThemeLight, ThemeDark, or empty to follow the system.
	Theme string `json:"theme,omitempty"`

	// AddedColour and RemovedColour tint the lines of a diff, as #rrggbb.
	AddedColour   string `json:"addedColour"`
	RemovedColour string `json:"removedColour"`

	// Repository is the repository last opened.
	Repository string `json:"repository,omitempty"`
}

// Defaults returns the settings of a first run.
func Defaults() Settings {
	return Settings{
		Width:         1100,
		Height:        760,
		Sidebar:       Panes{300, 800},
		Lists:         Panes{340, 340},
		Editor:        Panes{520, 180},
		AddedColour:   DefaultAdded,
		RemovedColour: DefaultRemoved,
	}
}

// Path returns where the settings are kept, making its folder.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "merge-monkey")
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
	if s.Sidebar[0] <= 0 || s.Sidebar[1] <= 0 {
		s.Sidebar = d.Sidebar
	}
	if s.Lists[0] <= 0 || s.Lists[1] <= 0 {
		s.Lists = d.Lists
	}
	if s.Editor[0] <= 0 || s.Editor[1] <= 0 {
		s.Editor = d.Editor
	}
	if s.Theme != ThemeLight && s.Theme != ThemeDark {
		s.Theme = ""
	}
	if !colourPattern.MatchString(s.AddedColour) {
		s.AddedColour = d.AddedColour
	}
	if !colourPattern.MatchString(s.RemovedColour) {
		s.RemovedColour = d.RemovedColour
	}
}

// Save writes the settings to the file, through a temporary file so that a
// failure leaves the old one whole.
func (s Settings) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
