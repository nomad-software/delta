package ui

import (
	"errors"
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
			w.tellFailed(name, err)
		} else {
			w.tellDone(name, out)
		}
		if after != nil {
			after(err)
		}
	})
}

// tellDone says in the status bar that the command named is done, with the last
// line of what git said.
func (w *Window) tellDone(name, out string) {
	w.say(strings.TrimSpace(name + " done. " + lastLine(out)))
}

// tellFailed says that the command named failed, and why.
func (w *Window) tellFailed(name string, err error) {
	w.say(name + " failed")
	w.failed(name+" Failed", err.Error())
}

// readFirst reads, in the background, what a dialog needs before it opens, and
// gives it to then on the main thread, so that the window never waits on git.
// What it read is dropped if by then another repository is open or a command has
// started, and a failure is told of under the title. One is read at a time:
// asking again while one is being read does nothing, so that a shortcut pressed
// twice opens one dialog.
func readFirst[T any](w *Window, title string, read func(*git.Repo) (T, error), then func(T)) {
	r := w.repo
	if r == nil || w.busy || w.reading {
		return
	}
	w.reading = true
	w.beginWork()
	go func() {
		got, err := read(r)
		mainthread.Start(func() {
			w.endWork()
			w.reading = false
			if r != w.repo || w.busy {
				return
			}
			if err != nil {
				w.failed(title, err.Error())
				return
			}
			then(got)
		})
	}()
}

// branchLists are the local and remote branches, read together.
type branchLists struct{ local, remote []string }

// readBranches reads the local and remote branches in the background, as
// readFirst does, for then.
func (w *Window) readBranches(then func(local, remote []string)) {
	readFirst(w, "Branches", func(r *git.Repo) (branchLists, error) {
		local, remote, err := r.Branches()
		return branchLists{local, remote}, err
	}, func(b branchLists) { then(b.local, b.remote) })
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
	message := w.changes.message.ToPlainText()
	if strings.TrimSpace(message) == "" {
		return
	}
	w.run("Commit", func() (string, error) { return r.Commit(message) }, func(err error) {
		if err == nil {
			w.changes.message.Clear()
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
	// A branch with an upstream pushes to it; one without asks where, once the
	// remotes are read in the background.
	if w.status.Upstream != "" {
		w.pushTo("", force)
		return
	}
	readFirst(w, "Push Failed", (*git.Repo).Remotes, func(remotes []string) {
		if remote, ok := w.pushRemote(remotes); ok {
			w.pushTo(remote, force)
		}
	})
}

// pushTo pushes the current branch to the remote, or to its upstream if that is
// empty, asking first if it is forced.
func (w *Window) pushTo(remote string, force bool) {
	r := w.repo
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
			w.changes.force.SetChecked(false)
		}
	})
}

// pushRemote returns the remote of those given to push to and make the
// upstream, asking which if there are several.
func (w *Window) pushRemote(remotes []string) (string, bool) {
	if len(remotes) == 0 {
		w.failed("Push Failed", "This repository has no remotes. Add one from the Remote menu.")
		return "", false
	}
	if len(remotes) == 1 {
		return remotes[0], true
	}
	return askChoice(w.QWidget, "Push", "Push to", remotes)
}

// without returns the items other than the one named.
func without(items []string, name string) []string {
	return slices.DeleteFunc(slices.Clone(items), func(item string) bool { return item == name })
}

// newBranch asks for a new branch and makes it, with its name made fit for one.
func (w *Window) newBranch() {
	w.readBranches(func(local, remote []string) {
		d, name, start, checkout := w.newBranchDialog(local, remote)
		if !d.run() {
			return
		}
		from := start.CurrentText()
		if from == "HEAD" {
			from = ""
		}
		w.createBranch(git.SanitiseBranchName(name.Text()), from, checkout.IsChecked())
	})
}

// newBranchDialog returns the dialog asking for a new branch, with its fields:
// the name, where it starts, and whether to check it out. OK is enabled while
// the name, made fit for a branch as it will be on OK, can be one.
func (w *Window) newBranchDialog(local, remote []string) (d *formDialog, name *qt.QLineEdit, start *qt.QComboBox, checkout *qt.QCheckBox) {
	r := w.repo
	d = newFormDialog(w.QWidget, "New Branch")
	name = d.addText("Name", "", func(text string) bool {
		sanitised := git.SanitiseBranchName(text)
		return sanitised != "" && r.ValidBranchName(sanitised)
	})
	start = d.addChoice("Start at", append(append([]string{"HEAD"}, local...), remote...))
	checkout = qt.NewQCheckBox3("Check out the new branch")
	checkout.SetChecked(true)
	d.form.AddRowWithWidget(checkout.QWidget)
	return d, name, start, checkout
}

// createBranch makes a branch at the start point, or HEAD if it is empty.
func (w *Window) createBranch(name, start string, checkout bool) {
	r := w.repo
	w.run("New Branch", func() (string, error) { return r.CreateBranch(name, start, checkout) }, nil)
}

// checkoutBranch asks for a branch and switches to it. A remote branch is
// checked out as a new local branch that tracks it.
func (w *Window) checkoutBranch() {
	w.readBranches(func(local, remote []string) {
		if len(local)+len(remote) == 0 {
			w.failed("Checkout", "There are no branches yet.")
			return
		}
		fetched := false
		d, branch, _ := newCheckoutDialog(w.QWidget, w.status.Branch, local, remote, w.checkoutFetch(&fetched))
		if d.run() {
			name := branch.CurrentText()
			w.checkout(name, !slices.Contains(local, name))
		} else if fetched {
			// What was fetched changes how far the branch is from its upstream.
			w.rescanRepository()
		}
	})
}

// checkoutFetch returns what fetches for the checkout dialog, in the background,
// and gives the remote branches after it, noting that it has in fetched. Both
// that and the count of work are only touched on the main thread, as the
// window reads them there, perhaps while the fetch is still going on.
func (w *Window) checkoutFetch(fetched *bool) func() ([]string, error) {
	r := w.repo
	return func() ([]string, error) {
		mainthread.Wait(func() {
			*fetched = true
			w.beginWork()
		})
		defer mainthread.Start(w.endWork)
		if _, err := r.Fetch(); err != nil {
			return nil, err
		}
		_, remote, err := r.Branches()
		return remote, err
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
	w.readBranches(func(local, _ []string) { w.askRenameBranch(local) })
}

// askRenameBranch asks which of the local branches to rename, and to what.
func (w *Window) askRenameBranch(local []string) {
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
	w.readBranches(func(local, _ []string) {
		items := without(local, w.status.Branch)
		if len(items) == 0 {
			w.failed("Delete Branch", "There are no other branches.")
			return
		}
		if name, ok := askChoice(w.QWidget, "Delete Branch", "Branch", items); ok {
			w.removeBranch(name)
		}
	})
}

// removeBranch deletes a branch once that is confirmed.
func (w *Window) removeBranch(name string) {
	r := w.repo
	const title = "Delete Branch"
	if !w.confirm(title, "Delete the branch "+name+"?") {
		return
	}
	// What git says to deleting it decides what to ask next.
	w.background(title, func() (string, error) { return r.DeleteBranch(name, false) }, func(_ string, err error) {
		if err == nil {
			w.say("Deleted the branch " + name)
			return
		}
		w.say("Kept the branch " + name)
		w.askForceDelete(title, name, err)
	})
}

// askForceDelete asks, once git has refused to delete a branch as it is not
// merged, saying why, whether to delete it anyway, and if so does.
func (w *Window) askForceDelete(title, name string, refusal error) {
	r := w.repo
	if w.confirm(title, refusal.Error()+"\n\nDelete it anyway? Its unmerged commits will be lost.") {
		w.run("Delete Branch", func() (string, error) { return r.DeleteBranch(name, true) }, nil)
	}
}

// mergeBranch asks for a branch and merges it into the current one.
func (w *Window) mergeBranch() {
	w.readBranches(func(local, remote []string) {
		items := append(without(local, w.status.Branch), remote...)
		if len(items) == 0 {
			w.failed("Merge", "There are no other branches.")
			return
		}
		if name, ok := askChoice(w.QWidget, "Merge", "Merge into the current branch", items); ok {
			r := w.repo
			w.run("Merge", func() (string, error) { return r.Merge(name) }, nil)
		}
	})
}

// doAbortMerge abandons the merge in progress once that is confirmed.
func (w *Window) doAbortMerge() {
	r := w.repo
	if !w.confirm("Abort Merge", "Abandon the merge? Changes made to resolve it will be lost.") {
		return
	}
	w.run("Abort Merge", r.AbortMerge, func(err error) {
		if err == nil {
			w.changes.message.Clear()
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

// chooseRemote asks which remote to act on, once they are read in the
// background, and gives it to then.
func (w *Window) chooseRemote(title string, then func(name string)) {
	readFirst(w, title, (*git.Repo).Remotes, func(remotes []string) {
		if len(remotes) == 0 {
			w.failed(title, "This repository has no remotes.")
			return
		}
		if name, ok := askChoice(w.QWidget, title, "Remote", remotes); ok {
			then(name)
		}
	})
}

// renameRemote asks for a remote and its new name.
func (w *Window) renameRemote() {
	w.chooseRemote("Rename Remote", func(from string) {
		to, ok := askText(w.QWidget, "Rename Remote", "New name", from, notEmpty)
		if !ok || to == from {
			return
		}
		r := w.repo
		w.run("Rename Remote", func() (string, error) { return r.RenameRemote(from, to) }, nil)
	})
}

// changeRemoteURL asks for a remote and, once its URL is read in the
// background, its new URL.
func (w *Window) changeRemoteURL() {
	w.chooseRemote("Change URL", func(name string) {
		// A URL that can't be read is offered empty.
		readFirst(w, "Change URL", func(r *git.Repo) (string, error) {
			current, _ := r.RemoteURL(name)
			return current, nil
		}, func(current string) {
			url, ok := askText(w.QWidget, "Change URL", "URL", current, notEmpty)
			if !ok || url == current {
				return
			}
			r := w.repo
			w.run("Change URL", func() (string, error) { return r.SetRemoteURL(name, url) }, nil)
		})
	})
}

// removeRemote asks for a remote and removes it once that is confirmed.
func (w *Window) removeRemote() {
	w.chooseRemote("Remove Remote", func(name string) {
		if !w.confirm("Remove Remote", "Remove the remote "+name+" and its remote tracking branches?") {
			return
		}
		r := w.repo
		w.run("Remove Remote", func() (string, error) { return r.RemoveRemote(name) }, nil)
	})
}

// rebaseTitle names an interactive rebase, in its questions and what is said of
// it.
const rebaseTitle = "Interactive Rebase"

// rebaseFrom asks how to rewrite the commits from the one given up to HEAD, and
// rewrites them. Git needs a clean work tree for that, and no merge or rebase
// already going.
func (w *Window) rebaseFrom(c git.Commit) {
	const title = rebaseTitle
	if w.repo == nil || w.busy || c.Hash == git.Uncommitted {
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
	// The commits are read in the background, and the plan asked for once they
	// are. A refusal is told of in words of its own, not as an error of git's.
	type plan struct {
		commits []git.Commit
		base    string
		refused error
	}
	readFirst(w, title, func(r *git.Repo) (plan, error) {
		commits, base, err := r.RebaseCommits(c.Hash)
		return plan{commits, base, err}, nil
	}, func(p plan) {
		if p.refused != nil {
			w.failed(title, "Commit "+c.Short+" can't be rebased from: "+p.refused.Error()+".")
			return
		}
		w.askRebase(p.commits, p.base)
	})
}

// askRebase asks how to rewrite the commits, on top of the base, and rewrites
// them.
func (w *Window) askRebase(commits []git.Commit, base string) {
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
	if dropped := countAction(steps, git.Drop); dropped > 0 && !w.confirm(rebaseTitle, fmt.Sprintf("Drop %d of the commits? Their changes will be lost from the branch.", dropped)) {
		return
	}
	w.rebaseStep(rebaseTitle, func() (string, error) { return r.Rebase(base, steps) })
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
			w.tellFailed(name, err)
		default:
			w.tellDone(name, out)
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
	all := slices.Concat(w.status.Staged, w.status.Unstaged)
	for _, c := range all {
		if c.Code == git.Untracked || c.Code == 'A' {
			added[c.Path] = true
		}
	}
	for _, c := range all {
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

// doAbortRebase abandons the rebase in progress once that is confirmed.
func (w *Window) doAbortRebase() {
	r := w.repo
	if !w.confirm("Abort Rebase", "Abandon the rebase? The branch goes back to how it was before it, and changes made since it started are lost.") {
		return
	}
	w.run("Abort Rebase", r.AbortRebase, nil)
}

// stashConflictText is what the status bar says of a stash applied with conflicts.
const stashConflictText = "Stash applied with conflicts: resolve the files in orange and stage them. The stash was kept."

// askStashChanges asks for a message for the stash, and whether untracked files
// go in it too, then stashes.
func (w *Window) askStashChanges() {
	d, message, untracked := w.newStashDialog()
	if d.run() {
		w.stashChanges(strings.TrimSpace(message.Text()), untracked.IsChecked())
	}
}

// newStashDialog returns the dialog asking for a stash's message, which may be
// left empty for git's own, and whether to include untracked files, unticked to
// start with.
func (w *Window) newStashDialog() (d *formDialog, message *qt.QLineEdit, untracked *qt.QCheckBox) {
	d = newFormDialog(w.QWidget, "Stash Changes")
	message = d.addText("Message", "", anyText)
	message.SetPlaceholderText("Optional")
	untracked = qt.NewQCheckBox3("Include untracked files")
	d.form.AddRowWithWidget(untracked.QWidget)
	return d, message, untracked
}

// stashChanges stashes everything uncommitted, with the message.
func (w *Window) stashChanges(message string, untracked bool) {
	r := w.repo
	w.run("Stash Changes", func() (string, error) { return r.StashChanges(message, untracked) }, nil)
}

// askStash asks for one of the stashes, newest first, once they are read in the
// background, and gives its ref to then.
func (w *Window) askStash(title string, then func(ref string)) {
	readFirst(w, title, (*git.Repo).Stashes, func(stashes []git.Stash) {
		if len(stashes) == 0 {
			w.failed(title, "There are no stashes.")
			return
		}
		var items []string
		for _, s := range stashes {
			items = append(items, s.Ref+": "+s.Subject)
		}
		if chosen, ok := askChoice(w.QWidget, title, "Stash", items); ok {
			ref, _, _ := strings.Cut(chosen, ": ")
			then(ref)
		}
	})
}

// askApplyStash asks for a stash and applies it, dropping it once it is if pop
// says so.
func (w *Window) askApplyStash(pop bool) {
	w.askStash(stashTitle(pop), func(ref string) { w.applyStash(ref, pop, true) })
}

// stashTitle names applying a stash, or popping it if pop says so.
func stashTitle(pop bool) string {
	if pop {
		return "Pop Stash"
	}
	return "Apply Stash"
}

// applyStash puts a stash's changes back, with what was staged staged again if
// index says so. With conflicts, what did not clash is applied and the stash is
// kept, which the status bar says; if what was staged can't be put back as it
// was, it asks whether to apply it with everything unstaged.
func (w *Window) applyStash(ref string, pop, index bool) {
	name := stashTitle(pop)
	r := w.repo
	w.background(name, func() (string, error) { return r.ApplyStash(ref, pop, index) }, func(out string, err error) {
		switch {
		case err == nil:
			w.tellDone(name, out)
		case errors.Is(err, git.ErrStashConflicts):
			w.say(stashConflictText)
		case errors.Is(err, git.ErrStashIndex):
			w.say(name + " stopped")
			if w.confirm(name, "The stash's staged changes can't be put back as they were. Apply it with everything unstaged?") {
				w.applyStash(ref, pop, false)
			}
		default:
			w.tellFailed(name, err)
		}
	})
}

// askDropStash asks for a stash and deletes it, after asking, as it can't be
// undone.
func (w *Window) askDropStash() {
	w.askStash("Drop Stash", w.dropStash)
}

// dropStash deletes a stash, if the question before it is answered yes.
func (w *Window) dropStash(ref string) {
	if !w.confirm("Drop Stash", "Drop "+ref+"? It can't be undone.") {
		return
	}
	r := w.repo
	w.run("Drop Stash", func() (string, error) { return r.DropStash(ref) }, nil)
}

// askQuickFix asks, every time, as it rewrites the branch, whether to fold the
// changes not committed into the last commit, or with none the last commit into
// the one before it, as the last rescan found, and if so does it.
func (w *Window) askQuickFix() {
	q := w.quickFix
	if w.busy || w.repo == nil || q.Head == "" || q.Refused != "" {
		return
	}
	if w.confirm("Quick Fix", quickFixQuestion(q, w.status)) {
		r := w.repo
		w.run("Quick Fix", func() (string, error) { return r.RunQuickFix(q) }, nil)
	}
}

// quickFixQuestion returns what is asked before a quick fix: what goes into
// what, and, if what is rewritten has been pushed, that the branch will need a
// force push.
func quickFixQuestion(q git.QuickFix, s git.Status) string {
	text := fmt.Sprintf("Fold the last commit, “%s”, into the one before it, “%s”, keeping that one's message?", q.Last, q.Previous)
	rewritten := 2
	if q.Amend {
		text = fmt.Sprintf("Fold every change not committed, untracked files included, into the last commit, “%s”, keeping its message?", q.Last)
		rewritten = 1
	}
	return text + " This rewrites the branch's history." + pushedNote(s, rewritten)
}

// pushedNote returns what a question about rewriting the last commits, as many
// as rewritten, adds if any of them have been pushed: that the branch will then
// need a force push. Those not yet pushed are those ahead of the upstream.
func pushedNote(s git.Status, rewritten int) string {
	if s.Upstream == "" || s.Ahead >= rewritten {
		return ""
	}
	return fmt.Sprintf("\n\nIt has been pushed to %s, so the branch will need a force push.", s.Upstream)
}

// uncommitLast takes the last commit back, its changes left unstaged as if they
// were new, once asked.
func (w *Window) uncommitLast() {
	r := w.repo
	if r == nil || w.busy || !w.canUncommit {
		return
	}
	if w.confirm("Uncommit", uncommitQuestion(w.quickFix.Last, w.status)) {
		w.run("Uncommit", r.Uncommit, nil)
	}
}

// uncommitQuestion returns what is asked before uncommitting the last commit,
// of the subject: that its changes are kept, and, if it has been pushed, that
// the branch will need a force push.
func uncommitQuestion(subject string, s git.Status) string {
	return fmt.Sprintf("Uncommit the last commit, “%s”? Its changes are kept, unstaged.", subject) + pushedNote(s, 1)
}

// finishBranch finishes with the current branch, once asked: it checks out the
// main branch, main or master, pulls into it, and deletes the branch. There must
// be nothing uncommitted, as the branch goes. If git won't delete it, as it is
// not merged, it asks whether to delete it anyway.
func (w *Window) finishBranch() {
	r, branch, main := w.repo, w.status.Branch, git.MainBranch(w.localBranches)
	if r == nil || w.busy || branch == "" || main == "" || branch == main {
		return
	}
	if w.status.HasChanges() {
		w.failed("Finish Branch", "Commit or stash your changes first, as the branch "+branch+" is deleted.")
		return
	}
	if !w.confirm("Finish Branch", fmt.Sprintf("Check out %s, pull into it, and delete the branch %s?", main, branch)) {
		return
	}
	w.background("Finish Branch", func() (string, error) { return r.FinishBranch(branch, main) }, func(_ string, err error) {
		switch {
		case err == nil:
			w.say(fmt.Sprintf("Finished with %s: %s is checked out and pulled, and %s deleted.", branch, main, branch))
		case errors.Is(err, git.ErrBranchKept):
			w.say(fmt.Sprintf("%s is checked out and pulled, and %s kept.", main, branch))
			w.askForceDelete("Finish Branch", branch, err)
		default:
			w.tellFailed("Finish Branch", err)
		}
	})
}

// tagRelease tags a release of the current branch: once the latest version is
// read in the background, from the tags here and on the remote it goes to, it
// asks for the version, above the latest, and the message, then tags HEAD with
// it, annotated, and pushes the branch and the tag together.
func (w *Window) tagRelease() {
	branch := w.status.Branch
	if w.repo == nil || w.busy || branch == "" {
		return
	}
	readFirst(w, "Tag Release", func(r *git.Repo) (git.Release, error) { return r.ReadRelease(branch) }, func(rel git.Release) {
		d, version, message := newTagDialog(w.QWidget, rel, w.status.HasChanges())
		if !d.run() {
			return
		}
		r, v, text := w.repo, version(), strings.TrimSpace(message.Text())
		w.run("Tag Release", func() (string, error) { return r.TagRelease(branch, v, text, rel.Remote) }, func(err error) {
			if err == nil {
				w.say(fmt.Sprintf("Tagged %s and pushed it with %s to %s.", v, branch, rel.Remote))
			}
		})
	})
}
