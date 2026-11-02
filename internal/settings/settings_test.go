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
	s.Theme = ThemeDark
	s.AddedColour, s.RemovedColour, s.HunkColour = "#010203", "#040506", "#070809"
	s.CodeFont, s.CodeFontSize = "Some Font", 14
	s.Repository = "/some/where"
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
	write(`{"width": 5, "height": 5, "sidebar": [0, 10], "lists": [-1, 2], "editor": [3, 0], "history": [0, 1], "historyDetail": [1, -1],
		"codeFont": "  ", "codeFontSize": 500, "theme": "purple", "addedColour": "green", "removedColour": "#12", "hunkColour": "grey"}`)
	if got := Load(path); got != Defaults() {
		t.Errorf("got %+v", got)
	}
	write("not json")
	if got := Load(path); got != Defaults() {
		t.Errorf("got %+v", got)
	}
}
