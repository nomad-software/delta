package ui

import (
	"strings"
	"testing"

	qt "github.com/mappu/miqt/qt6"

	"github.com/nomad-software/delta/internal/git"
)

// Quick Fix, in the Commit menu on Ctrl+Shift+Q, with no button beside the
// message, folds every change not committed into the last commit, or with none the last
// commit into the one before it, asking first every time.
func TestQuickFix(t *testing.T) {
	dir := newRepo(t)
	runGit(t, dir, "commit", "-q", "--allow-empty", "-m", "second")
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) && w.quickFix.Head != "" })
	on(func() {
		if key := w.quickFixAction.Shortcut().ToString(); key != "Ctrl+Shift+Q" {
			t.Errorf("Quick Fix is on %q", key)
		}
		if !hasButton(w.changes.page.QObject, "Commit") || hasButton(w.changes.page.QObject, "Quick Fix") {
			t.Error("there is a Quick Fix button beside the message")
		}
		if !w.quickFixAction.IsEnabled() {
			t.Error("Quick Fix is disabled with a commit to fold")
		}
		// Answered no, nothing happens.
		w.answer = false
		w.quickFixAction.Trigger()
		if len(w.asked) != 1 || !strings.Contains(w.asked[0], "“second”, into the one before it, “first”") || w.busy {
			t.Errorf("asked %q, busy %v", w.asked, w.busy)
		}
		w.answer = true
		w.quickFixAction.Trigger()
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
		if w.quickFixAction.IsEnabled() || len(w.failures) != 0 {
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

// There is no quick fix or uncommit during a merge.
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
		if w.quickFixAction.IsEnabled() || w.uncommit.IsEnabled() {
			t.Error("Quick Fix or Uncommit is enabled during a merge")
		}
	})
}

// Uncommit, in the Commit menu after Commit on Ctrl+Shift+Return, then a
// separator and Quick Fix, takes the last commit back, its changes unstaged as if new; it is off with no commits, for a
// merge, and during one.
func TestUncommit(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "file", "second\n")
	runGit(t, dir, "commit", "-q", "-am", "second")
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) && w.canUncommit })
	on(func() {
		if key := w.uncommit.Shortcut().ToString(); key != "Ctrl+Shift+Return" || w.uncommit.Text() != "U&ncommit…" || !w.uncommit.IsEnabled() {
			t.Errorf("Uncommit on %q, %q, enabled %v", key, w.uncommit.Text(), w.uncommit.IsEnabled())
		}
		var items []string
		for _, a := range w.commit.AssociatedObjects() {
			if a.Inherits("QMenu") {
				for _, item := range qt.UnsafeNewQMenu(a.UnsafePointer()).Actions() {
					items = append(items, item.Text())
				}
			}
		}
		if want := "&Commit U&ncommit…  &Quick Fix…"; !strings.HasSuffix(strings.Join(items, " "), want) {
			t.Errorf("the Commit menu ends %q", items)
		}
		// It asks first: answered no, nothing happens.
		w.answer = false
		w.uncommit.Trigger()
		if len(w.asked) != 1 || !strings.Contains(w.asked[0], "“second”") || w.busy {
			t.Errorf("asked %q, busy %v", w.asked, w.busy)
		}
		w.answer = true
		w.uncommit.Trigger()
	})
	waitFor(t, "the uncommit", func() bool { return w.idleWith(1, 0) && w.working == 0 })
	if got := strings.TrimSpace(runGitOut(t, dir, "log", "--format=%s")); got != "first" || len(w.failures) != 0 {
		t.Errorf("commits %q, failures %q", got, w.failures)
	}
	on(func() {
		if w.status.Unstaged[0].Path != "file" || w.status.Unstaged[0].Code != 'M' {
			t.Errorf("unstaged %+v", w.status.Unstaged)
		}
	})

	// The first commit too: none are left, and Uncommit is off.
	runGit(t, dir, "checkout", "--", "file")
	on(w.rescanRepository)
	waitFor(t, "the rescan", func() bool { return w.idleWith(0, 0) })
	on(w.uncommit.Trigger)
	waitFor(t, "the last uncommit", func() bool { return w.idleWith(1, 0) && w.working == 0 })
	on(func() {
		if w.uncommit.IsEnabled() || w.status.Unstaged[0].Code != git.Untracked {
			t.Errorf("enabled %v, %+v", w.uncommit.IsEnabled(), w.status.Unstaged)
		}
	})
}

// The question before uncommitting says when the commit has been pushed.
func TestUncommitQuestion(t *testing.T) {
	for _, c := range []struct {
		s      git.Status
		pushed bool
	}{
		{git.Status{}, false},
		{git.Status{Upstream: "origin/main", Ahead: 0}, true},
		{git.Status{Upstream: "origin/main", Ahead: 1}, false},
	} {
		text := uncommitQuestion("last", c.s)
		if strings.Contains(text, "force push") != c.pushed || !strings.Contains(text, "“last”") {
			t.Errorf("%+v: %q", c.s, text)
		}
	}
}

// hasButton says whether a push button with the text is in the object, at any
// depth.
func hasButton(object *qt.QObject, text string) bool {
	for _, child := range object.Children() {
		if child.Inherits("QPushButton") && qt.UnsafeNewQPushButton(child.UnsafePointer()).Text() == text {
			return true
		}
		if hasButton(child, text) {
			return true
		}
	}
	return false
}
