// Package diff turns git's diff output into what is shown: without the headers
// of each file, and saying what each line is so that it can be coloured. It
// knows nothing of the UI.
package diff

import "strings"

// Kind is what a line of a diff is.
type Kind int

const (
	// Context is a line that did not change.
	Context Kind = iota
	// Added is a line that was added.
	Added
	// Removed is a line that was removed.
	Removed
	// Hunk is the heading of a hunk, the @@ line.
	Hunk
	// File is the name of a file, heading its diff.
	File
	// Meta is anything else: a commit's header, a rename, or a note.
	Meta
)

// Parsed is a diff ready to be shown.
type Parsed struct {
	// Text is what to show. Kinds says what each of its lines is, and it has one
	// line for each, whatever line endings the files have.
	Text  string
	Kinds []Kind
	// RangeEnds says, for each Hunk line, where its @@ range ends, in bytes,
	// which is where any text about the hunk starts.
	RangeEnds []int
	// Files are the lines at which the diff of each file starts.
	Files []int
}

// cleaner replaces whatever a text widget would take as the end of a block,
// other than a newline.
var cleaner = strings.NewReplacer(
	"\r\n", "\n",
	"\r", " ",
	"\u2029", " ",
	"\ufdd0", " ",
	"\ufdd1", " ",
)

// hiddenHeaders start the lines of a file's header that are not shown. What is
// left, as a rename, a change of mode or that a file is binary, is news.
var hiddenHeaders = []string{
	"index ", "--- ", "+++ ", "new file mode ", "deleted file mode ",
	"similarity index ", "dissimilarity index ",
}

// Parse makes the output of git ready to be shown. Each file's "diff" line is
// dropped with the rest of its header, or, if titles is set, as when several
// files are shown together, replaced by the file's name.
//
// A line starting "--- " is a removal in a hunk and a header outside one, so it
// matters which it is in. Every line in a hunk starts with a space, + or - (for
// each parent, in the combined diff of a merge), so a "diff " line can only be
// the start of the next file.
func Parse(text string, titles bool) Parsed {
	lines := strings.Split(cleaner.Replace(strings.ToValidUTF8(text, "�")), "\n")
	p := Parsed{}
	shown := make([]string, 0, len(lines))
	add := func(line string, kind Kind, rangeEnd int) {
		shown = append(shown, line)
		p.Kinds = append(p.Kinds, kind)
		p.RangeEnds = append(p.RangeEnds, rangeEnd)
	}
	// columns is how many characters start each line of the hunk, or 0 outside
	// a hunk. inHeader says that a file's header is being read, and title is
	// where its name goes, or -1.
	columns, inHeader, title := 0, false, -1
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff "):
			columns, inHeader, title = 0, true, -1
			p.Files = append(p.Files, len(shown))
			if titles {
				title = len(shown)
				add(nameFromDiffLine(line), File, 0)
			}
		case strings.HasPrefix(line, "@@"):
			inHeader = false
			columns = len(line) - len(strings.TrimLeft(line, "@")) - 1
			add(line, Hunk, rangeEnd(line, columns+1))
		case inHeader && line == "":
			add(line, Context, 0)
		case inHeader:
			if name := nameFromHeader(line); name != "" && title >= 0 {
				shown[title] = name
			}
			if !hasAnyPrefix(line, hiddenHeaders) {
				add(line, Meta, 0)
			}
		case columns == 0:
			add(line, Meta, 0)
		default:
			add(line, inHunk(line, columns), 0)
		}
	}
	p.Text = strings.Join(shown, "\n")
	return p
}

// hasAnyPrefix says whether the line starts with any of the prefixes.
func hasAnyPrefix(line string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// rangeEnd returns where the closing marker of a hunk's heading ends, the marker
// being as many @ as the heading starts with.
func rangeEnd(line string, marker int) int {
	closing := " " + strings.Repeat("@", marker)
	if i := strings.Index(line[marker:], closing); i >= 0 {
		return marker + i + len(closing)
	}
	return len(line)
}

// nameFromHeader returns the name a line of a file's header gives it, if any.
func nameFromHeader(line string) string {
	for _, prefix := range []string{"rename to ", "copy to "} {
		if name, ok := strings.CutPrefix(line, prefix); ok {
			return name
		}
	}
	// git ends a name with spaces in it with a tab. A file that is new or gone
	// has /dev/null on one side, whose other side names it.
	if name, ok := strings.CutPrefix(line, "+++ b/"); ok {
		return strings.TrimRight(name, "\t")
	}
	if name, ok := strings.CutPrefix(line, "--- a/"); ok {
		return strings.TrimRight(name, "\t")
	}
	return ""
}

// nameFromDiffLine returns the name of a file from its "diff" line, for when no
// other line of its header names it, as when only its mode changed. It is
// "diff --git a/name b/name", which can only be cut in two at the middle if the
// name has spaces in it, or "diff --cc name".
func nameFromDiffLine(line string) string {
	if name, ok := strings.CutPrefix(line, "diff --cc "); ok {
		return name
	}
	if names, ok := strings.CutPrefix(line, "diff --git a/"); ok {
		if half := (len(names) - len(" b/")) / 2; half > 0 && strings.HasPrefix(names[half:], " b/") {
			return names[:half]
		}
		return names
	}
	return line
}

// inHunk returns what a line in a hunk is, from its first columns characters.
func inHunk(line string, columns int) Kind {
	prefix := line[:min(columns, len(line))]
	switch {
	case line == "":
		return Context
	case strings.Contains(prefix, "+"):
		return Added
	case strings.Contains(prefix, "-"):
		return Removed
	case strings.Trim(prefix, " ") == "":
		return Context
	}
	// "\ No newline at end of file", or a note of the program's.
	return Meta
}
