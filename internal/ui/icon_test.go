package ui

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/mappu/miqt/qt6/mainthread"
)

// packagedIcons are the icons kept for packaging, relative to packaging/, with
// how each is drawn.
var packagedIcons = []struct {
	path         string
	size, margin int
}{
	{"merge-monkey.png", appIconSize, 0},
	{filepath.Join("macos", "merge-monkey.png"), macIconSize, macIconMargin},
}

// saveIcon draws an icon into a file.
func saveIcon(t *testing.T, path string, size, margin int) {
	t.Helper()
	mainthread.Wait(func() {
		pixmap := drawIcon(size, margin)
		defer pixmap.Delete()
		if !pixmap.Save(path) {
			t.Errorf("%s could not be written", path)
		}
	})
}

func readImage(t *testing.T, path string) image.Image {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("%v: run make icons", err)
	}
	defer file.Close()
	img, err := png.Decode(file)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return img
}

// TestWriteIcons is make icons: it draws the packaged icons into the folder in
// MERGE_MONKEY_ICONS.
func TestWriteIcons(t *testing.T) {
	dir := os.Getenv("MERGE_MONKEY_ICONS")
	if dir == "" {
		t.Skip("no folder given")
	}
	for _, icon := range packagedIcons {
		saveIcon(t, filepath.Join(dir, icon.path), icon.size, icon.margin)
	}
}

// TestPackagedIconsAreCurrent says when the code drawing the icon has changed
// without make icons being run. Another Qt may round a little differently, which
// is not another icon.
func TestPackagedIconsAreCurrent(t *testing.T) {
	const tolerance = 0x400
	for _, icon := range packagedIcons {
		drawn := filepath.Join(t.TempDir(), "icon.png")
		saveIcon(t, drawn, icon.size, icon.margin)
		want, got := readImage(t, drawn), readImage(t, filepath.Join("..", "..", "packaging", icon.path))
		if got.Bounds() != want.Bounds() {
			t.Fatalf("%s is %v, not %v: run make icons", icon.path, got.Bounds(), want.Bounds())
		}
		var total, count uint64
		for y := 0; y < icon.size; y += 7 {
			for x := 0; x < icon.size; x += 7 {
				r1, g1, b1, a1 := want.At(x, y).RGBA()
				r2, g2, b2, a2 := got.At(x, y).RGBA()
				total += distance(r1, r2) + distance(g1, g2) + distance(b1, b2) + distance(a1, a2)
				count += 4
			}
		}
		if total/count > tolerance {
			t.Errorf("%s differs by %d on average: run make icons", icon.path, total/count)
		}
	}
}

func distance(a, b uint32) uint64 {
	if a > b {
		return uint64(a - b)
	}
	return uint64(b - a)
}

func TestMacIconHasItsMargin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "icon.png")
	saveIcon(t, path, macIconSize, macIconMargin)
	img := readImage(t, path)
	if _, _, _, a := img.At(macIconMargin/2, macIconSize/2).RGBA(); a != 0 {
		t.Error("the margin is not clear")
	}
	if _, _, _, a := img.At(macIconMargin+60, macIconSize/2).RGBA(); a == 0 {
		t.Error("the tile is clear")
	}
}
