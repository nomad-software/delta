package ui

import (
	"strings"
	"testing"
)

// finishRepo returns a repository on the branch topic, pushed and merged into
// main upstream, as finishing it expects.
func finishRepo(t *testing.T) string {
	t.Helper()
	dir := newRepo(t)
	bare := t.TempDir()
	runGit(t, bare, "init", "-q", "--bare", "-b", "main")
	runGit(t, dir, "remote", "add", "origin", bare)
	runGit(t, dir, "push", "-q", "-u", "origin", "main")
	runGit(t, dir, "switch", "-q", "-c", "topic")
	writeFile(t, dir, "work", "done\n")
	runGit(t, dir, "add", "work")
	runGit(t, dir, "commit", "-q", "-m", "work")
	runGit(t, dir, "push", "-q", "origin", "topic")
	other := t.TempDir()
	runGit(t, other, "clone", "-q", bare, ".")
	runGit(t, other, "merge", "-q", "--ff-only", "origin/topic")
	runGit(t, other, "push", "-q", "origin", "main")
	return dir
}

// Finish Branch, after Checkout in the Branch menu and the toolbar, asks first,
// then checks out main, pulls, and deletes the branch it was on.
func TestFinishBranch(t *testing.T) {
	dir := finishRepo(t)
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) && w.status.Branch == "topic" && w.finishAction.IsEnabled() })
	on(func() {
		actions := w.toolbar.Actions()
		for i, a := range actions {
			if a.UnsafePointer() == w.checkoutAction.UnsafePointer() && (i+1 == len(actions) || actions[i+1].UnsafePointer() != w.finishAction.UnsafePointer()) {
				t.Error("Finish is not after Checkout in the toolbar")
			}
		}
		if w.finishAction.Text() != "&Finish Branch…" || w.finishAction.IconText() != "Finish" || w.finishAction.Icon().IsNull() {
			t.Errorf("%q, %q", w.finishAction.Text(), w.finishAction.IconText())
		}
		w.answer = false
		w.finishAction.Trigger()
		if len(w.asked) != 1 || !strings.Contains(w.asked[0], "Check out main, pull into it, and delete the branch topic?") || w.busy {
			t.Errorf("asked %q, busy %v", w.asked, w.busy)
		}
		w.answer = true
		w.finishAction.Trigger()
	})
	waitFor(t, "the finish", func() bool { return w.idleWith(0, 0) && w.status.Branch == "main" && w.working == 0 })
	if got := runGitOut(t, dir, "branch", "--format=%(refname:short)"); got != "main\n" || len(w.failures) != 0 {
		t.Errorf("branches %q, failures %q", got, w.failures)
	}
	if got := strings.TrimSpace(runGitOut(t, dir, "log", "-1", "--format=%s")); got != "work" {
		t.Errorf("main is at %q, not pulled", got)
	}
	// On main there is nothing to finish.
	on(func() {
		if w.finishAction.IsEnabled() {
			t.Error("Finish Branch is enabled on main")
		}
	})
}

// A branch not merged is kept, on main, and deleted only if asked again; with
// changes not committed nothing is done.
func TestFinishBranchKept(t *testing.T) {
	dir := finishRepo(t)
	runGit(t, dir, "commit", "-q", "--allow-empty", "-m", "not merged")
	writeFile(t, dir, "file", "changed\n")
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.idleWith(1, 0) && w.finishAction.IsEnabled() })
	on(func() {
		w.finishAction.Trigger()
		if len(w.failures) != 1 || len(w.asked) != 0 || !strings.Contains(w.failures[0], "Commit or stash") {
			t.Errorf("failures %q, asked %q", w.failures, w.asked)
		}
	})
	runGit(t, dir, "checkout", "--", "file")
	on(w.rescanRepository)
	waitFor(t, "the rescan", func() bool { return w.idleWith(0, 0) })
	on(w.finishAction.Trigger)
	// Asked to finish, then whether to delete it anyway, answered yes.
	waitFor(t, "the forced delete", func() bool { return w.working == 0 && !w.busy && len(w.asked) == 2 && w.status.Branch == "main" })
	if !strings.Contains(w.asked[1], "Delete it anyway?") {
		t.Errorf("asked %q", w.asked[1])
	}
	waitFor(t, "the branch gone", func() bool { return runGitOut(t, dir, "branch", "--format=%(refname:short)") == "main\n" })
}
