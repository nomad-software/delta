package ui

import (
	"fmt"
	"slices"
	"strings"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"

	"github.com/nomad-software/merge-monkey/internal/git"
)

// run runs a git command that changes something in the background, with the rest
// disabled, and then reads the status again. A failure is told of. after, if
// given, is called with the outcome.
func (w *Window) run(name string, op func() (string, error), after func(error)) {
	if w.busy || w.repo == nil {
		return
	}
	w.busy = true
	w.updateActions()
	w.say(name + "…")
	go func() {
		out, err := op()
		mainthread.Start(func() {
			w.busy = false
			if err != nil {
				w.say(name + " failed")
				w.failed(name+" Failed", err.Error())
			} else {
				w.say(strings.TrimSpace(name + " done. " + lastLine(out)))
			}
			if after != nil {
				after(err)
			}
			w.updateActions()
			w.rescanRepository()
		})
	}()
}

// firstLine returns the first line of the text.
func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return line
}

// lastLine returns the last line of the text.
func lastLine(text string) string {
	text = strings.TrimSpace(text)
	return text[strings.LastIndexByte(text, '\n')+1:]
}

// stageChanges adds the changes to the index.
func (w *Window) stageChanges(changes []git.Change) {
	r := w.repo
	changes = slices.Clone(changes)
	w.run("Stage", func() (string, error) { return "", r.Stage(changes) }, nil)
}

// unstageChanges takes the changes out of the index.
func (w *Window) unstageChanges(changes []git.Change) {
	r := w.repo
	changes = slices.Clone(changes)
	w.run("Unstage", func() (string, error) { return "", r.Unstage(changes) }, nil)
}

// commitChanges commits the index, and clears the message if that works.
func (w *Window) commitChanges() {
	r := w.repo
	message := w.tab.message.ToPlainText()
	if strings.TrimSpace(message) == "" {
		return
	}
	w.run("Commit", func() (string, error) { return r.Commit(message) }, func(err error) {
		if err == nil {
			w.tab.message.Clear()
		}
	})
}

// pushBranch pushes the current branch, with its upstream set if it has none.
func (w *Window) pushBranch(force bool) {
	r := w.repo
	if r == nil || w.busy {
		return
	}
	if w.status.Branch == "" {
		w.failed("Push Failed", "HEAD is detached, so there is no branch to push.")
		return
	}
	remote, ok := w.pushRemote()
	if !ok {
		return
	}
	name := "Push"
	if force {
		name = "Force Push"
		target := w.status.Upstream
		if target == "" {
			target = remote + "/" + w.status.Branch
		}
		if !w.confirm("Force Push", fmt.Sprintf("Overwrite %s with %s?\n\nCommits on the remote that are not in this branch will be lost. "+
			"Git refuses if the remote has commits that have not been fetched.", target, w.status.Branch)) {
			return
		}
	}
	w.run(name, func() (string, error) { return r.Push(remote, force) }, func(err error) {
		// Forcing is for one push at a time, not a state to be left in.
		if err == nil {
			w.tab.force.SetChecked(false)
		}
	})
}

// pushRemote returns the remote to push to and make the upstream, which is
// nothing if the branch has an upstream, asking which if there are several.
func (w *Window) pushRemote() (string, bool) {
	if w.status.Upstream != "" {
		return "", true
	}
	remotes, err := w.repo.Remotes()
	if err != nil {
		w.failed("Push Failed", err.Error())
		return "", false
	}
	if len(remotes) == 0 {
		w.failed("Push Failed", "This repository has no remotes. Add one from the Remote menu.")
		return "", false
	}
	if len(remotes) == 1 {
		return remotes[0], true
	}
	return askChoice(w.QWidget, "Push", "Push to", remotes)
}

// branches returns the local and remote branches, telling of any failure.
func (w *Window) branches() (local, remote []string, ok bool) {
	local, remote, err := w.repo.Branches()
	if err != nil {
		w.failed("Branches", err.Error())
		return nil, nil, false
	}
	return local, remote, true
}

// without returns the items other than the one named.
func without(items []string, name string) []string {
	return slices.DeleteFunc(slices.Clone(items), func(item string) bool { return item == name })
}

// newBranch asks for a new branch and makes it.
func (w *Window) newBranch() {
	local, remote, ok := w.branches()
	if !ok {
		return
	}
	r := w.repo
	d := newFormDialog(w.QWidget, "New Branch")
	name := d.addText("Name", "", r.ValidBranchName)
	start := d.addChoice("Start at", append(append([]string{"HEAD"}, local...), remote...))
	checkout := qt.NewQCheckBox3("Check out the new branch")
	checkout.SetChecked(true)
	d.form.AddRowWithWidget(checkout.QWidget)
	if !d.run() {
		return
	}
	from := start.CurrentText()
	if from == "HEAD" {
		from = ""
	}
	w.createBranch(strings.TrimSpace(name.Text()), from, checkout.IsChecked())
}

// createBranch makes a branch at the start point, or HEAD if it is empty.
func (w *Window) createBranch(name, start string, checkout bool) {
	r := w.repo
	w.run("New Branch", func() (string, error) { return r.CreateBranch(name, start, checkout) }, nil)
}

// checkoutBranch asks for a branch and switches to it. A remote branch is
// checked out as a new local branch that tracks it.
func (w *Window) checkoutBranch() {
	local, remote, ok := w.branches()
	if !ok {
		return
	}
	items := append(without(local, w.status.Branch), remote...)
	if len(items) == 0 {
		w.failed("Checkout", "There are no other branches.")
		return
	}
	if name, ok := askChoice(w.QWidget, "Checkout", "Branch", items); ok {
		w.checkout(name, !slices.Contains(local, name))
	}
}

// checkout switches to a branch.
func (w *Window) checkout(name string, isRemote bool) {
	r := w.repo
	w.run("Checkout", func() (string, error) {
		if isRemote {
			return r.CheckoutRemote(name)
		}
		return r.Checkout(name)
	}, nil)
}

// renameBranch asks for a branch, the current one first, and its new name.
func (w *Window) renameBranch() {
	local, _, ok := w.branches()
	if !ok {
		return
	}
	if len(local) == 0 {
		w.failed("Rename Branch", "There are no branches yet.")
		return
	}
	r := w.repo
	if slices.Contains(local, w.status.Branch) {
		local = append([]string{w.status.Branch}, without(local, w.status.Branch)...)
	}
	d := newFormDialog(w.QWidget, "Rename Branch")
	from := d.addChoice("Branch", local)
	to := d.addText("New name", "", r.ValidBranchName)
	if !d.run() {
		return
	}
	oldName, newName := from.CurrentText(), strings.TrimSpace(to.Text())
	w.run("Rename Branch", func() (string, error) { return r.RenameBranch(oldName, newName) }, nil)
}

// deleteBranch asks for a branch other than the current one and deletes it,
// asking again before deleting one that has not been merged.
func (w *Window) deleteBranch() {
	local, _, ok := w.branches()
	if !ok {
		return
	}
	items := without(local, w.status.Branch)
	if len(items) == 0 {
		w.failed("Delete Branch", "There are no other branches.")
		return
	}
	if name, ok := askChoice(w.QWidget, "Delete Branch", "Branch", items); ok {
		w.removeBranch(name)
	}
}

// removeBranch deletes a branch once that is confirmed.
func (w *Window) removeBranch(name string) {
	r := w.repo
	if !w.confirm("Delete Branch", "Delete the branch "+name+"?") {
		return
	}
	// This is quick and local, and what it says decides what to ask next.
	_, err := r.DeleteBranch(name, false)
	if err != nil {
		if !w.confirm("Delete Branch", err.Error()+"\n\nDelete it anyway? Its unmerged commits will be lost.") {
			return
		}
		w.run("Delete Branch", func() (string, error) { return r.DeleteBranch(name, true) }, nil)
		return
	}
	w.say("Deleted the branch " + name)
	w.rescanRepository()
}

// mergeBranch asks for a branch and merges it into the current one.
func (w *Window) mergeBranch() {
	local, remote, ok := w.branches()
	if !ok {
		return
	}
	items := append(without(local, w.status.Branch), remote...)
	if len(items) == 0 {
		w.failed("Merge", "There are no other branches.")
		return
	}
	if name, ok := askChoice(w.QWidget, "Merge", "Merge into the current branch", items); ok {
		r := w.repo
		w.run("Merge", func() (string, error) { return r.Merge(name) }, nil)
	}
}

// doAbortMerge abandons the merge in progress once that is confirmed.
func (w *Window) doAbortMerge() {
	r := w.repo
	if !w.confirm("Abort Merge", "Abandon the merge? Changes made to resolve it will be lost.") {
		return
	}
	w.run("Abort Merge", r.AbortMerge, func(err error) {
		if err == nil {
			w.tab.message.Clear()
		}
	})
}

// notEmpty says whether a name or URL has been given.
func notEmpty(text string) bool { return text != "" }

// addRemote asks for a remote and adds it.
func (w *Window) addRemote() {
	r := w.repo
	d := newFormDialog(w.QWidget, "Add Remote")
	name := d.addText("Name", "origin", notEmpty)
	url := d.addText("URL", "", notEmpty)
	if !d.run() {
		return
	}
	n, u := strings.TrimSpace(name.Text()), strings.TrimSpace(url.Text())
	w.run("Add Remote", func() (string, error) { return r.AddRemote(n, u) }, nil)
}

// chooseRemote asks which remote to act on.
func (w *Window) chooseRemote(title string) (string, bool) {
	remotes, err := w.repo.Remotes()
	if err != nil {
		w.failed(title, err.Error())
		return "", false
	}
	if len(remotes) == 0 {
		w.failed(title, "This repository has no remotes.")
		return "", false
	}
	return askChoice(w.QWidget, title, "Remote", remotes)
}

// renameRemote asks for a remote and its new name.
func (w *Window) renameRemote() {
	r := w.repo
	from, ok := w.chooseRemote("Rename Remote")
	if !ok {
		return
	}
	to, ok := askText(w.QWidget, "Rename Remote", "New name", from, notEmpty)
	if !ok || to == from {
		return
	}
	w.run("Rename Remote", func() (string, error) { return r.RenameRemote(from, to) }, nil)
}

// changeRemoteURL asks for a remote and its new URL.
func (w *Window) changeRemoteURL() {
	r := w.repo
	name, ok := w.chooseRemote("Change URL")
	if !ok {
		return
	}
	current, _ := r.RemoteURL(name)
	url, ok := askText(w.QWidget, "Change URL", "URL", current, notEmpty)
	if !ok || url == current {
		return
	}
	w.run("Change URL", func() (string, error) { return r.SetRemoteURL(name, url) }, nil)
}

// removeRemote asks for a remote and removes it once that is confirmed.
func (w *Window) removeRemote() {
	r := w.repo
	name, ok := w.chooseRemote("Remove Remote")
	if !ok || !w.confirm("Remove Remote", "Remove the remote "+name+" and its remote tracking branches?") {
		return
	}
	w.run("Remove Remote", func() (string, error) { return r.RemoveRemote(name) }, nil)
}
