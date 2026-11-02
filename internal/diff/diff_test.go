package diff

import (
	"slices"
	"strings"
	"testing"
)

func TestClassify(t *testing.T) {
	lines := []struct {
		text string
		kind Kind
	}{
		{"diff --git a/f b/f", Meta},
		{"index 1..2 100644", Meta},
		{"--- a/f", Meta},
		{"+++ b/f", Meta},
		{"@@ -1,3 +1,3 @@ func main()", Hunk},
		{" same", Context},
		{"-gone", Removed},
		{"--- a removed line that looks like a header", Removed},
		{"+new", Added},
		{"+++ an added line that looks like a header", Added},
		{"\\ No newline at end of file", Meta},
		{"", Context},
		{"diff --git a/g b/g", Meta},
		{"--- a/g", Meta},
		{"Binary files a/g and b/g differ", Meta},
	}
	var text []string
	var want []Kind
	for _, l := range lines {
		text = append(text, l.text)
		want = append(want, l.kind)
	}
	if got := Classify(strings.Join(text, "\n")); !slices.Equal(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}

func TestClassifyCombined(t *testing.T) {
	text := "diff --cc f\n--- a/f\n+++ b/f\n@@@ -1,1 -1,1 +1,5 @@@\n  same\n++<<<<<<< HEAD\n +ours\n- theirs\n"
	want := []Kind{Meta, Meta, Meta, Hunk, Context, Added, Added, Removed, Context}
	if got := Classify(text); !slices.Equal(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}

func TestClassifyCutOff(t *testing.T) {
	got := Classify("@@ -1 +1 @@\n+a\n[The rest is not shown]\n")
	want := []Kind{Hunk, Added, Meta, Context}
	if !slices.Equal(got, want) {
		t.Errorf("got %v", got)
	}
}

func TestClean(t *testing.T) {
	got := Clean("a\r\nb\rc d\xffe\n")
	if want := "a\nb c d�e\n"; got != want {
		t.Errorf("got %q", got)
	}
}
