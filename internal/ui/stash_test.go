package ui

import (
	"os/exec"
	"strings"
	"testing"
)

// runGitOut runs git in the folder and returns what it wrote.
func runGitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// stashCount returns how many stashes the repository has, by git.
func stashCount(t *testing.T, dir string) int {
	t.Helper()
	out := strings.TrimSpace(runGitOut(t, dir, "stash", "list"))
	if out == "" {
		return 0
	}
	return len(strings.Split(out, "\n"))
}

// The Stash menu stashes changes, untracked files too if asked, with the
// message given, and applies, pops and drops stashes; each is enabled only when
// there is something for it, and none during a merge or rebase.
func TestStash(t *testing.T) {
	dir := newRepo(t)
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })
	on(func() {
		if w.stashAction.IsEnabled() || w.applyStashAction.IsEnabled() || w.popStashAction.IsEnabled() || w.dropStashAction.IsEnabled() {
			t.Error("stashing is enabled with nothing to stash and no stashes")
		}
		if key := w.stashAction.Shortcut().ToString(); key != "Ctrl+Shift+Z" {
			t.Errorf("Stash Changes is on %q", key)
		}
	})
	writeFile(t, dir, "file", "changed\n")
	writeFile(t, dir, "loose", "untracked\n")
	on(w.rescanRepository)
	waitFor(t, "the changes", func() bool { return w.idleWith(2, 0) })
	on(func() {
		if !w.stashAction.IsEnabled() || w.applyStashAction.IsEnabled() {
			t.Error("Stash Changes is disabled, or Apply enabled with no stashes")
		}
		// The dialog asks for a message, which may be empty, and whether to
		// include untracked files, unticked.
		d, message, untracked := w.newStashDialog()
		defer d.Delete()
		if !d.ok.IsEnabled() || message.Text() != "" || untracked.IsChecked() {
			t.Error("the dialog doesn't start empty and unticked, with OK enabled")
		}
		w.stashChanges("my work", false)
	})
	// The untracked file stays.
	waitFor(t, "the stash", func() bool { return w.idleWith(1, 0) && w.stashes == 1 })
	on(func() {
		if !w.applyStashAction.IsEnabled() || !w.popStashAction.IsEnabled() || !w.dropStashAction.IsEnabled() {
			t.Error("using a stash is disabled with one")
		}
		w.stashChanges("", true)
	})
	waitFor(t, "the second stash", func() bool { return w.idleWith(0, 0) && w.stashes == 2 })
	if got := runGitOut(t, dir, "stash", "list"); !strings.Contains(got, "On main: my work") {
		t.Errorf("stashes %q", got)
	}

	// Applying keeps it, popping drops it, and dropping asks first.
	on(func() { w.applyStash("stash@{1}", false, true) })
	waitFor(t, "the apply", func() bool { return w.idleWith(1, 0) && w.stashes == 2 })
	runGit(t, dir, "checkout", "--", "file")
	on(func() { w.applyStash("stash@{0}", true, true) })
	waitFor(t, "the pop", func() bool { return w.idleWith(1, 0) && w.stashes == 1 && w.status.Unstaged[0].Path == "loose" })
	on(func() {
		w.answer = false
		w.dropStash("stash@{0}")
		if len(w.asked) != 1 || !strings.Contains(w.asked[0], "stash@{0}") || w.busy {
			t.Errorf("asked %q, busy %v", w.asked, w.busy)
		}
		w.answer = true
		w.dropStash("stash@{0}")
	})
	waitFor(t, "the drop", func() bool { return w.working == 0 && w.stashes == 0 })
	if n := stashCount(t, dir); n != 0 || len(w.failures) != 0 {
		t.Errorf("%d stashes left, failures %q", n, w.failures)
	}
}

// A stash that clashes is applied with conflicts, which the status bar says,
// with no error, and kept even by a pop; one whose staged changes can't be put
// back is offered with everything unstaged; one refused tells why.
func TestStashThatDoesNotApply(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "file", "stashed\n")
	runGit(t, dir, "stash", "push", "-m", "mine")
	writeFile(t, dir, "file", "committed\n")
	runGit(t, dir, "commit", "-q", "-am", "two")
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) && w.stashes == 1 })
	on(func() { w.applyStash("stash@{0}", true, true) })
	waitFor(t, "the conflict", func() bool { return w.idleWith(1, 0) && w.working == 0 })
	on(func() {
		if w.said.Text() != stashConflictText || len(w.failures) != 0 || w.stashes != 1 {
			t.Errorf("said %q, failures %q, %d stashes", w.said.Text(), w.failures, w.stashes)
		}
	})

	// The index can't be put back: asked, and on yes applied unstaged.
	runGit(t, dir, "reset", "-q", "--hard")
	runGit(t, dir, "stash", "clear")
	writeFile(t, dir, "file", "staged\n")
	runGit(t, dir, "add", "file")
	runGit(t, dir, "stash", "push")
	writeFile(t, dir, "file", "other\n")
	runGit(t, dir, "commit", "-q", "-am", "three")
	on(func() {
		w.asked = nil
		w.answer = false
		w.applyStash("stash@{0}", false, true)
	})
	waitFor(t, "the question", func() bool { return w.working == 0 && !w.busy && len(w.asked) == 1 })
	on(func() {
		if !strings.Contains(w.asked[0], "everything unstaged") || len(w.status.Unstaged)+len(w.status.Staged) != 0 {
			t.Errorf("asked %q, status %+v", w.asked, w.status)
		}
		w.answer = true
		w.applyStash("stash@{0}", false, true)
	})
	waitFor(t, "the unstaged apply", func() bool {
		return w.working == 0 && !w.busy && len(w.asked) == 2 && w.said.Text() == stashConflictText
	})

	// Refused, as what is in the way would be overwritten: git says why.
	runGit(t, dir, "reset", "-q", "--hard")
	writeFile(t, dir, "file", "in the way\n")
	on(w.rescanRepository)
	waitFor(t, "the change", func() bool { return w.idleWith(1, 0) })
	on(func() { w.applyStash("stash@{0}", false, false) })
	waitFor(t, "the refusal", func() bool { return w.working == 0 && !w.busy && len(w.failures) == 1 })
	if !strings.Contains(w.failures[0], "would be overwritten") {
		t.Errorf("failure %q", w.failures[0])
	}
}

// Stashing is disabled during a merge.
func TestNoStashDuringMerge(t *testing.T) {
	dir := newRepo(t)
	runGit(t, dir, "switch", "-q", "-c", "other")
	writeFile(t, dir, "file", "other\n")
	runGit(t, dir, "commit", "-q", "-am", "other")
	runGit(t, dir, "switch", "-q", "main")
	writeFile(t, dir, "file", "main\n")
	runGit(t, dir, "commit", "-q", "-am", "main")
	// The merge stops with a conflict, as it is meant to.
	exec.Command("git", "-C", dir, "merge", "other").Run()
	w := newTestWindow(t, dir)
	waitFor(t, "the merge", func() bool { return w.working == 0 && w.status.Merging })
	on(func() {
		if w.stashAction.IsEnabled() {
			t.Error("Stash Changes is enabled during a merge")
		}
	})
}
