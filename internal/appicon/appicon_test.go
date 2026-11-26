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
	// Its top corners are clear, as it is a triangle.
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
		t.Error("the corner is not clear")
	}
}

// The picture is as wide as the icon, with no margin at its sides, and in the
// middle of its height, as an equilateral triangle that wide is not as high.
func TestPNGFillsItsWidth(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(PNG))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	left, right, top, bottom := b.Max.X, b.Min.X, b.Max.Y, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0 {
				left, right = min(left, x), max(right, x)
				top, bottom = min(top, y), max(bottom, y)
			}
		}
	}
	if left != b.Min.X || right != b.Max.X-1 {
		t.Errorf("the picture is drawn from %d to %d across, not the icon's width", left, right)
	}
	above, below := top-b.Min.Y, b.Max.Y-1-bottom
	if d := above - below; d < -1 || d > 1 {
		t.Errorf("there are %d rows clear above the picture and %d below", above, below)
	}
	// An equilateral triangle is √3/2 as high as it is wide.
	width, height := float64(right-left+1), float64(bottom-top+1)
	if ratio := height / width; ratio < 0.86 || ratio > 0.89 {
		t.Errorf("the picture is %v×%v, not an equilateral triangle's", width, height)
	}
}
