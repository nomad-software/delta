package appicon

import (
	"bytes"
	"image/png"
	"testing"
)

// The desktop entry is written with the icon as it is, so it must be a PNG of
// the size it is installed as.
func TestPNG(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(PNG))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != Size || b.Dy() != Size {
		t.Errorf("the icon is %v, not %d×%d", b, Size, Size)
	}
	// Its corners are clear, as it is round.
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
		t.Error("the corner is not clear")
	}
}
