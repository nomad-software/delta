package ui

import (
	"strings"
	"testing"

	"github.com/nomad-software/delta/internal/git"
	"github.com/nomad-software/qute/qtwidgets"
)

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
				for _, item := range qtwidgets.QMenuCast(a).Actions() {
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
