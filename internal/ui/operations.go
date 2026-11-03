package ui

import (
	"fmt"
	"slices"
	"strings"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"

	"github.com/nomad-software/delta/internal/git"
)

// background runs a git command that changes something in the background, with the
// rest disabled, and then reads the status again. done is given the outcome.
func (w *Window) background(name string, op func() (string, error), done func(string, error)) {
	if w.busy || w.repo == nil {
		return
	}
	w.busy = true
	w.updateActions()
	w.say(name + "…")
	w.beginWork()
	go func() {
		out, err := op()
		mainthread.Start(func() {
			w.endWork()
			w.busy = false
			done(out, err)
			w.updateActions()
			w.rescanRepository()
		})
	}()
}

// run runs a command as background does, telling of a failure. after, if given,
// is called with the outcome.
func (w *Window) run(name string, op func() (string, error), after func(error)) {
	w.background(name, op, func(out string, err error) {
		if err != nil {
			w.say(name + " failed")
			w.failed(name+" Failed", err.Error())
		} else {
			w.say(strings.TrimSpace(name + " done. " + lastLine(out)))
		}
		if after != nil {
			after(err)
		}
	})
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
	if len(local)+len(remote) == 0 {
		w.failed("Checkout", "There are no branches yet.")
		return
	}
	r := w.repo
	fetched := false
	// It is called in the background, and the spinner shows it.
	fetch := func() ([]string, error) {
		mainthread.Wait(w.beginWork)
		defer mainthread.Start(w.endWork)
		fetched = true
		if _, err := r.Fetch(); err != nil {
			return nil, err
		}
		_, remote, err := r.Branches()
		return remote, err
	}
	d, branch, _ := newCheckoutDialog(w.QWidget, w.status.Branch, local, remote, fetch)
	if d.run() {
		name := branch.CurrentText()
		w.checkout(name, !slices.Contains(local, name))
	} else if fetched {
		// What was fetched changes how far the branch is from its upstream.
		w.rescanRepository()
	}
}

// newCheckoutDialog returns the dialog asking which branch to check out, its
// list of them, and its checkbox: the list has the local branches, with the
// current one chosen, and the remote ones after them while the checkbox is
// ticked, each in order of name.
// OK is enabled only while a branch other than the current one is chosen, as
// there is nothing to do otherwise. Ticking it the first time fetches, in the
// background, and lists the remote branches that gives, or, if it fails, says
// why and lists those already known.
func newCheckoutDialog(parent *qt.QWidget, current string, local, remote []string, fetch func() ([]string, error)) (*formDialog, *qt.QComboBox, *qt.QCheckBox) {
	local, remote = byName(local), byName(remote)
	d := newFormDialog(parent, "Checkout")
	branch := d.addChoice("Branch", local)
	branch.SetCurrentIndex(max(branch.FindText(current), 0))
	d.focus = branch.QWidget
	withRemote := qt.NewQCheckBox3("Include remote branches")
	d.form.AddRowWithWidget(withRemote.QWidget)
	note := qt.NewQLabel2()
	note.SetTextFormat(qt.PlainText)
	note.SetWordWrap(true)
	note.Hide()
	d.form.AddRowWithWidget(note.QWidget)
	fetching := false
	d.fieldsValid = append(d.fieldsValid, func() bool {
		return !fetching && branch.Count() > 0 && branch.CurrentText() != current
	})
	branch.OnCurrentIndexChanged(func(int) { d.check() })
	fill := func() {
		chosen := branch.CurrentText()
		branch.Clear()
		branch.AddItems(local)
		if withRemote.IsChecked() {
			branch.AddItems(remote)
		}
		// A branch no longer listed gives way to the current one.
		at := branch.FindText(chosen)
		if at < 0 {
			at = branch.FindText(current)
		}
		branch.SetCurrentIndex(max(at, 0))
		d.check()
	}
	fetched := false
	withRemote.OnToggled(func(on bool) {
		if !on || fetched {
			fill()
			return
		}
		fetched, fetching = true, true
		branch.SetEnabled(false)
		withRemote.SetEnabled(false)
		note.SetText("Fetching…")
		note.Show()
		d.check()
		go func() {
			got, err := fetch()
			mainthread.Start(func() {
				if d.finished {
					return
				}
				fetching = false
				branch.SetEnabled(true)
				withRemote.SetEnabled(true)
				switch {
				case err != nil:
					note.SetText("Fetching failed, so these are the remote branches as last fetched: " + firstLine(err.Error()))
				case len(got) == 0:
					note.SetText("There are no remote branches.")
					remote = nil
				default:
					note.Hide()
					remote = byName(got)
				}
				fill()
			})
		}()
	})
	d.check()
	return d, branch, withRemote
}

// byName returns the names in order, ignoring case.
func byName(names []string) []string {
	names = slices.Clone(names)
	slices.SortStableFunc(names, func(a, b string) int {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	})
	return names
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
	// What git says to deleting it decides what to ask next.
	w.background("Delete Branch", func() (string, error) { return r.DeleteBranch(name, false) }, func(_ string, err error) {
		if err == nil {
			w.say("Deleted the branch " + name)
			return
		}
		w.say("Kept the branch " + name)
		if w.confirm("Delete Branch", err.Error()+"\n\nDelete it anyway? Its unmerged commits will be lost.") {
			w.run("Delete Branch", func() (string, error) { return r.DeleteBranch(name, true) }, nil)
		}
	})
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

// rebaseFrom asks how to rewrite the commits from the one given up to HEAD, and
// rewrites them. Git needs a clean work tree for that, and no merge or rebase
// already going.
func (w *Window) rebaseFrom(c git.Commit) {
	const title = "Interactive Rebase"
	r := w.repo
	if r == nil || w.busy || c.Hash == git.Uncommitted {
		return
	}
	switch {
	case w.status.Merging:
		w.failed(title, "Commit or abort the merge in progress first.")
		return
	case w.status.Rebasing:
		w.failed(title, "Continue or abort the rebase in progress first, from the Branch menu.")
		return
	case w.hasTrackedChanges():
		w.failed(title, "Commit or stash the changes first: git rebases only a clean work tree.")
		return
	}
	commits, base, err := r.RebaseCommits(c.Hash)
	if err != nil {
		w.failed(title, "Commit "+c.Short+" can't be rebased from: "+err.Error()+".")
		return
	}
	d := newRebaseDialog(w.QWidget, w.config, commits)
	accepted := d.run()
	w.config.RebaseWidth, w.config.RebaseHeight = d.width, d.height
	w.saveSettings()
	if !accepted {
		return
	}
	w.startRebase(base, d.steps)
}

// hasTrackedChanges says whether anything but untracked files has changed.
func (w *Window) hasTrackedChanges() bool {
	return len(w.status.Staged) > 0 || slices.ContainsFunc(w.status.Unstaged, func(c git.Change) bool {
		return c.Code != git.Untracked
	})
}

// startRebase rewrites the commits on top of the base following the steps,
// asking first if any are dropped.
func (w *Window) startRebase(base string, steps []git.Step) {
	r := w.repo
	dropped := 0
	for _, s := range steps {
		if s.Action == git.Drop {
			dropped++
		}
	}
	if dropped > 0 && !w.confirm("Interactive Rebase", fmt.Sprintf("Drop %d of the commits? Their changes will be lost from the branch.", dropped)) {
		return
	}
	w.rebaseStep("Interactive Rebase", func() (string, error) { return r.Rebase(base, steps) })
}

// rebaseStep runs a command of a rebase as background does, and says where the
// rebase has got to: done, failed, or stopped, and how to go on.
func (w *Window) rebaseStep(name string, op func() (string, error)) {
	r := w.repo
	var stopped bool
	w.background(name, func() (string, error) {
		out, err := op()
		stopped = r.Rebasing()
		return out, err
	}, func(out string, err error) {
		const next = "choose Continue Rebase from the Branch menu, or Abort Rebase to put the branch back as it was."
		switch {
		case stopped && err != nil:
			w.say(name + " stopped")
			w.failed(name+" Stopped", err.Error()+"\n\nOnce that is put right, with any conflicts resolved and staged, "+next)
		case stopped:
			w.say("Stopped to edit a commit: stage what to add to it, then " + next)
		case err != nil:
			w.say(name + " failed")
			w.failed(name+" Failed", err.Error())
		default:
			w.say(strings.TrimSpace(name + " done. " + lastLine(out)))
		}
	})
}

// discardChanges undoes everything not committed, once that is confirmed, as
// it can't be taken back: the branch is left as if just checked out.
func (w *Window) discardChanges() {
	r := w.repo
	if r == nil || w.busy {
		return
	}
	// A file new to the index is deleted as much as one untracked is, as the
	// last commit has neither.
	changed, added := map[string]bool{}, map[string]bool{}
	for _, c := range slices.Concat(w.status.Staged, w.status.Unstaged) {
		if c.Code == git.Untracked || c.Code == 'A' {
			added[c.Path] = true
		}
	}
	for _, c := range slices.Concat(w.status.Staged, w.status.Unstaged) {
		if !added[c.Path] {
			changed[c.Path] = true
		}
	}
	branch := w.status.Branch
	if branch == "" {
		branch = "the detached HEAD"
	}
	var what []string
	if n := len(changed); n > 0 {
		what = append(what, plural(n, "changed file")+" will be put back as last committed")
	}
	if n := len(added); n > 0 {
		what = append(what, plural(n, "new file")+" will be deleted")
	}
	if w.status.Merging {
		what = append(what, "the merge in progress will be abandoned")
	}
	text := fmt.Sprintf("Discard all changes to %s?\n\n%s. Files ignored by git are kept.\n\nThis can't be undone.",
		branch, upperFirst(strings.Join(what, ", and ")))
	if !w.confirm("Discard All Changes", text) {
		return
	}
	w.run("Discard All Changes", r.DiscardChanges, nil)
}

// plural returns how many of a thing there are, with the thing's name.
func plural(n int, name string) string {
	if n == 1 {
		return "1 " + name
	}
	return fmt.Sprintf("%d %ss", n, name)
}

// upperFirst returns the text with its first letter a capital.
func upperFirst(text string) string {
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}

// doAbortRebase abandons the rebase in progress once that is confirmed.
func (w *Window) doAbortRebase() {
	r := w.repo
	if !w.confirm("Abort Rebase", "Abandon the rebase? The branch goes back to how it was before it, and changes made since it started are lost.") {
		return
	}
	w.run("Abort Rebase", r.AbortRebase, nil)
}
