package ui

import (
	"math"
	"testing"

	"github.com/nomad-software/qute/qtwidgets"
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
		holder := qtwidgets.NewQWidget(nil)
		defer holder.Delete()
		holder.SetLayout(title)
		if title.Count() != 3 {
			t.Fatalf("%d items in the title", title.Count())
		}
		name := qtwidgets.QLabelCast(title.ItemAt(0).Widget())
		normal := qtwidgets.QApplicationFont()
		if !name.Font().Bold() || math.Abs(name.Font().PointSizeF()-normal.PointSizeF()*titleScale) > 0.01 {
			t.Errorf("name %q at %v points, bold %v", name.Text(), name.Font().PointSizeF(), name.Font().Bold())
		}
		if title.ItemAt(1).Widget().Ptr() != label.Ptr() || label.Text() != version() {
			t.Errorf("version %q is not beside the name", label.Text())
		}
		if math.Abs(label.Font().PointSizeF()-normal.PointSizeF()*versionScale) > 0.01 || label.Font().Bold() {
			t.Errorf("version at %v points", label.Font().PointSizeF())
		}
		effect := label.GraphicsEffect()
		if effect == nil || qtwidgets.QGraphicsOpacityEffectCast(effect).Opacity() != versionOpacity {
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
		if want := d.Layout().AsQLayoutItem().HeightForWidth(d.Width()); d.Height() != want {
			t.Errorf("%d high, where its contents need %d", d.Height(), want)
		}
		d.Hide()
	})
}
