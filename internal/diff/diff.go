// Package diff works out what each line of git's diff output is, so that it can
// be coloured. It knows nothing of the UI.
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
	// Hunk is a heading of a hunk, the @@ line.
	Hunk
	// Meta is anything else: the headers of a file, or a note.
	Meta
)

// cleaner replaces whatever a text widget would take as the end of a block,
// other than a newline, so that a document has one block for each line.
var cleaner = strings.NewReplacer(
	"\r\n", "\n",
	"\r", " ",
	" ", " ",
	"﷐", " ",
	"﷑", " ",
)

// Clean returns the diff ready to be shown, valid and with a line for each of
// git's lines, whatever line endings the files have.
func Clean(text string) string {
	return cleaner.Replace(strings.ToValidUTF8(text, "�"))
}

// Classify returns what each line of the text is, split on newlines.
//
// A line starting "--- " is a removal inside a hunk and a header outside one,
// so it matters which it is in. Every line in a hunk starts with a space, + or
// - (for each parent, in the combined diff of a merge), so a "diff " line can
// only be the start of the next file.
func Classify(text string) []Kind {
	lines := strings.Split(text, "\n")
	kinds := make([]Kind, len(lines))
	// columns is how many characters start each line of the hunk, or 0
	// outside a hunk.
	columns := 0
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff "):
			columns = 0
			kinds[i] = Meta
		case strings.HasPrefix(line, "@@"):
			// @@ for a plain diff, one more @ for each further parent.
			columns = len(line) - len(strings.TrimLeft(line, "@")) - 1
			kinds[i] = Hunk
		case columns == 0:
			kinds[i] = Meta
		default:
			kinds[i] = inHunk(line, columns)
		}
	}
	return kinds
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
