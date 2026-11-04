package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestMissingFileGivesDefaults(t *testing.T) {
	if got := Load(filepath.Join(t.TempDir(), "none.json")); !reflect.DeepEqual(got, Defaults()) {
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
	s.ChangeColours = ChangeColours{"#000001", "#000002", "#000003", "#000004", "#000005", "#000006", "#000007", "#000008"}
	s.ProgressColour = "#abcdef"
	s.CodeFont, s.CodeFontSize = "Some Font", 14
	s.Repositories = []string{"/one", "/two"}
	s.LastRepository = "/two"
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	if got := Load(path); !reflect.DeepEqual(got, s) {
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
		"codeFont": "  ", "codeFontSize": 500, "lastRepository": "relative", "theme": "purple", "addedColour": "green", "removedColour": "#12", "hunkColour": "grey",
		"changeColours": {"modified": "blue", "untracked": ""}, "progressColour": "#ggg000"}`)
	if got := Load(path); !reflect.DeepEqual(got, Defaults()) {
		t.Errorf("got %+v", got)
	}
	write("not json")
	if got := Load(path); !reflect.DeepEqual(got, Defaults()) {
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

// The repositories remembered are absolute paths, each once, however they are
// written in the file.
func TestRepositories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"repositories": ["/a/b", "/a/b/", "relative", "", "/a/./b", "/c", "/a/b"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Load(path)
	if want := []string{"/a/b", "/c"}; !slices.Equal(s.Repositories, want) {
		t.Errorf("loaded %q", s.Repositories)
	}
	if s.Remember("/c/") || s.Remember("/a/b") || s.Remember("d") {
		t.Error("remembered one already there, or a relative path")
	}
	if !s.Remember("/d") || !slices.Equal(s.Repositories, []string{"/a/b", "/c", "/d"}) {
		t.Errorf("remembered %q", s.Repositories)
	}
}

// Colours given in part keep those that are usable, the rest their defaults.
func TestColoursInPart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"changeColours": {"added": "#111111", "deleted": "nope"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Load(path)
	want := DefaultChangeColours
	want.Added = "#111111"
	if s.ChangeColours != want {
		t.Errorf("changes %+v", s.ChangeColours)
	}
	if s.ProgressColour != DefaultProgressColour {
		t.Errorf("progress %s", s.ProgressColour)
	}
}

// Fields gives every colour of a ChangeColours, each once, in the order of its
// fields, as normalising and the settings dialog rely on.
func TestChangeColourFields(t *testing.T) {
	var c ChangeColours
	fields := c.Fields()
	value := reflect.ValueOf(&c).Elem()
	if len(fields) != value.NumField() {
		t.Fatalf("%d fields given of %d", len(fields), value.NumField())
	}
	for i, field := range fields {
		if field != value.Field(i).Addr().Interface().(*string) {
			t.Errorf("field %d is not %s", i, value.Type().Field(i).Name)
		}
	}
}
