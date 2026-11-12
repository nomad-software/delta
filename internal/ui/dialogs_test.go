package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nomad-software/qute/qtcore"

	"github.com/nomad-software/delta/internal/git"
	"github.com/nomad-software/delta/internal/settings"
	"github.com/nomad-software/qute"
	"github.com/nomad-software/qute/qtwidgets"
)

// answer starts the action on the main thread without waiting for it, as it
// waits on a dialog, and answers that dialog with reply once it is open.
func answer(t *testing.T, action func(), reply func(d *qtwidgets.QDialog)) {
	t.Helper()
	qute.Post(action)
	answerNext(t, reply)
}

// answerNext waits for a dialog to be open and waiting for an answer, as one
// opened from what another answered is, and answers it with reply, on the main
// thread.
func answerNext(t *testing.T, reply func(d *qtwidgets.QDialog)) {
	t.Helper()
	var d *qtwidgets.QDialog
	waitFor(t, "a dialog", func() bool {
		modal := qtwidgets.QApplicationActiveModalWidget()
		if modal == nil || !modal.IsVisible() || !modal.Inherits("QDialog") {
			return false
		}
		d = qtwidgets.QDialogCast(modal)
		return true
	})
	on(func() { reply(d) })
}

// children returns what the object holds of the class, at any depth, in the
// order they were made.
func children[T any](object *qtcore.QObject, class string, wrap func(qtcore.QObjectLike) T) []T {
	var found []T
	for _, child := range object.Children() {
		if child.Inherits(class) {
			found = append(found, wrap(child))
		}
		found = append(found, children(child, class, wrap)...)
	}
	return found
}

// texts returns the dialog's fields of text, and combos its lists to choose from.
func texts(d *qtwidgets.QDialog) []*qtwidgets.QLineEdit {
	return children(d.AsQObject(), "QLineEdit", qtwidgets.QLineEditCast)
}

func combos(d *qtwidgets.QDialog) []*qtwidgets.QComboBox {
	return children(d.AsQObject(), "QComboBox", qtwidgets.QComboBoxCast)
}

// choose chooses the item with the text in the combo, failing if there is none.
func choose(t *testing.T, combo *qtwidgets.QComboBox, text string) {
	t.Helper()
	at := combo.FindText(text)
	if at < 0 {
		t.Errorf("%q is not offered", text)
		return
	}
	combo.SetCurrentIndex(at)
}

// items returns the texts of the combo's items.
func items(combo *qtwidgets.QComboBox) []string {
	var all []string
	for i := range combo.Count() {
		all = append(all, combo.ItemText(i))
	}
	return all
}

// currentBranch returns the branch checked out in the folder, by git.
func currentBranch(t *testing.T, dir string) string {
	t.Helper()
	return strings.TrimSpace(runGitOut(t, dir, "branch", "--show-current"))
}

// hasBranch says whether the folder's repository has the local branch, by git.
func hasBranch(t *testing.T, dir, name string) bool {
	t.Helper()
	return slices.Contains(strings.Fields(runGitOut(t, dir, "branch", "--format=%(refname:short)")), name)
}

// idle says whether the window has nothing going on in the background.
func (w *Window) idle() bool {
	return !w.busy && !w.reading && w.working == 0
}

// The Branch menu's New Branch, Checkout, Rename, Merge and Delete ask what to
// do in their dialogs, once the branches are read, and do it.
func TestBranchDialogs(t *testing.T) {
	dir := newRepo(t)
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })

	// The name is made fit for a branch, and the branch checked out.
	answer(t, w.newBranch, func(d *qtwidgets.QDialog) {
		texts(d)[0].SetText("my feature")
		d.Accept()
	})
	waitFor(t, "the new branch", func() bool { return w.idle() && w.status.Branch == "my-feature" })
	if got := currentBranch(t, dir); got != "my-feature" {
		t.Errorf("on %q", got)
	}

	// The current branch is offered first, and chosen.
	answer(t, w.renameBranch, func(d *qtwidgets.QDialog) {
		if got := items(combos(d)[0]); !slices.Equal(got, []string{"my-feature", "main"}) {
			t.Errorf("offered %v", got)
		}
		texts(d)[0].SetText("renamed")
		d.Accept()
	})
	waitFor(t, "the rename", func() bool { return w.idle() && w.status.Branch == "renamed" })
	if hasBranch(t, dir, "my-feature") {
		t.Error("the old name is still there")
	}

	answer(t, w.checkoutBranch, func(d *qtwidgets.QDialog) {
		choose(t, combos(d)[0], "main")
		d.Accept()
	})
	waitFor(t, "the checkout", func() bool { return w.idle() && w.status.Branch == "main" })

	// Cancelled, nothing is renamed.
	answer(t, w.renameBranch, func(d *qtwidgets.QDialog) {
		texts(d)[0].SetText("unwanted")
		d.Reject()
	})
	waitFor(t, "the dialog to go", func() bool { return w.idle() })
	if hasBranch(t, dir, "unwanted") {
		t.Error("a cancelled rename was done")
	}

	// A branch with a commit of its own is merged in.
	runGit(t, dir, "switch", "-q", "renamed")
	writeFile(t, dir, "merged", "in\n")
	runGit(t, dir, "add", "merged")
	runGit(t, dir, "commit", "-q", "-m", "on renamed")
	runGit(t, dir, "switch", "-q", "main")
	on(w.rescanRepository)
	waitFor(t, "the rescan", func() bool { return w.idle() && w.status.Branch == "main" })
	answer(t, w.mergeBranch, func(d *qtwidgets.QDialog) {
		// The current branch is not offered.
		if got := items(combos(d)[0]); slices.Contains(got, "main") {
			t.Errorf("offered %v", got)
		}
		choose(t, combos(d)[0], "renamed")
		d.Accept()
	})
	waitFor(t, "the merge", func() bool { return w.idle() })
	if _, err := os.Stat(dir + "/merged"); err != nil {
		t.Errorf("not merged: %v", err)
	}

	// A merged branch is deleted once asked.
	answer(t, w.deleteBranch, func(d *qtwidgets.QDialog) {
		choose(t, combos(d)[0], "renamed")
		d.Accept()
	})
	waitFor(t, "the deletion", func() bool { return w.idle() && !slices.Contains(w.localBranches, "renamed") })

	// One that is not merged is deleted only once asked again.
	runGit(t, dir, "switch", "-q", "-c", "side")
	writeFile(t, dir, "side", "unmerged\n")
	runGit(t, dir, "add", "side")
	runGit(t, dir, "commit", "-q", "-m", "on side")
	runGit(t, dir, "switch", "-q", "main")
	on(w.rescanRepository)
	waitFor(t, "the rescan", func() bool { return w.idle() && slices.Contains(w.localBranches, "side") })
	on(func() { w.asked = nil })
	answer(t, w.deleteBranch, func(d *qtwidgets.QDialog) {
		choose(t, combos(d)[0], "side")
		d.Accept()
	})
	waitFor(t, "the forced deletion", func() bool { return w.idle() && !slices.Contains(w.localBranches, "side") })
	on(func() {
		if len(w.asked) != 2 || !strings.Contains(w.asked[1], "Delete it anyway?") {
			t.Errorf("asked %q", w.asked)
		}
	})

	// With no other branch, there is nothing to delete or merge.
	on(func() { w.failures = nil })
	on(w.deleteBranch)
	waitFor(t, "the branches", func() bool { return w.idle() && len(w.failures) == 1 })
	on(w.mergeBranch)
	waitFor(t, "the branches", func() bool { return w.idle() && len(w.failures) == 2 })
	on(func() {
		if !slices.Equal(w.failures, []string{"There are no other branches.", "There are no other branches."}) {
			t.Errorf("failures %q", w.failures)
		}
	})
}

// The Remote menu adds, renames, changes the URL of and removes remotes, asking
// which in its dialogs; a branch with no upstream is pushed to the remote
// chosen, which becomes its upstream.
func TestRemoteDialogs(t *testing.T) {
	dir := newRepo(t)
	first, second := t.TempDir(), t.TempDir()
	runGit(t, first, "init", "-q", "--bare", "-b", "main")
	runGit(t, second, "init", "-q", "--bare", "-b", "main")
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })

	// With no remotes, there is nothing to choose.
	on(w.renameRemote)
	waitFor(t, "the remotes", func() bool { return w.idle() && len(w.failures) == 1 })
	on(func() {
		if w.failures[0] != "This repository has no remotes." {
			t.Errorf("failures %q", w.failures)
		}
		w.failures = nil
	})

	// The name starts as origin.
	answer(t, w.addRemote, func(d *qtwidgets.QDialog) {
		fields := texts(d)
		if fields[0].Text() != "origin" {
			t.Errorf("name %q", fields[0].Text())
		}
		fields[1].SetText(first)
		d.Accept()
	})
	waitFor(t, "the remote", func() bool { return w.idle() })
	if got := strings.TrimSpace(runGitOut(t, dir, "remote")); got != "origin" {
		t.Fatalf("remotes %q", got)
	}

	answer(t, w.renameRemote, func(d *qtwidgets.QDialog) { d.Accept() })
	answerNext(t, func(d *qtwidgets.QDialog) {
		texts(d)[0].SetText("upstream")
		d.Accept()
	})
	waitFor(t, "the rename", func() bool { return w.idle() })
	if got := strings.TrimSpace(runGitOut(t, dir, "remote")); got != "upstream" {
		t.Fatalf("remotes %q", got)
	}

	// The URL starts as it is.
	answer(t, w.changeRemoteURL, func(d *qtwidgets.QDialog) { d.Accept() })
	answerNext(t, func(d *qtwidgets.QDialog) {
		if got := texts(d)[0].Text(); got != first {
			t.Errorf("URL %q", got)
		}
		texts(d)[0].SetText(second)
		d.Accept()
	})
	waitFor(t, "the URL", func() bool { return w.idle() })
	if got := strings.TrimSpace(runGitOut(t, dir, "remote", "get-url", "upstream")); got != second {
		t.Errorf("URL %q", got)
	}

	// With two remotes, a push asks which, and that becomes the upstream.
	runGit(t, dir, "remote", "add", "other", first)
	answer(t, w.push.Trigger, func(d *qtwidgets.QDialog) {
		if got := items(combos(d)[0]); !slices.Equal(got, []string{"other", "upstream"}) {
			t.Errorf("offered %v", got)
		}
		choose(t, combos(d)[0], "other")
		d.Accept()
	})
	waitFor(t, "the push", func() bool { return w.idle() && w.status.Upstream == "other/main" })
	on(func() {
		if len(w.failures) != 0 {
			t.Errorf("failures %q", w.failures)
		}
	})

	answer(t, w.removeRemote, func(d *qtwidgets.QDialog) {
		choose(t, combos(d)[0], "upstream")
		d.Accept()
	})
	waitFor(t, "the removal", func() bool { return w.idle() })
	if got := strings.TrimSpace(runGitOut(t, dir, "remote")); got != "other" {
		t.Errorf("remotes %q", got)
	}
}

// With no remote at all, a branch with no upstream has nowhere to go.
func TestPushWithNoRemotes(t *testing.T) {
	w := newTestWindow(t, newRepo(t))
	// Until the status is read, there is no branch to push.
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) && w.status.Branch == "main" })
	on(w.push.Trigger)
	waitFor(t, "the remotes", func() bool { return w.idle() && len(w.failures) == 1 })
	on(func() {
		if !strings.Contains(w.failures[0], "no remotes") {
			t.Errorf("failures %q", w.failures)
		}
	})
}

// The Stash menu asks for a stash's message, and which stash to apply, pop or
// drop, from a list of them.
func TestStashDialogs(t *testing.T) {
	dir := newRepo(t)
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })

	// With no stashes, there is nothing to choose.
	on(w.askDropStash)
	waitFor(t, "the stashes", func() bool { return w.idle() && len(w.failures) == 1 })
	on(func() {
		if w.failures[0] != "There are no stashes." {
			t.Errorf("failures %q", w.failures)
		}
		w.failures = nil
	})

	writeFile(t, dir, "file", "changed\n")
	on(w.rescanRepository)
	waitFor(t, "the change", func() bool { return w.idleWith(1, 0) })
	answer(t, w.askStashChanges, func(d *qtwidgets.QDialog) {
		texts(d)[0].SetText("  saved  ")
		d.Accept()
	})
	waitFor(t, "the stash", func() bool { return w.idleWith(0, 0) && w.stashes == 1 })
	if got := runGitOut(t, dir, "stash", "list"); !strings.Contains(got, "On main: saved\n") {
		t.Errorf("stashes %q", got)
	}

	// Applied, the stash is kept; the list says what each is.
	answer(t, func() { w.askApplyStash(false) }, func(d *qtwidgets.QDialog) {
		if d.WindowTitle() != "Apply Stash" {
			t.Errorf("titled %q", d.WindowTitle())
		}
		if got := items(combos(d)[0]); !slices.Equal(got, []string{"stash@{0}: On main: saved"}) {
			t.Errorf("offered %v", got)
		}
		d.Accept()
	})
	waitFor(t, "the apply", func() bool { return w.idleWith(1, 0) })
	if stashCount(t, dir) != 1 {
		t.Error("applying dropped the stash")
	}

	// Popped, it goes.
	runGit(t, dir, "checkout", "--", "file")
	answer(t, func() { w.askApplyStash(true) }, func(d *qtwidgets.QDialog) {
		if d.WindowTitle() != "Pop Stash" {
			t.Errorf("titled %q", d.WindowTitle())
		}
		d.Accept()
	})
	waitFor(t, "the pop", func() bool { return w.idleWith(1, 0) && w.stashes == 0 })

	// Dropped, once asked, it goes and its changes with it.
	runGit(t, dir, "stash", "-q")
	on(w.rescanRepository)
	waitFor(t, "the stash", func() bool { return w.idleWith(0, 0) && w.stashes == 1 })
	answer(t, w.askDropStash, func(d *qtwidgets.QDialog) { d.Accept() })
	waitFor(t, "the drop", func() bool { return w.idleWith(0, 0) && w.stashes == 0 })
	on(func() {
		if len(w.asked) == 0 || !strings.HasPrefix(w.asked[len(w.asked)-1], "Drop stash@{0}?") {
			t.Errorf("asked %q", w.asked)
		}
		if len(w.failures) != 0 {
			t.Errorf("failures %q", w.failures)
		}
	})
}

// The rebase dialog's size is saved whenever it closes, cancelled or not, and a
// cancelled one rewrites nothing.
func TestRebaseDialogSizeSaved(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "file", "second\n")
	runGit(t, dir, "commit", "-q", "-a", "-m", "second")
	head := strings.TrimSpace(runGitOut(t, dir, "rev-parse", "HEAD"))
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })
	r, err := git.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	commits, err := r.Log(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	answer(t, func() { w.rebaseFrom(commits[0]) }, func(d *qtwidgets.QDialog) {
		d.Resize(700, 500)
		d.Reject()
	})
	waitFor(t, "the dialog to go", func() bool { return w.idle() })
	if got := strings.TrimSpace(runGitOut(t, dir, "rev-parse", "HEAD")); got != head {
		t.Error("a cancelled rebase rewrote the branch")
	}
	if s := settings.Load(w.path); s.RebaseWidth != 700 || s.RebaseHeight != 500 {
		t.Errorf("saved %d×%d", s.RebaseWidth, s.RebaseHeight)
	}
}

// A question has No as the answer to return, and Yes is yes.
func TestAskYesNo(t *testing.T) {
	for _, yes := range []bool{false, true} {
		var got bool
		answer(t, func() { got = askYesNo(nil, "Title", "Really?") }, func(d *qtwidgets.QDialog) {
			box := qtwidgets.QMessageBoxCast(d)
			if box.StandardButton(box.DefaultButton().AsQAbstractButton()) != qtwidgets.QMessageBoxNo {
				t.Error("No is not the default")
			}
			button := qtwidgets.QMessageBoxNo
			if yes {
				button = qtwidgets.QMessageBoxYes
			}
			box.Button(button).Click()
		})
		waitFor(t, "the answer", func() bool { return qtwidgets.QApplicationActiveModalWidget() == nil })
		on(func() {
			if got != yes {
				t.Errorf("answered %v, got %v", yes, got)
			}
		})
	}
}

// An error shows only the last lines of a long one, and goes once dismissed.
func TestShowErrorKeepsTheLastLines(t *testing.T) {
	on(func() {
		parent := qtwidgets.NewQWidget(nil)
		defer parent.Delete()
		var lines []string
		for i := range maxErrorLines + 10 {
			lines = append(lines, fmt.Sprintf("line %d", i))
		}
		showError(parent, "Failed", strings.Join(lines, "\n"))
		boxes := children(parent.AsQObject(), "QMessageBox", qtwidgets.QMessageBoxCast)
		if len(boxes) != 1 {
			t.Fatalf("%d boxes", len(boxes))
		}
		shown := strings.Split(boxes[0].Text(), "\n")
		if len(shown) != maxErrorLines+1 || shown[0] != "…" || shown[1] != "line 10" || shown[len(shown)-1] != lines[len(lines)-1] {
			t.Errorf("shown %q", shown)
		}
		boxes[0].Done(0)
	})
}

// A change is said in words, and named with where it came from.
func TestDescribeAndExplain(t *testing.T) {
	for _, test := range []struct {
		change          git.Change
		text, explained string
	}{
		{git.Change{Path: "a", Code: 'M'}, "a", "Modified"},
		{git.Change{Path: "a", Code: 'A'}, "a", "Added"},
		{git.Change{Path: "a", Code: 'D'}, "a", "Deleted"},
		{git.Change{Path: "b", OldPath: "a", Code: 'R'}, "a → b", "Renamed"},
		{git.Change{Path: "b", OldPath: "a", Code: 'C'}, "a → b", "Copied"},
		{git.Change{Path: "a", Code: 'T'}, "a", "Type changed"},
		{git.Change{Path: "a", Code: git.Untracked}, "a", "Untracked"},
		{git.Change{Path: "a", Code: git.Conflict}, "a", "In conflict: stage it once it is resolved"},
		{git.Change{Path: "a", Code: 'X'}, "a", ""},
	} {
		if got := describe(test.change); got != test.text {
			t.Errorf("describe(%+v) = %q", test.change, got)
		}
		if got := explain(test.change); got != test.explained {
			t.Errorf("explain(%+v) = %q", test.change, got)
		}
	}
	for in, want := range map[string]string{"": "", "pick": "Pick", "Done": "Done"} {
		if got := upperFirst(in); got != want {
			t.Errorf("upperFirst(%q) = %q", in, got)
		}
	}
}

// File > Open asks for a folder, starting beside the repository open, and opens
// the repository it is in.
func TestOpenFromTheFileMenu(t *testing.T) {
	first, second := newRepo(t), newRepo(t)
	w := newTestWindow(t, first)
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })
	answer(t, w.chooseRepository, func(d *qtwidgets.QDialog) {
		files := qtwidgets.QFileDialogCast(d)
		if got := files.Directory().AbsolutePath(); got != filepath.Dir(first) {
			t.Errorf("starts in %q", got)
		}
		// In a dialog of folders, the folder it is in is the one chosen.
		files.SetDirectory(second)
		d.Accept()
	})
	waitFor(t, "the repository", func() bool { return w.idle() && w.root() == second })

	// Cancelled, the repository open stays.
	answer(t, w.chooseRepository, func(d *qtwidgets.QDialog) { d.Reject() })
	waitFor(t, "the dialog to go", func() bool { return w.idle() })
	on(func() {
		if w.root() != second {
			t.Errorf("open %q", w.root())
		}
	})
}

// File > Settings shows the settings, and puts those chosen into effect and
// saves them when accepted.
func TestShowSettings(t *testing.T) {
	w := newTestWindow(t, newRepo(t))
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })
	answer(t, w.showSettings, func(d *qtwidgets.QDialog) {
		spins := children(d.AsQObject(), "QSpinBox", qtwidgets.QSpinBoxCast)
		for _, spin := range spins {
			if spin.Suffix() == " commits" {
				spin.SetValue(1234)
			}
		}
		boxes := children(d.AsQObject(), "QDialogButtonBox", qtwidgets.QDialogButtonBoxCast)
		boxes[0].Button(qtwidgets.QDialogButtonBoxOk).Click()
	})
	on(func() {
		if w.history.pageSize != 1234 {
			t.Errorf("page size %d", w.history.pageSize)
		}
	})
	if s := settings.Load(w.path); s.HistoryPageSize != 1234 {
		t.Errorf("saved %d", s.HistoryPageSize)
	}
}

// The settings dialog does not stop the window opening a repository, as one
// started again with a folder makes it, so what it chose goes on top of the
// settings as they are when it is accepted, not as they were when it opened:
// the repository opened meanwhile stays remembered, and the last opened.
func TestSettingsKeepRepositoryOpenedMeanwhile(t *testing.T) {
	first, second := namedRepo(t, "first"), namedRepo(t, "second")
	w := newTestWindow(t, first)
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })
	answer(t, w.showSettings, func(d *qtwidgets.QDialog) {
		if err := w.openRepository(second); err != nil {
			t.Fatal(err)
		}
		boxes := children(d.AsQObject(), "QDialogButtonBox", qtwidgets.QDialogButtonBoxCast)
		boxes[0].Button(qtwidgets.QDialogButtonBoxOk).Click()
	})
	on(func() {
		if !slices.Equal(w.config.Repositories, []string{first, second}) || w.config.LastRepository != second {
			t.Errorf("remembered %q, last %q", w.config.Repositories, w.config.LastRepository)
		}
	})
	if s := settings.Load(w.path); !slices.Equal(s.Repositories, []string{first, second}) || s.LastRepository != second {
		t.Errorf("saved %q, last %q", s.Repositories, s.LastRepository)
	}
}
