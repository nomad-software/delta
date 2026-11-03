package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMissingFileGivesDefaults(t *testing.T) {
	if got := Load(filepath.Join(t.TempDir(), "none.json")); got != Defaults() {
		t.Errorf("got %+v", got)
	}
}

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := Defaults()
	s.Width, s.Height, s.Maximized = 1000, 700, true
	s.Sidebar, s.Lists, s.Editor = Panes{1, 2}, Panes{3, 4}, Panes{5, 6}
	s.History, s.HistoryDetail = Panes{7, 8}, Panes{9, 10}
	s.RebaseWidth, s.RebaseHeight = 900, 600
	s.ForcePush = true
	s.HistoryPageSize = 2000
	s.Theme = ThemeDark
	s.AddedColour, s.RemovedColour, s.HunkColour = "#010203", "#040506", "#070809"
	s.CodeFont, s.CodeFontSize = "Some Font", 14
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	if got := Load(path); got != s {
		t.Errorf("got %+v\nwant %+v", got, s)
	}
}

func TestUnusableValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	write := func(text string) {
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"width": 5, "height": 5, "sidebar": [0, 10], "lists": [-1, 2], "editor": [3, 0], "history": [0, 1], "historyDetail": [1, -1], "rebaseWidth": 900, "rebaseHeight": 3, "historyPageSize": 10,
		"codeFont": "  ", "codeFontSize": 500, "theme": "purple", "addedColour": "green", "removedColour": "#12", "hunkColour": "grey"}`)
	if got := Load(path); got != Defaults() {
		t.Errorf("got %+v", got)
	}
	write("not json")
	if got := Load(path); got != Defaults() {
		t.Errorf("got %+v", got)
	}
}

func TestHistoryPageSizeBounds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	for text, want := range map[string]int{
		`{}`:                         DefaultHistoryPageSize,
		`{"historyPageSize": 49}`:    DefaultHistoryPageSize,
		`{"historyPageSize": 50}`:    50,
		`{"historyPageSize": 10000}`: 10000,
		`{"historyPageSize": 10001}`: DefaultHistoryPageSize,
	} {
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := Load(path).HistoryPageSize; got != want {
			t.Errorf("%s gives %d, not %d", text, got, want)
		}
	}
}

// TestOldDefaultColoursFollow checks that colours saved while they were the old
// defaults become the new ones, Solarized's, and any other is kept.
func TestOldDefaultColoursFollow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"addedColour": "#00A000", "removedColour": "#ff1919", "hunkColour": "#123456"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Load(path)
	if s.AddedColour != DefaultAdded || s.RemovedColour != DefaultRemoved || s.HunkColour != "#123456" {
		t.Errorf("colours %s %s %s", s.AddedColour, s.RemovedColour, s.HunkColour)
	}
	if DefaultAdded != "#859900" || DefaultRemoved != "#dc322f" || DefaultHunk != "#839496" {
		t.Error("the defaults are not Solarized's")
	}
}
