package ui

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/mappu/miqt/qt6/mainthread"
)

// macIconPath is where the macOS app's icon is kept, relative to packaging/. It
// is the only icon kept for packaging: the AppImage has the program's own.
var macIconPath = filepath.Join("macos", "delta.png")

// saveIcon draws the macOS app's icon into a file.
func saveIcon(t *testing.T, path string) {
	t.Helper()
	mainthread.Wait(func() {
		pixmap := drawMacIcon(macIconSize, macIconMargin)
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

// TestWriteIcons is make icons: it draws the macOS app's icon into the folder in
// DELTA_ICONS.
func TestWriteIcons(t *testing.T) {
	dir := os.Getenv("DELTA_ICONS")
	if dir == "" {
		t.Skip("no folder given")
	}
	saveIcon(t, filepath.Join(dir, macIconPath))
}

// TestPackagedIconsAreCurrent says when the icon, or the code drawing it for
// packaging, has changed without make icons being run. Another Qt may round a
// little differently, which is not another icon.
func TestPackagedIconsAreCurrent(t *testing.T) {
	const tolerance = 0x400
	drawn := filepath.Join(t.TempDir(), "icon.png")
	saveIcon(t, drawn)
	want, got := readImage(t, drawn), readImage(t, filepath.Join("..", "..", "packaging", macIconPath))
	if got.Bounds() != want.Bounds() {
		t.Fatalf("%s is %v, not %v: run make icons", macIconPath, got.Bounds(), want.Bounds())
	}
	var total, count uint64
	for y := 0; y < macIconSize; y += 7 {
		for x := 0; x < macIconSize; x += 7 {
			r1, g1, b1, a1 := want.At(x, y).RGBA()
			r2, g2, b2, a2 := got.At(x, y).RGBA()
			total += distance(r1, r2) + distance(g1, g2) + distance(b1, b2) + distance(a1, a2)
			count += 4
		}
	}
	if total/count > tolerance {
		t.Errorf("%s differs by %d on average: run make icons", macIconPath, total/count)
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
	saveIcon(t, path)
	img := readImage(t, path)
	if _, _, _, a := img.At(macIconMargin/2, macIconSize/2).RGBA(); a != 0 {
		t.Error("the margin is not clear")
	}
	if _, _, _, a := img.At(macIconSize/2, macIconSize/2).RGBA(); a == 0 {
		t.Error("the icon is not in the middle")
	}
}
