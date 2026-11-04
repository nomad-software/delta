// Package settings keeps what is remembered between runs in a JSON file under
// the system's folder for program configuration. It knows nothing of the UI.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/nomad-software/delta/internal/atomicfile"
)

// Themes, besides following the system, which is "".
const (
	ThemeLight = "light"
	ThemeDark  = "dark"
)

// Default colours of a diff: lines added and removed, and the @@ range of a hunk's
// heading. They are Solarized's green, red and cyan, as the circles of changed
// files are.
const (
	DefaultAdded   = Green
	DefaultRemoved = Red
	DefaultHunk    = Cyan
)

// DefaultChangeColours are the default colours of the circles of changed files:
// Solarized's accents, untracked files green as added ones are (the user's
// choice).
var DefaultChangeColours = ChangeColours{
	Modified:    Blue,
	Added:       Green,
	Deleted:     Red,
	Renamed:     Violet,
	Copied:      Cyan,
	TypeChanged: Yellow,
	Conflict:    Orange,
	Untracked:   Green,
}

// DefaultProgressColour is the default colour of the progress bar's segment,
// Solarized's blue.
const DefaultProgressColour = Blue

// ChangeColours are the colours of the circles of changed files, one for each
// thing that can happen to a file, as #rrggbb.
type ChangeColours struct {
	Modified    string `json:"modified"`
	Added       string `json:"added"`
	Deleted     string `json:"deleted"`
	Renamed     string `json:"renamed"`
	Copied      string `json:"copied"`
	TypeChanged string `json:"typeChanged"`
	Conflict    string `json:"conflict"`
	Untracked   string `json:"untracked"`
}

// Fields returns where each colour is kept, in the order of the fields: Modified,
// Added, Deleted, Renamed, Copied, TypeChanged, Conflict, Untracked.
func (c *ChangeColours) Fields() []*string {
	return []*string{&c.Modified, &c.Added, &c.Deleted, &c.Renamed, &c.Copied, &c.TypeChanged, &c.Conflict, &c.Untracked}
}

// leastWindowSize is the least width and height of a window that is kept; a
// smaller one is taken as unusable.
const leastWindowSize = 200

// How many repositories opened can be remembered, and are by default.
const (
	MinRepositoryLimit     = 1
	MaxRepositoryLimit     = 100
	DefaultRepositoryLimit = 20
)

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
	// ChangeColours are those of the circles of changed files, and
	// ProgressColour that of the progress bar's segment, each as #rrggbb.
	ChangeColours  ChangeColours `json:"changeColours"`
	ProgressColour string        `json:"progressColour"`

	// CodeFont is the family of the font of diffs and the commit message, empty
	// for the system's fixed width font, and CodeFontSize its size in points,
	// 0 for that font's size.
	CodeFont     string `json:"codeFont,omitempty"`
	CodeFontSize int    `json:"codeFontSize,omitempty"`

	// Repositories are the top folders of the repositories opened, each once,
	// as absolute paths, so that they can be opened again quickly: the one
	// opened longest ago first, and at most RepositoryLimit of them.
	Repositories    []string `json:"repositories,omitempty"`
	RepositoryLimit int      `json:"repositoryLimit"`
	// LastRepository is the top folder of the repository last opened, which is
	// opened on starting where no other is to be. Older versions kept one as
	// "repository", which is ignored.
	LastRepository string `json:"lastRepository,omitempty"`
}

// Defaults returns the settings of a first run.
func Defaults() Settings {
	return Settings{
		Width:         1320,
		Height:        760,
		Sidebar:       Panes{300, 800},
		Lists:         Panes{340, 340},
		Editor:        Panes{520, 180},
		History:       Panes{280, 420},
		HistoryDetail: Panes{280, 800},
		AddedColour:   DefaultAdded,
		RemovedColour: DefaultRemoved,
		HunkColour:    DefaultHunk,

		ChangeColours:  DefaultChangeColours,
		ProgressColour: DefaultProgressColour,

		HistoryPageSize: DefaultHistoryPageSize,
		RepositoryLimit: DefaultRepositoryLimit,
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
	fallbacks := d.ChangeColours.Fields()
	for i, colour := range s.ChangeColours.Fields() {
		*colour = colourOr(*colour, *fallbacks[i])
	}
	s.ProgressColour = colourOr(s.ProgressColour, d.ProgressColour)
	if filepath.IsAbs(s.LastRepository) {
		s.LastRepository = filepath.Clean(s.LastRepository)
	} else {
		s.LastRepository = ""
	}
	if s.RepositoryLimit < MinRepositoryLimit || s.RepositoryLimit > MaxRepositoryLimit {
		s.RepositoryLimit = d.RepositoryLimit
	}
	paths := s.Repositories
	s.Repositories = nil
	for _, path := range paths {
		s.Remember(path)
	}
}

// LimitRepositories drops the repositories opened longest ago beyond
// RepositoryLimit, and says whether any were.
func (s *Settings) LimitRepositories() bool {
	extra := len(s.Repositories) - max(s.RepositoryLimit, MinRepositoryLimit)
	if extra <= 0 {
		return false
	}
	s.Repositories = slices.Delete(s.Repositories, 0, extra)
	return true
}

// Remember makes the repository's folder the one opened last, once, unless it
// is not an absolute path, dropping the one opened longest ago if there are then
// more than RepositoryLimit. It says whether the list changed.
func (s *Settings) Remember(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	path = filepath.Clean(path)
	if n := len(s.Repositories); n > 0 && s.Repositories[n-1] == path {
		return false
	}
	s.Repositories = append(slices.DeleteFunc(s.Repositories, func(p string) bool { return p == path }), path)
	s.LimitRepositories()
	return true
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
