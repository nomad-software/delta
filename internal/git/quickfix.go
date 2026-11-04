package git

import (
	"fmt"
	"strings"
)

// QuickFix is what a quick fix would do, read with the status: fold the changes
// not committed into the last commit, or, with none, fold the last commit into
// the one before it, each keeping the message of the commit folded into.
type QuickFix struct {
	// Amend says that there are changes not committed, which go into the last
	// commit; otherwise the last commit goes into the one before it.
	Amend bool
	// Head is the last commit, and Last and Previous the subjects of it and of
	// the one before it, which is empty when amending.
	Head, Last, Previous string
	// Refused says why there can be no quick fix, and is empty if there can.
	Refused string
}

// PlanQuickFix says what a quick fix would do, given whether there are changes
// not committed, or why there can be none.
func (r *Repo) PlanQuickFix(uncommitted bool) QuickFix {
	out, err := r.run("log", "-2", "--format=%H%x00%P%x00%s", "HEAD")
	if err != nil {
		return QuickFix{Amend: uncommitted, Refused: "There are no commits yet."}
	}
	type commit struct {
		hash, subject string
		parents       int
	}
	var commits []commit
	for line := range strings.Lines(out) {
		if fields := strings.SplitN(strings.TrimRight(line, "\n"), "\x00", 3); len(fields) == 3 {
			commits = append(commits, commit{fields[0], fields[2], len(strings.Fields(fields[1]))})
		}
	}
	if len(commits) == 0 {
		return QuickFix{Amend: uncommitted, Refused: "There are no commits yet."}
	}
	q := QuickFix{Amend: uncommitted, Head: commits[0].hash, Last: commits[0].subject}
	switch {
	case uncommitted:
	case commits[0].parents == 0:
		q.Refused = "The last commit has no commit before it to fold into."
	case commits[0].parents > 1:
		q.Refused = "The last commit is a merge."
	case len(commits) < 2:
		q.Refused = "The commit before the last can't be read."
	case commits[1].parents > 1:
		q.Refused = "The commit before the last is a merge."
	default:
		q.Previous = commits[1].subject
	}
	return q
}

// RunQuickFix does what the plan says: it stages everything, untracked files
// included, and amends the last commit with it, keeping its message; or it
// folds the last commit into the one before it, which keeps its message and
// author, and moves the branch to the result only if the last commit is still
// the one planned for. Neither can conflict, as the files come out as they are.
func (r *Repo) RunQuickFix(q QuickFix) (string, error) {
	if q.Amend {
		if _, err := r.run("add", "--all"); err != nil {
			return "", err
		}
		return r.run("commit", "--amend", "--no-edit")
	}
	return r.foldHead(q.Head)
}

// foldHead makes a commit of head's files with the parents, message and author
// of the commit before it, and moves the branch from head to it.
func (r *Repo) foldHead(head string) (string, error) {
	previous := head + "~1"
	raw, err := r.run("cat-file", "commit", previous)
	if err != nil {
		return "", err
	}
	// The message is all after the headers, exactly as it was.
	_, message, _ := strings.Cut(raw, "\n\n")
	who, err := r.run("log", "-1", "--format=%an%x00%ae%x00%ad", "--date=raw", previous)
	if err != nil {
		return "", err
	}
	author := strings.SplitN(strings.TrimRight(who, "\n"), "\x00", 3)
	if len(author) != 3 {
		return "", fmt.Errorf("the author of %s can't be read", previous)
	}
	args := []string{"commit-tree", head + "^{tree}"}
	parents, err := r.run("rev-list", "--parents", "-n", "1", previous)
	if err != nil {
		return "", err
	}
	for _, parent := range strings.Fields(parents)[1:] {
		args = append(args, "-p", parent)
	}
	cmd := r.command(args...)
	cmd.Env = append(cmd.Env, "GIT_AUTHOR_NAME="+author[0], "GIT_AUTHOR_EMAIL="+author[1], "GIT_AUTHOR_DATE="+author[2])
	folded, err := r.runCommand(cmd, message)
	if err != nil {
		return "", err
	}
	// The branch moves only if it is still at head, so that nothing done since
	// the plan was read is lost.
	return r.run("update-ref", "-m", "quick fix: fold into the commit before", "HEAD", strings.TrimSpace(folded), head)
}
