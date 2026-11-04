// Package diff turns git's diff output into what is shown: without the headers
// of each file, and saying what each line is so that it can be coloured. It
// knows nothing of the UI.
package diff

import (
	"slices"
	"strings"
)

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
	// Meta is anything else: a rename, a change of mode, or a note.
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

// Parse makes the output of git ready to be shown, a file at a time: each
// file's "diff" line is dropped with the rest of its header, as the file is
// named above the diff.
//
// A line starting "--- " is a removal in a hunk and a header outside one, so it
// matters which it is in. Every line in a hunk starts with a space, + or - (for
// each parent, in the combined diff of a merge), so a "diff " line can only be
// the start of the next file.
func Parse(text string) Parsed {
	lines := strings.Split(cleaner.Replace(strings.ToValidUTF8(text, "�")), "\n")
	p := Parsed{}
	shown := make([]string, 0, len(lines))
	add := func(line string, kind Kind, rangeEnd int) {
		shown = append(shown, line)
		p.Kinds = append(p.Kinds, kind)
		p.RangeEnds = append(p.RangeEnds, rangeEnd)
	}
	// columns is how many characters start each line of the hunk, or 0 outside
	// a hunk. inHeader says that a file's header is being read.
	columns, inHeader := 0, false
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff "):
			columns, inHeader = 0, true
		case strings.HasPrefix(line, "@@"):
			inHeader = false
			columns = len(line) - len(strings.TrimLeft(line, "@")) - 1
			add(line, Hunk, rangeEnd(line, columns+1))
		case inHeader && line == "":
			add(line, Context, 0)
		case inHeader:
			if !slices.ContainsFunc(hiddenHeaders, func(prefix string) bool { return strings.HasPrefix(line, prefix) }) {
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

// rangeEnd returns where the closing marker of a hunk's heading ends, the marker
// being as many @ as the heading starts with.
func rangeEnd(line string, marker int) int {
	closing := " " + strings.Repeat("@", marker)
	if i := strings.Index(line[marker:], closing); i >= 0 {
		return marker + i + len(closing)
	}
	return len(line)
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
