// Package ui is the program's window.
package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"

	"github.com/nomad-software/merge-monkey/internal/git"
	"github.com/nomad-software/merge-monkey/internal/settings"
)

// Window is the program's main window.
type Window struct {
	*qt.QMainWindow
	// path is where the settings are saved.
	path   string
	config settings.Settings
	icon   *qt.QIcon

	tabs *qt.QTabWidget
	tab  *branchTab
	// where and said are the status bar's text: what branch is checked out, and
	// what last happened.
	where, said *qt.QLabel

	// repo is the repository open, or nil, and status what was last read of it.
	repo        *git.Repo
	status      git.Status
	mergeFilled bool

	// busy says that a git command that changes something is running, during
	// which no other can be started.
	busy bool
	// rescans and diffs count the requests for each, so that the answer to any
	// but the latest, which may be about something else by now, is dropped.
	rescans, diffs int

	// repoOnly are the actions needing only an open repository.
	repoOnly                                              []*qt.QAction
	rescan, stageAll, stageSelected, unstageSelected      *qt.QAction
	unstageAll, commit, push, forcePush, abortMerge, open *qt.QAction

	// confirm asks before doing what can't be taken back, and error tells of a
	// failure. Tests replace them.
	confirm func(title, text string) bool
	failed  func(title, text string)
}

// NewWindow makes the window, with its settings kept at the path. Start shows it.
func NewWindow(path string) *Window {
	w := &Window{QMainWindow: qt.NewQMainWindow2(), path: path, config: settings.Load(path)}
	w.confirm = func(title, text string) bool { return askYesNo(w.QWidget, title, text) }
	w.failed = func(title, text string) { showError(w.QWidget, title, text) }
	// The desktop's own look is left alone unless another is chosen.
	if w.config.Theme != "" {
		applyTheme(w.config.Theme)
	}
	w.icon = newAppIcon()
	w.SetWindowIcon(w.icon)
	qt.QGuiApplication_SetWindowIcon(w.icon)
	w.Resize(w.config.Width, w.config.Height)

	w.tab = newBranchTab(w.config)
	w.tabs = qt.NewQTabWidget2()
	w.tabs.AddTab(w.tab.page, "Branch")
	w.SetCentralWidget(w.tabs.QWidget)
	w.makeStatusBar()

	w.makeMenus()
	w.connect()
	w.showTitle()
	w.updateActions()

	w.OnCloseEvent(func(super func(*qt.QCloseEvent), event *qt.QCloseEvent) {
		w.saveSettings()
		super(event)
	})
	return w
}

// Start shows the window with a repository: the one named by the argument if
// there is one, or else the one the current folder is in, or the one last open.
func (w *Window) Start(argument string) {
	if w.config.Maximized {
		w.ShowMaximized()
	} else {
		w.Show()
	}
	// The sizes are taken as they stand only once the window has its own.
	setPanes(w.tab.sidebar, w.config.Sidebar)
	setPanes(w.tab.lists, w.config.Lists)
	setPanes(w.tab.editor, w.config.Editor)

	if argument != "" {
		if err := w.openRepository(argument); err != nil {
			w.failed("Open Repository", err.Error())
		}
		return
	}
	if cwd, err := os.Getwd(); err == nil && w.openRepository(cwd) == nil {
		return
	}
	if w.config.Repository != "" && w.openRepository(w.config.Repository) == nil {
		return
	}
	w.say("Open a repository from the File menu.")
}

// makeStatusBar makes the status bar, with its text at the left and a handle in
// its corner to drag the window bigger or smaller by. The text is in labels of
// its own, as a message shown by the bar itself would hide what is beside it.
func (w *Window) makeStatusBar() {
	bar := w.StatusBar()
	bar.SetSizeGripEnabled(true)
	w.where = qt.NewQLabel2()
	w.where.SetTextFormat(qt.PlainText)
	w.said = qt.NewQLabel2()
	w.said.SetTextFormat(qt.PlainText)
	// However long a message is, it does not hold the window open.
	w.said.SetSizePolicy2(qt.QSizePolicy__Ignored, qt.QSizePolicy__Preferred)
	bar.AddWidget(w.where.QWidget)
	bar.AddWidget2(w.said.QWidget, 1)
}

// say shows the message in the status bar.
func (w *Window) say(message string) {
	w.said.SetText(message)
}

// action adds an action of the window to a menu.
func (w *Window) action(menu *qt.QMenu, text, key string, triggered func()) *qt.QAction {
	a := qt.NewQAction5(text, w.QObject)
	if key != "" {
		a.SetShortcut(qt.NewQKeySequence2(key))
	}
	a.OnTriggered(triggered)
	menu.AddAction(a)
	return a
}

// makeMenus makes the menu bar.
func (w *Window) makeMenus() {
	bar := w.MenuBar()

	file := bar.AddMenuWithTitle("&File")
	w.open = w.action(file, "&Open Repository…", "Ctrl+O", w.chooseRepository)
	w.action(file, "&Settings…", "Ctrl+,", w.showSettings)
	file.AddSeparator()
	w.action(file, "&Quit", "Ctrl+Q", func() { w.Close() })

	branch := bar.AddMenuWithTitle("&Branch")
	w.repoOnly = append(w.repoOnly,
		w.action(branch, "&New Branch…", "Ctrl+N", w.newBranch),
		w.action(branch, "&Checkout…", "Ctrl+B", w.checkoutBranch),
		w.action(branch, "&Rename…", "", w.renameBranch),
		w.action(branch, "&Delete…", "", w.deleteBranch),
	)
	branch.AddSeparator()
	w.repoOnly = append(w.repoOnly, w.action(branch, "&Merge…", "Ctrl+M", w.mergeBranch))
	w.abortMerge = w.action(branch, "&Abort Merge", "", w.doAbortMerge)

	commit := bar.AddMenuWithTitle("&Commit")
	w.rescan = w.action(commit, "&Rescan", "F5", w.rescanRepository)
	commit.AddSeparator()
	w.stageAll = w.action(commit, "&Stage Changes", "Ctrl+Shift+T", func() { w.stageChanges(w.status.Unstaged) })
	w.stageSelected = w.action(commit, "Stage Se&lected", "Ctrl+T", func() {
		w.stageChanges(selection(w.tab.unstaged, w.status.Unstaged))
	})
	w.unstageSelected = w.action(commit, "&Unstage Selected", "Ctrl+U", func() {
		w.unstageChanges(selection(w.tab.staged, w.status.Staged))
	})
	w.unstageAll = w.action(commit, "Unstage &All", "Ctrl+Shift+U", func() { w.unstageChanges(w.status.Staged) })
	commit.AddSeparator()
	w.commit = w.action(commit, "&Commit", "Ctrl+Return", w.commitChanges)
	w.push = w.action(commit, "&Push", "Ctrl+P", func() { w.pushBranch(w.tab.force.IsChecked()) })
	w.forcePush = w.action(commit, "&Force Push…", "Ctrl+Shift+P", func() { w.pushBranch(true) })

	remote := bar.AddMenuWithTitle("&Remote")
	w.repoOnly = append(w.repoOnly,
		w.action(remote, "&Fetch", "Ctrl+Shift+F", func() {
			r := w.repo
			w.run("Fetch", r.Fetch, nil)
		}),
		w.action(remote, "Pu&ll", "Ctrl+Shift+L", func() {
			r := w.repo
			w.run("Pull", r.Pull, nil)
		}),
	)
	remote.AddAction(w.push)
	remote.AddAction(w.forcePush)
	remote.AddSeparator()
	w.repoOnly = append(w.repoOnly,
		w.action(remote, "&Add Remote…", "", w.addRemote),
		w.action(remote, "&Rename Remote…", "", w.renameRemote),
		w.action(remote, "Change &URL…", "", w.changeRemoteURL),
		w.action(remote, "R&emove Remote…", "", w.removeRemote),
	)

	help := bar.AddMenuWithTitle("&Help")
	w.action(help, "&About", "Ctrl+I", func() { newAboutDialog(w.QWidget, w.icon).Open() })
}

// connect joins the widgets of the tab to what they do.
func (w *Window) connect() {
	t := w.tab
	t.rescan.OnClicked(w.rescan.Trigger)
	t.stage.OnClicked(func() {
		if w.allStaged() {
			w.unstageAll.Trigger()
		} else {
			w.stageAll.Trigger()
		}
	})
	t.commit.OnClicked(w.commit.Trigger)
	t.push.OnClicked(w.push.Trigger)
	t.force.OnToggled(func(on bool) {
		if on {
			t.push.SetText("Force Push")
		} else {
			t.push.SetText("Push")
		}
	})
	t.message.OnTextChanged(w.updateActions)

	t.unstaged.OnItemSelectionChanged(func() { w.selectionChanged(t.unstaged, t.staged) })
	t.staged.OnItemSelectionChanged(func() { w.selectionChanged(t.staged, t.unstaged) })
	t.unstaged.OnItemDoubleClicked(func(item *qt.QListWidgetItem) {
		if row := t.unstaged.Row(item); !w.busy && row < len(w.status.Unstaged) {
			w.stageChanges(w.status.Unstaged[row : row+1])
		}
	})
	t.staged.OnItemDoubleClicked(func(item *qt.QListWidgetItem) {
		if row := t.staged.Row(item); !w.busy && row < len(w.status.Staged) {
			w.unstageChanges(w.status.Staged[row : row+1])
		}
	})
}

// selectionChanged shows the diff of what was chosen in a list. Only one list
// has a selection at a time, so that it is clear which diff is shown.
func (w *Window) selectionChanged(list, other *qt.QListWidget) {
	t := w.tab
	if t.updating {
		return
	}
	if len(list.SelectedItems()) > 0 && len(other.SelectedItems()) > 0 {
		t.updating = true
		other.ClearSelection()
		t.updating = false
	}
	w.updateActions()
	w.showDiff()
}

// allStaged says whether everything changed is staged, when the stage button
// takes it all back out instead.
func (w *Window) allStaged() bool {
	return len(w.status.Unstaged) == 0 && len(w.status.Staged) > 0
}

// updateActions enables what can be done now, in the menus and buttons alike.
func (w *Window) updateActions() {
	t := w.tab
	open := w.repo != nil
	idle := open && !w.busy
	for _, a := range w.repoOnly {
		a.SetEnabled(idle)
	}
	message := strings.TrimSpace(t.message.ToPlainText()) != ""
	set := func(a *qt.QAction, enabled bool) { a.SetEnabled(enabled) }
	set(w.open, !w.busy)
	set(w.rescan, idle)
	set(w.stageAll, idle && len(w.status.Unstaged) > 0)
	set(w.stageSelected, idle && len(t.unstaged.SelectedItems()) > 0)
	set(w.unstageSelected, idle && len(t.staged.SelectedItems()) > 0)
	set(w.unstageAll, idle && len(w.status.Staged) > 0)
	// A merge can be committed with nothing staged, when its result is the
	// same as the current branch.
	set(w.commit, idle && message && (len(w.status.Staged) > 0 || w.status.Merging))
	set(w.push, idle)
	set(w.forcePush, idle)
	set(w.abortMerge, idle && w.status.Merging)

	t.rescan.SetEnabled(w.rescan.IsEnabled())
	if w.allStaged() {
		t.stage.SetText(unstageText)
		t.stage.SetEnabled(w.unstageAll.IsEnabled())
	} else {
		t.stage.SetText(stageText)
		t.stage.SetEnabled(w.stageAll.IsEnabled())
	}
	t.commit.SetEnabled(w.commit.IsEnabled())
	t.push.SetEnabled(w.push.IsEnabled())
	t.force.SetEnabled(idle)
}

// showTitle shows the repository and branch in the title and the status bar.
func (w *Window) showTitle() {
	if w.repo == nil {
		w.SetWindowTitle("Merge Monkey")
		w.where.SetText("")
		return
	}
	s := w.status
	name := s.Branch
	if name == "" {
		name = "detached HEAD"
	}
	w.SetWindowTitle(fmt.Sprintf("%s (%s) — Merge Monkey", filepath.Base(w.repo.Root), name))
	where := name
	if s.Upstream != "" {
		where += fmt.Sprintf(" → %s  ↑%d ↓%d", s.Upstream, s.Ahead, s.Behind)
	}
	if s.Merging {
		where += "  (merging)"
	}
	w.where.SetText(where)
}

// openRepository opens the repository that holds the folder, in place of any
// that was open.
func (w *Window) openRepository(dir string) error {
	r, err := git.Open(dir)
	if err != nil {
		return err
	}
	w.repo = r
	w.config.Repository = r.Root
	w.applyStatus(git.Status{})
	w.tab.message.Clear()
	w.mergeFilled = false
	w.say("Opened " + r.Root)
	w.rescanRepository()
	return nil
}

// chooseRepository asks for a repository to open.
func (w *Window) chooseRepository() {
	start := w.config.Repository
	dir := qt.QFileDialog_GetExistingDirectory3(w.QWidget, "Open Repository", start)
	if dir == "" {
		return
	}
	if err := w.openRepository(dir); err != nil {
		w.failed("Open Repository", err.Error())
	}
}

// rescanRepository reads the status of the repository again, in the background.
func (w *Window) rescanRepository() {
	r := w.repo
	if r == nil {
		return
	}
	w.rescans++
	request := w.rescans
	go func() {
		s, err := r.Status()
		var mergeMessage string
		if err == nil && s.Merging {
			mergeMessage = r.MergeMessage()
		}
		mainthread.Start(func() {
			if request != w.rescans || r != w.repo {
				return
			}
			if err != nil {
				w.say("Rescan failed: " + firstLine(err.Error()))
				return
			}
			w.applyStatus(s)
			// The message git prepared for a merge is offered once, and not
			// again if it is cleared.
			if !s.Merging {
				w.mergeFilled = false
			} else if !w.mergeFilled {
				w.mergeFilled = true
				if strings.TrimSpace(w.tab.message.ToPlainText()) == "" {
					w.tab.message.SetPlainText(mergeMessage)
				}
			}
		})
	}()
}

// applyStatus shows the status.
func (w *Window) applyStatus(s git.Status) {
	t := w.tab
	old := w.status
	w.status = s
	t.setList(t.unstaged, old.Unstaged, s.Unstaged)
	t.setList(t.staged, old.Staged, s.Staged)
	t.unstagedLabel.SetText(fmt.Sprintf("Unstaged Changes (%d)", len(s.Unstaged)))
	t.stagedLabel.SetText(fmt.Sprintf("Staged Changes (%d)", len(s.Staged)))
	w.showTitle()
	w.updateActions()
	w.showDiff()
}

// showDiff shows, in the background, the diff of the file chosen in a list: the
// current one if it is among those selected, or else the first.
func (w *Window) showDiff() {
	t := w.tab
	list, changes, staged := t.unstaged, w.status.Unstaged, false
	if len(list.SelectedItems()) == 0 {
		list, changes, staged = t.staged, w.status.Staged, true
	}
	chosen := selection(list, changes)
	w.diffs++
	request := w.diffs
	if len(chosen) == 0 || w.repo == nil {
		t.diffLabel.SetText("Diff")
		t.diff.show("")
		return
	}
	change := chosen[0]
	if current := list.CurrentItem(); current != nil && current.IsSelected() {
		if row := list.Row(current); row < len(changes) {
			change = changes[row]
		}
	}
	side := "Unstaged"
	if staged {
		side = "Staged"
	}
	r := w.repo
	go func() {
		text, err := r.Diff(change, staged)
		mainthread.Start(func() {
			if request != w.diffs {
				return
			}
			t.diffLabel.SetText(side + ": " + change.Path)
			if err != nil {
				t.diff.show(err.Error())
				return
			}
			t.diff.show(text)
		})
	}()
}

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
	// Both have to be filled in, whichever was typed in last.
	check := func(string) {
		d.ok.SetEnabled(strings.TrimSpace(name.Text()) != "" && strings.TrimSpace(url.Text()) != "")
	}
	name.OnTextChanged(check)
	url.OnTextChanged(check)
	check("")
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

// showSettings opens the settings dialog, without waiting for it.
func (w *Window) showSettings() {
	newSettingsDialog(w.QWidget, w.config, w.applySettings).Open()
}

// applySettings puts settings chosen into effect and saves them.
func (w *Window) applySettings(s settings.Settings) {
	themeChanged := s.Theme != w.config.Theme
	w.config = s
	if themeChanged {
		applyTheme(s.Theme)
	}
	w.tab.diff.setColours(s.AddedColour, s.RemovedColour)
	w.saveSettings()
}

// saveSettings saves the settings with the window and panes as they are now.
func (w *Window) saveSettings() {
	geometry := w.NormalGeometry()
	if geometry.Width() > 0 && geometry.Height() > 0 {
		w.config.Width, w.config.Height = geometry.Width(), geometry.Height()
	}
	w.config.Maximized = w.IsMaximized()
	if w.IsVisible() {
		w.config.Sidebar = panes(w.tab.sidebar)
		w.config.Lists = panes(w.tab.lists)
		w.config.Editor = panes(w.tab.editor)
	}
	if err := w.config.Save(w.path); err != nil {
		fmt.Fprintln(os.Stderr, "merge-monkey: the settings could not be saved:", err)
	}
}
