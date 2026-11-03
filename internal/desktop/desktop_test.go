package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEntry(t *testing.T) {
	text := entry("/opt/my delta/app", "app", "/icons/delta.png")
	for _, line := range []string{
		"[Desktop Entry]", "Type=Application", "Name=Delta",
		`Exec="/opt/my delta/app"`, "TryExec=/opt/my delta/app",
		"Icon=/icons/delta.png", "Terminal=false", "StartupWMClass=app",
	} {
		if !strings.Contains(text, line+"\n") {
			t.Errorf("no %q in\n%s", line, text)
		}
	}
}

func TestQuoteArgument(t *testing.T) {
	for path, want := range map[string]string{
		"/usr/bin/app": "/usr/bin/app",
		"/a b/app":     `"/a b/app"`,
		"/100%/app":    "/100%%/app",
		`/a"b/app`:     `"/a\"b/app"`,
		"/a$b/app":     `"/a\$b/app"`,
		`/a\b/app`:     `"/a\\b/app"`,
		"/a`b/app":     "\"/a\\`b/app\"",
	} {
		if got := quoteArgument(path); got != want {
			t.Errorf("%q: got %q, not %q", path, got, want)
		}
	}
}

func TestEscapeValue(t *testing.T) {
	if got, want := escapeValue("a\\b\nc\td"), `a\\b\nc\td`; got != want {
		t.Errorf("got %q", got)
	}
}

func TestLauncher(t *testing.T) {
	t.Setenv("APPIMAGE", "/some/Merge.AppImage")
	if got, err := launcher(); err != nil || got != "/some/Merge.AppImage" {
		t.Errorf("in an AppImage: %q, %v", got, err)
	}
	t.Setenv("APPIMAGE", "")
	got, err := launcher()
	if err != nil || !filepath.IsAbs(got) {
		t.Errorf("outside one: %q, %v", got, err)
	}
}

func TestDataFolder(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/xdg/data")
	if got, _ := dataFolder(); got != "/xdg/data" {
		t.Errorf("got %q", got)
	}
	t.Setenv("HOME", "/home/someone")
	for _, ignored := range []string{"", "relative/path"} {
		t.Setenv("XDG_DATA_HOME", ignored)
		if got, _ := dataFolder(); got != "/home/someone/.local/share" {
			t.Errorf("with %q: got %q", ignored, got)
		}
	}
}

func TestInstall(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("APPIMAGE", "/some/Merge.AppImage")
	if err := Install([]byte("image")); err != nil {
		t.Fatal(err)
	}
	icon := filepath.Join(data, "icons", "hicolor", "256x256", "apps", "delta.png")
	if got, err := os.ReadFile(icon); err != nil || string(got) != "image" {
		t.Errorf("icon %q, %v", got, err)
	}
	path := filepath.Join(data, "applications", "delta.desktop")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "Exec=/some/Merge.AppImage\n") || !strings.Contains(string(got), "Icon="+icon+"\n") {
		t.Errorf("entry\n%s", got)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("mode %v, %v", info.Mode(), err)
	}
}

func TestOnlyWrittenWhenChanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "file")
	if err := writeIfChanged(path, []byte("one")); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if err := writeIfChanged(path, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(path); !info.ModTime().Equal(old) {
		t.Error("the same data was written again")
	}
	if err := writeIfChanged(path, []byte("two")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "two" {
		t.Errorf("got %q", got)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("left behind %v", entries)
	}
}

func TestFailureIsReported(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := install(blocked, "/app", "app", []byte("image")); err == nil {
		t.Error("no error with a file in place of the folder")
	}
}

// TestPackagedEntryAgrees checks that the entry an AppImage carries says what the
// one written at every start does.
func TestPackagedEntryAgrees(t *testing.T) {
	packaged, err := os.ReadFile(filepath.Join("..", "..", "packaging", Name+".desktop"))
	if err != nil {
		t.Fatal(err)
	}
	written := entry("/app", "app", "/icon")
	for _, key := range []string{"Name=", "Comment=", "Categories="} {
		start := strings.Index(written, key)
		line := written[start : start+strings.IndexByte(written[start:], '\n')]
		if !strings.Contains(string(packaged), line+"\n") {
			t.Errorf("packaged entry has no %q", line)
		}
	}
}

// TestOldIconIsRemoved checks that the icon written at 128 pixels, before it was
// 256, is taken away.
func TestOldIconIsRemoved(t *testing.T) {
	data := t.TempDir()
	old := filepath.Join(data, "icons", "hicolor", "128x128", "apps", "delta.png")
	if err := os.MkdirAll(filepath.Dir(old), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := install(data, "/app", "app", []byte("icon")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("the old icon is still there")
	}
	// Nor is it a failure when there is none.
	if err := install(data, "/app", "app", []byte("icon")); err != nil {
		t.Error(err)
	}
}
