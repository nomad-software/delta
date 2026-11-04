package git

import "strings"

// noCommits is why there is nothing to do to the last commit on a branch with
// none.
const noCommits = "There are no commits yet."

// Tip is the last commit and the one before it, as they were read, which is what
// a quick fix and uncommitting need to know, so that a rescan reads them once.
type Tip struct {
	// commits are the last commit and the one before it, newest first: none on
	// a branch with no commits, and one if the last has no parent.
	commits []tipCommit
}

// tipCommit is a commit of the tip: its hash, its parents' and its subject.
type tipCommit struct {
	hash, subject string
	parents       []string
}

// ReadTip reads the last commit and the one before it. On a branch with no
// commits, or if they can't be read, it holds none.
func (r *Repo) ReadTip() Tip {
	out, err := r.run("log", "-2", "--format=%H%x00%P%x00%s", "HEAD")
	if err != nil {
		return Tip{}
	}
	var t Tip
	for line := range strings.Lines(out) {
		if fields := strings.SplitN(strings.TrimRight(line, "\n"), "\x00", 3); len(fields) == 3 {
			t.commits = append(t.commits, tipCommit{hash: fields[0], parents: strings.Fields(fields[1]), subject: fields[2]})
		}
	}
	return t
}
