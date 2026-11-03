package ui

import (
	"math"
	"testing"

	qt "github.com/mappu/miqt/qt6"
)

func TestRelease(t *testing.T) {
	for version, want := range map[string]string{
		"v1.2.3":                               "v1.2.3",
		"v1.2.3+dirty":                         "v1.2.3",
		"v1.2.4-0.20261027184954-00c19a9187bb": "v1.2.3",
		"v1.3.0-0.20261027184954-00c19a9187bb": "v1.3.0",
		"v1.2.4-rc.1":                          "v1.2.4",
		"v0.0.0-20261027184954-00c19a9187bb":   "v0.0.0",
	} {
		if got := release(version); got != want {
			t.Errorf("release(%q) = %q, want %q", version, got, want)
		}
	}
}

// TestAboutTitle checks that the title is set out as apemail's is: the name in
// bold at half as big again, and the version beside it, smaller and half
// transparent, on the same line.
func TestAboutTitle(t *testing.T) {
	on(func() {
		title, label := newAboutTitle()
		holder := qt.NewQWidget2()
		defer holder.Delete()
		holder.SetLayout(title.QLayout)
		if title.Count() != 3 {
			t.Fatalf("%d items in the title", title.Count())
		}
		name := qt.UnsafeNewQLabel(title.ItemAt(0).Widget().UnsafePointer())
		normal := qt.QApplication_Font()
		if !name.Font().Bold() || math.Abs(name.Font().PointSizeF()-normal.PointSizeF()*titleScale) > 0.01 {
			t.Errorf("name %q at %v points, bold %v", name.Text(), name.Font().PointSizeF(), name.Font().Bold())
		}
		if title.ItemAt(1).Widget().UnsafePointer() != label.UnsafePointer() || label.Text() != version() {
			t.Errorf("version %q is not beside the name", label.Text())
		}
		if math.Abs(label.Font().PointSizeF()-normal.PointSizeF()*versionScale) > 0.01 || label.Font().Bold() {
			t.Errorf("version at %v points", label.Font().PointSizeF())
		}
		effect := label.GraphicsEffect()
		if effect == nil || qt.UnsafeNewQGraphicsOpacityEffect(effect.UnsafePointer()).Opacity() != versionOpacity {
			t.Error("the version is not half transparent")
		}
	})
}

// TestAboutHasNoGap checks that the about box is only as high as its contents
// need at its width, so that there is no gap between what the program is and
// the licences, as there was at the height Qt gave it.
func TestAboutHasNoGap(t *testing.T) {
	on(func() {
		icon := newAppIcon()
		defer icon.Delete()
		d := newAboutDialog(nil, icon)
		defer d.Delete()
		d.Show()
		if want := d.Layout().HeightForWidth(d.Width()); d.Height() != want {
			t.Errorf("%d high, where its contents need %d", d.Height(), want)
		}
		d.Hide()
	})
}
