// Package ui is the program's window.
package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"

	"github.com/nomad-software/delta/internal/git"
	"github.com/nomad-software/delta/internal/settings"
)

// Window is the program's main window.
type Window struct {
	*qt.QMainWindow
	// path is where the settings are saved.
	path   string
	config settings.Settings
	icon   *qt.QIcon

	// views shows the branch's changes, or the history while showHistory is
	// on.
	views *qt.QStackedWidget
	tab   *branchTab
	// history is the history, shown in place of the changes.
	history *historyTab
	// toolbar is the bar of buttons under the menus.
	toolbar *qt.QToolBar
	// where and said are the status bar's text: what branch is checked out, and
	// what last happened.
	where, said *qt.QLabel

	// repo is the repository open, or nil, and status what was last read of it.
	repo   *git.Repo
	status git.Status
	// mergeFilled says that the message git prepared for the merge in progress
	// has been offered.
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
	newBranchAction, checkoutAction, deleteAction, pull   *qt.QAction
	showHistory                                           *qt.QAction
	continueRebase, skipRebase, abortRebase               *qt.QAction

	// confirm asks before doing what can't be taken back, and failed tells of a
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
	w.history = newHistoryTab(w.config)
	w.views = qt.NewQStackedWidget2()
	w.views.AddWidget(w.tab.page)
	w.views.AddWidget(w.history.page)
	w.SetCentralWidget(w.views.QWidget)
	w.makeStatusBar()

	w.makeMenus()
	w.makeToolBar()
	w.applyCodeFont()
	w.connect()
	w.connectHistory()
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
	fade(w.where)
	fade(w.said)
	bar.AddWidget(w.where.QWidget)
	bar.AddWidget2(w.said.QWidget, 1)
}

// statusOpacity is how opaque the status bar's text is.
const statusOpacity = 0.5

// fade makes a label of the status bar see-through. The label owns the effect.
func fade(label *qt.QLabel) {
	effect := qt.NewQGraphicsOpacityEffect()
	effect.SetOpacity(statusOpacity)
	label.SetGraphicsEffect(effect.QGraphicsEffect)
}

// say shows the message in the status bar.
func (w *Window) say(message string) {
	w.said.SetText(message)
}

// makeToolBar makes the bar of large buttons under the menus, as apemail's,
// each the action of a menu, but History, which shows the history in place of
// the changes while it is on.
func (w *Window) makeToolBar() {
	w.showHistory = qt.NewQAction5("History", w.QObject)
	w.showHistory.SetCheckable(true)
	w.showHistory.SetToolTip("Show the history in place of the changes")
	w.showHistory.OnToggled(w.setHistoryShown)

	for a, shape := range map[*qt.QAction]iconShape{
		w.newBranchAction: newBranchShape, w.checkoutAction: checkoutShape, w.deleteAction: deleteShape,
		w.pull: pullShape, w.push: pushShape, w.showHistory: historyShape,
	} {
		icon := toolIcon(shape)
		a.SetIcon(icon)
		icon.Delete()
	}
	// The buttons are named without the ellipsis of their menu items.
	w.newBranchAction.SetIconText("New Branch")
	w.checkoutAction.SetIconText("Checkout")
	w.deleteAction.SetIconText("Delete")

	w.toolbar = qt.NewQToolBar4("Buttons", w.QWidget)
	w.toolbar.SetMovable(false)
	w.toolbar.SetToolButtonStyle(qt.ToolButtonTextUnderIcon)
	size := qt.NewQSize2(toolIconSize, toolIconSize)
	defer size.Delete()
	w.toolbar.SetIconSize(size)
	for _, a := range []*qt.QAction{w.newBranchAction, w.checkoutAction, w.deleteAction, nil, w.pull, w.push, nil, w.showHistory} {
		if a == nil {
			w.toolbar.AddSeparator()
			continue
		}
		w.toolbar.QWidget.AddAction(a)
		w.toolbar.WidgetForAction(a).SetMinimumWidth(toolButtonWidth)
	}
	w.AddToolBar(qt.TopToolBarArea, w.toolbar)
}

// action adds an action of the window to a menu.
func (w *Window) action(menu *qt.QMenu, text, key string, triggered func()) *qt.QAction {
	a := qt.NewQAction5(text, w.QObject)
	if key != "" {
		sequence := qt.NewQKeySequence2(key)
		defer sequence.Delete()
		a.SetShortcut(sequence)
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
	w.newBranchAction = w.action(branch, "&New Branch…", "Ctrl+N", w.newBranch)
	w.checkoutAction = w.action(branch, "&Checkout…", "Ctrl+B", w.checkoutBranch)
	rename := w.action(branch, "&Rename…", "", w.renameBranch)
	w.deleteAction = w.action(branch, "&Delete…", "", w.deleteBranch)
	w.repoOnly = append(w.repoOnly, w.newBranchAction, w.checkoutAction, rename, w.deleteAction)
	branch.AddSeparator()
	w.repoOnly = append(w.repoOnly, w.action(branch, "&Merge…", "Ctrl+M", w.mergeBranch))
	w.abortMerge = w.action(branch, "&Abort Merge", "", w.doAbortMerge)
	branch.AddSeparator()
	w.continueRebase = w.action(branch, "C&ontinue Rebase", "", func() { w.rebaseStep("Continue Rebase", w.repo.ContinueRebase) })
	w.skipRebase = w.action(branch, "S&kip Commit", "", func() { w.rebaseStep("Skip Commit", w.repo.SkipRebase) })
	w.abortRebase = w.action(branch, "A&bort Rebase", "", w.doAbortRebase)

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
	fetch := w.action(remote, "&Fetch", "Ctrl+Shift+F", func() { w.run("Fetch", w.repo.Fetch, nil) })
	w.pull = w.action(remote, "Pu&ll", "Ctrl+Shift+L", func() { w.run("Pull", w.repo.Pull, nil) })
	w.repoOnly = append(w.repoOnly, fetch, w.pull)
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
	idle := w.repo != nil && !w.busy
	for _, a := range w.repoOnly {
		a.SetEnabled(idle)
	}
	message := strings.TrimSpace(t.message.ToPlainText()) != ""
	w.open.SetEnabled(!w.busy)
	w.showHistory.SetEnabled(w.repo != nil)
	w.rescan.SetEnabled(idle)
	w.stageAll.SetEnabled(idle && len(w.status.Unstaged) > 0)
	w.stageSelected.SetEnabled(idle && len(t.unstaged.SelectedItems()) > 0)
	w.unstageSelected.SetEnabled(idle && len(t.staged.SelectedItems()) > 0)
	w.unstageAll.SetEnabled(idle && len(w.status.Staged) > 0)
	// A merge can be committed with nothing staged, when its result is the
	// same as the current branch.
	w.commit.SetEnabled(idle && message && (len(w.status.Staged) > 0 || w.status.Merging))
	w.push.SetEnabled(idle)
	w.forcePush.SetEnabled(idle)
	w.abortMerge.SetEnabled(idle && w.status.Merging)
	for _, a := range []*qt.QAction{w.continueRebase, w.skipRebase, w.abortRebase} {
		a.SetEnabled(idle && w.status.Rebasing)
	}

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
		w.SetWindowTitle("Delta")
		w.where.SetText("")
		return
	}
	s := w.status
	name := s.Branch
	if name == "" {
		name = "detached HEAD"
	}
	w.SetWindowTitle(fmt.Sprintf("%s (%s) — Delta", filepath.Base(w.repo.Root), name))
	where := name
	if s.Upstream != "" {
		where += fmt.Sprintf(" → %s  ↑%d ↓%d", s.Upstream, s.Ahead, s.Behind)
	}
	if s.Merging {
		where += "  (merging)"
	}
	switch {
	case s.Rebasing && s.Rebase.Steps > 0:
		where += fmt.Sprintf("  (rebasing %d of %d)", s.Rebase.Step, s.Rebase.Steps)
	case s.Rebasing:
		where += "  (rebasing)"
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
	w.clearHistory()
	w.tab.message.Clear()
	w.mergeFilled = false
	w.say("Opened " + r.Root)
	w.rescanRepository()
	return nil
}

// chooseRepository asks for a repository to open.
func (w *Window) chooseRepository() {
	dir := qt.QFileDialog_GetExistingDirectory3(w.QWidget, "Open Repository", w.config.Repository)
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
	w.markHistoryStale()
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
	w.tab.diff.setColours(s.AddedColour, s.RemovedColour, s.HunkColour)
	w.history.diff.setColours(s.AddedColour, s.RemovedColour, s.HunkColour)
	w.applyCodeFont()
	w.saveSettings()
}

// applyCodeFont gives the diffs and the commit message the font chosen.
func (w *Window) applyCodeFont() {
	font := codeFont(w.config.CodeFont, w.config.CodeFontSize)
	defer font.Delete()
	w.tab.diff.setCodeFont(font)
	w.history.diff.setCodeFont(font)
	w.tab.message.SetFont(font)
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
		// Panes never shown have no sizes of their own yet.
		if w.history.sized {
			w.config.History = panes(w.history.split)
			w.config.HistoryDetail = panes(w.history.detail)
		}
	}
	if err := w.config.Save(w.path); err != nil {
		fmt.Fprintln(os.Stderr, "delta: the settings could not be saved:", err)
	}
}
