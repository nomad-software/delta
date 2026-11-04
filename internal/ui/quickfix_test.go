package ui

import (
	"strings"
	"testing"

	"github.com/nomad-software/delta/internal/git"
)

// Quick Fix, in the Commit menu with no shortcut and a button under Commit,
// folds every change not committed into the last commit, or with none the last
// commit into the one before it, asking first every time.
func TestQuickFix(t *testing.T) {
	dir := newRepo(t)
	runGit(t, dir, "commit", "-q", "--allow-empty", "-m", "second")
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) && w.quickFix.Head != "" })
	on(func() {
		if key := w.quickFixAction.Shortcut().ToString(); key != "" {
			t.Errorf("Quick Fix is on %q", key)
		}
		// The button is under Commit, and does what the menu item does.
		v := w.changes
		if commit, quick, push := v.commit.Geometry(), v.quickFix.Geometry(), v.push.Geometry(); quick.X() != commit.X() || quick.Y() <= commit.Y() || quick.Y() >= push.Y() {
			t.Errorf("Quick Fix is at %d, %d; Commit at %d, %d; Push at %d", quick.X(), quick.Y(), commit.X(), commit.Y(), push.Y())
		}
		if !w.quickFixAction.IsEnabled() || !w.changes.quickFix.IsEnabled() {
			t.Error("Quick Fix is disabled with a commit to fold")
		}
		// Answered no, nothing happens.
		w.answer = false
		w.changes.quickFix.Click()
		if len(w.asked) != 1 || !strings.Contains(w.asked[0], "“second”, into the one before it, “first”") || w.busy {
			t.Errorf("asked %q, busy %v", w.asked, w.busy)
		}
		w.answer = true
		w.changes.quickFix.Click()
	})
	waitFor(t, "the fold", func() bool { return w.working == 0 && !w.busy && len(w.asked) == 2 })
	if got := strings.TrimSpace(runGitOut(t, dir, "log", "--format=%s")); got != "first" {
		t.Errorf("commits %q", got)
	}

	// With changes, untracked files too, they go into the last commit.
	writeFile(t, dir, "file", "changed\n")
	writeFile(t, dir, "loose", "new\n")
	on(w.rescanRepository)
	waitFor(t, "the changes", func() bool { return w.idleWith(2, 0) && w.quickFix.Amend })
	on(func() {
		w.quickFixAction.Trigger()
		if len(w.asked) != 3 || !strings.Contains(w.asked[2], "untracked files included, into the last commit, “first”") {
			t.Errorf("asked %q", w.asked)
		}
	})
	waitFor(t, "the amend", func() bool { return w.idleWith(0, 0) && !w.quickFix.Amend })
	if got := runGitOut(t, dir, "show", "--name-only", "--format=%s", "HEAD"); got != "first\n\nfile\nloose\n" {
		t.Errorf("the last commit is %q", got)
	}
	// The root commit can't be folded, so with nothing to commit it is off.
	on(func() {
		if w.quickFixAction.IsEnabled() || w.changes.quickFix.IsEnabled() || len(w.failures) != 0 {
			t.Errorf("enabled with nothing to fold, failures %q", w.failures)
		}
	})
}

// The question says when what is rewritten has been pushed: the last commit for
// an amend, and the one before it too for a fold.
func TestQuickFixQuestion(t *testing.T) {
	amend := git.QuickFix{Amend: true, Head: "h", Last: "last"}
	fold := git.QuickFix{Head: "h", Last: "last", Previous: "previous"}
	for _, c := range []struct {
		q      git.QuickFix
		s      git.Status
		pushed bool
	}{
		{amend, git.Status{}, false},
		{amend, git.Status{Upstream: "origin/main", Ahead: 0}, true},
		{amend, git.Status{Upstream: "origin/main", Ahead: 1}, false},
		{fold, git.Status{Upstream: "origin/main", Ahead: 1}, true},
		{fold, git.Status{Upstream: "origin/main", Ahead: 2}, false},
	} {
		text := quickFixQuestion(c.q, c.s)
		if strings.Contains(text, "force push") != c.pushed || !strings.Contains(text, "rewrites") {
			t.Errorf("%+v, %+v: %q", c.q, c.s, text)
		}
	}
}

// There is no quick fix during a merge.
func TestNoQuickFixDuringMerge(t *testing.T) {
	dir := newRepo(t)
	runGit(t, dir, "switch", "-q", "-c", "other")
	writeFile(t, dir, "file", "other\n")
	runGit(t, dir, "commit", "-q", "-am", "other")
	runGit(t, dir, "switch", "-q", "main")
	writeFile(t, dir, "file", "main\n")
	runGit(t, dir, "commit", "-q", "-am", "main")
	runGitOutFails(dir, "merge", "other")
	w := newTestWindow(t, dir)
	waitFor(t, "the merge", func() bool { return w.working == 0 && w.status.Merging })
	on(func() {
		if w.quickFixAction.IsEnabled() || w.changes.quickFix.IsEnabled() {
			t.Error("Quick Fix is enabled during a merge")
		}
	})
}
