package git

import "errors"

// UncommitRefused says why the last commit can't be uncommitted, or is empty if
// it can: there must be one, and it must not be a merge, as taking a merge back
// would mix the branch merged in with the changes.
func (t Tip) UncommitRefused() string {
	switch {
	case len(t.commits) == 0:
		return noCommits
	case len(t.commits[0].parents) > 1:
		return "The last commit is a merge."
	}
	return ""
}

// Uncommit takes the last commit back, leaving its changes in the work tree,
// unstaged as if they were new, as are any staged before. The first commit of a
// branch is taken back to none, its files untracked.
func (r *Repo) Uncommit() (string, error) {
	t := r.ReadTip()
	if refused := t.UncommitRefused(); refused != "" {
		return "", errors.New(refused)
	}
	last := t.commits[0]
	if len(last.parents) == 0 {
		// The branch goes back to having no commits, only if it is still at
		// this one, and nothing is staged.
		if _, err := r.run("update-ref", "-d", "HEAD", last.hash); err != nil {
			return "", err
		}
		return r.run("read-tree", "--empty")
	}
	return r.run("reset", "--mixed", "-q", last.parents[0])
}
