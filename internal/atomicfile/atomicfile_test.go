package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	for _, text := range []string{"one", "two"} {
		if err := Write(path, []byte(text), 0o640); err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != text {
			t.Errorf("got %q, %v", got, err)
		}
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Errorf("mode %v, %v", info.Mode(), err)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("left behind %v", entries)
	}
}

func TestFailureLeavesTheFolderClean(t *testing.T) {
	dir := t.TempDir()
	if err := Write(filepath.Join(dir, "missing", "file"), []byte("one"), 0o644); err == nil {
		t.Error("no error with no folder to write in")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("left behind %v", entries)
	}
}
