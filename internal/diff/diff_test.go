package diff

import (
	"slices"
	"strings"
	"testing"
)

const plain = `diff --git a/f b/f
index 1..2 100644
--- a/f
+++ b/f
@@ -1,3 +1,3 @@ func main()
 same
-gone
--- a removed line that looks like a header
+new
+++ an added line that looks like a header
\ No newline at end of file
diff --git a/g b/g
new file mode 100644
index 0..1
--- /dev/null
+++ b/g
@@ -0,0 +1 @@
+g
diff --git a/h b/h
index 1..2
Binary files a/h and b/h differ
`

func TestParseHidesHeaders(t *testing.T) {
	p := Parse(plain, false)
	want := []string{
		"@@ -1,3 +1,3 @@ func main()", " same", "-gone", "--- a removed line that looks like a header",
		"+new", "+++ an added line that looks like a header", `\ No newline at end of file`,
		"@@ -0,0 +1 @@", "+g", "Binary files a/h and b/h differ", "",
	}
	if got := strings.Split(p.Text, "\n"); !slices.Equal(got, want) {
		t.Errorf("got %q", got)
	}
	kinds := []Kind{Hunk, Context, Removed, Removed, Added, Added, Meta, Hunk, Added, Meta, Context}
	if !slices.Equal(p.Kinds, kinds) {
		t.Errorf("kinds %v", p.Kinds)
	}
	if !slices.Equal(p.Files, []int{0, 7, 9}) {
		t.Errorf("files %v", p.Files)
	}
}

func TestParseTitles(t *testing.T) {
	p := Parse("commit abc\n\n    diff of a message\n\n"+plain+
		"diff --git a/old name b/new name\nsimilarity index 100%\nrename from old name\nrename to new name\n"+
		"diff --git a/gone b/gone\ndeleted file mode 100644\n--- a/gone\n+++ /dev/null\n@@ -1 +0,0 @@\n-x\n"+
		"diff --git a/a b b/a b\nold mode 100644\nnew mode 100755\n"+
		"diff --git a/s p b/s p\n--- a/s p\t\n+++ b/s p\t\n@@ -1 +1 @@\n-a\n+b\n", true)
	lines := strings.Split(p.Text, "\n")
	var names []string
	for _, line := range p.Files {
		if p.Kinds[line] != File {
			t.Errorf("line %d is %v", line, p.Kinds[line])
		}
		names = append(names, lines[line])
	}
	if want := []string{"f", "g", "h", "new name", "gone", "a b", "s p"}; !slices.Equal(names, want) {
		t.Errorf("names %q", names)
	}
	if p.Files[0] != 4 || p.Kinds[2] != Meta || lines[2] != "    diff of a message" {
		t.Errorf("commit header: files %v, %q", p.Files, lines[:4])
	}
	if !slices.Contains(lines, "rename from old name") || !slices.Contains(lines, "old mode 100644") {
		t.Errorf("news was hidden: %q", lines)
	}
	if slices.Contains(lines, "similarity index 100%") {
		t.Errorf("header shown: %q", lines)
	}
}

func TestParseCombined(t *testing.T) {
	p := Parse("diff --cc f\nindex 1,2..0\n--- a/f\n+++ b/f\n@@@ -1,1 -1,1 +1,5 @@@ text\n  same\n++<<<<<<< HEAD\n +ours\n- theirs\n", true)
	want := []Kind{File, Hunk, Context, Added, Added, Removed, Context}
	if !slices.Equal(p.Kinds, want) {
		t.Errorf("kinds %v", p.Kinds)
	}
	line := strings.Split(p.Text, "\n")[1]
	if got := line[:p.RangeEnds[1]]; got != "@@@ -1,1 -1,1 +1,5 @@@" {
		t.Errorf("range %q", got)
	}
}

func TestRangeEnds(t *testing.T) {
	for line, want := range map[string]string{
		"@@ -1 +1 @@":                "@@ -1 +1 @@",
		"@@ -1,6 +1,7 @@ func a() {": "@@ -1,6 +1,7 @@",
		"@@ malformed":               "@@ malformed",
	} {
		if got := line[:rangeEnd(line, 2)]; got != want {
			t.Errorf("%q: got %q", line, got)
		}
	}
}

func TestParseCutOff(t *testing.T) {
	p := Parse("@@ -1 +1 @@\n+a\n[The rest is not shown]\n", false)
	if want := []Kind{Hunk, Added, Meta, Context}; !slices.Equal(p.Kinds, want) {
		t.Errorf("kinds %v", p.Kinds)
	}
}

func TestParseCleans(t *testing.T) {
	if got, want := Parse("a\r\nb\rc d\xffe\n", false).Text, "a\nb c d�e\n"; got != want {
		t.Errorf("got %q", got)
	}
}
