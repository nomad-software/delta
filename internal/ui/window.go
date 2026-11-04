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
	"github.com/nomad-software/delta/internal/instance"
	"github.com/nomad-software/delta/internal/settings"
)

// Window is the program's main window.
type Window struct {
	*qt.QMainWindow
	// path is where the settings are saved.
	path   string
	config settings.Settings
	icon   *qt.QIcon

	// views shows the changes, or the history in their place while
	// showHistory is on.
	views   *qt.QStackedWidget
	changes *changesView
	history *historyView
	// toolbar is the bar of buttons under the menus, and bar the dropdowns of
	// the repositories and branches at its right.
	toolbar *qt.QToolBar
	bar     *repoBar
	// where and said are the status bar's text: what branch is checked out, and
	// what last happened.
	where, said *qt.QLabel

	// repo is the repository open, or nil, and status what was last read of it.
	repo   *git.Repo
	status git.Status
	// mergeFilled says that the message git prepared for the merge in progress
	// has been offered.
	mergeFilled bool
	// chooseFirst says that a repository has just been opened, so that the first
	// rescan's first unstaged file is chosen, if there is one.
	chooseFirst bool

	// busy says that a git command that changes something is running, during
	// which no other can be started.
	busy bool
	// stashes is how many stashes there are, as the last rescan read.
	stashes int
	// rescans and diffs count the requests for each, so that the answer to any
	// but the latest, which may be about something else by now, is dropped.
	rescans, diffs int
	// working is how many pieces of work are going on in the background, which
	// progress, under the toolbar, shows.
	working  int
	progress *progress

	// The menus' actions, many of them the toolbar's too. repoOnly are those
	// needing only an open repository.
	repoOnly []*qt.QAction
	open     *qt.QAction
	// The Branch menu's.
	newBranchAction, checkoutAction, deleteAction, showHistory   *qt.QAction
	abortMerge, continueRebase, skipRebase, abortRebase, discard *qt.QAction
	// The Commit menu's.
	rescan, stageAll, stageSelected, unstageSelected, unstageAll, commit *qt.QAction
	// The Stash menu's.
	stashAction, applyStashAction, popStashAction, dropStashAction *qt.QAction
	// The Remote menu's.
	pull, push, forcePush *qt.QAction

	// What the buttons do, where it is not a menu's action. pushButton pushes,
	// forcing it if Force push is ticked, as the Remote menu's Push, on Ctrl+P,
	// never does. forceToggle is the toolbar's Push's option to force it, ticked
	// with the Force push checkbox.
	pushButton, forceToggle *qt.QAction
	// unstaging says that the Stage Changes button beside the message is Unstage
	// Changes, as it is while everything is staged.
	unstaging bool

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

	w.changes = newChangesView(w.config)
	w.history = newHistoryView(w.config)
	w.views = qt.NewQStackedWidget2()
	w.views.AddWidget(w.changes.page)
	w.views.AddWidget(w.history.page)
	w.bar = newRepoBar()
	w.bar.setRepositories(w.config.Repositories, "")
	// The progress bar is under the toolbar, the width of the window.
	w.progress = newProgress()
	central := qt.NewQWidget2()
	layout := qt.NewQVBoxLayout(central)
	layout.SetContentsMargins(0, 0, 0, 0)
	layout.SetSpacing(0)
	layout.AddWidget(w.progress.QWidget)
	layout.AddWidget2(w.views.QWidget, 1)
	w.SetCentralWidget(central)
	w.makeStatusBar()

	w.makeMenus()
	w.makeToolBar()
	w.applyColours()
	w.applyCodeFont()
	w.connect()
	w.connectHistory()
	// Once joined up, so that the toolbar's option and the buttons follow.
	w.changes.force.SetChecked(w.config.ForcePush)
	w.showTitle()
	w.updateActions()

	w.OnCloseEvent(func(super func(*qt.QCloseEvent), event *qt.QCloseEvent) {
		w.saveSettings()
		super(event)
	})
	return w
}

// Start shows the window with a repository: the one named by the argument if
// there is one, or else the one the current folder is in, or else the one last
// opened, or else none, until one is opened from the File menu.
func (w *Window) Start(argument string) {
	if w.config.Maximized {
		w.ShowMaximized()
	} else {
		w.Show()
	}
	// The sizes are taken as they stand only once the window has its own.
	setPanes(w.changes.sidebar, w.config.Sidebar)
	setPanes(w.changes.lists, w.config.Lists)
	setPanes(w.changes.editor, w.config.Editor)

	if argument != "" {
		if err := w.openRepository(argument); err != nil {
			w.failed("Open Repository", err.Error())
		}
		return
	}
	if cwd, err := os.Getwd(); err == nil && w.openRepository(cwd) == nil {
		return
	}
	last := w.config.LastRepository
	if last == "" {
		w.say("Open a repository from the File menu.")
	} else if err := w.openRepository(last); err != nil {
		w.say(fmt.Sprintf("%s could not be opened again: %s", last, firstLine(err.Error())))
	}
}

// Activate brings the window to the front, as it is when the program is started
// again while it runs, and opens the repository the folder handed over is in,
// unless it is the one open. A folder named on the command line that is in no
// repository is told of; the folder the program was started in is ignored if it
// is in none, as on starting. The request's token is the desktop's leave to take
// the focus under Wayland, which is refused without it; Qt reads it from the
// environment when it asks for the focus, and then takes it out.
func (w *Window) Activate(r instance.Request) {
	if r.Token != "" {
		os.Setenv("XDG_ACTIVATION_TOKEN", r.Token)
	}
	w.SetWindowState(w.WindowState() &^ qt.WindowMinimized)
	w.Show()
	w.Raise()
	w.ActivateWindow()
	if r.Folder == "" {
		return
	}
	repo, err := git.Open(r.Folder)
	switch {
	case err != nil:
		if r.Named {
			w.failed("Open Repository", err.Error())
		}
	case w.repo != nil && repo.Root == w.repo.Root:
	case w.busy:
		w.say(repo.Root + " was not opened, as a command is running.")
	default:
		if err := w.openRepository(repo.Root); err != nil {
			w.failed("Open Repository", err.Error())
		}
	}
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

// beginWork notes that a piece of work has gone into the background, which the
// progress bar shows, and endWork that one has come back, every time, whether what it
// gave is used or not, so that the two always pair.
func (w *Window) beginWork() {
	w.working++
	if w.working == 1 {
		w.progress.start()
	}
}

func (w *Window) endWork() {
	w.working--
	if w.working == 0 {
		w.progress.stop()
	}
}

// say shows the message in the status bar.
func (w *Window) say(message string) {
	w.said.SetText(message)
}

// makeToolBar makes the bar of large buttons under the menus, as apemail's: the
// menus' actions, but Push, whose action is that of the button beside the
// message too, and the dropdowns at the right.
func (w *Window) makeToolBar() {
	for a, shape := range map[*qt.QAction]iconShape{
		w.open: openShape, w.newBranchAction: newBranchShape, w.checkoutAction: checkoutShape, w.deleteAction: deleteShape,
		w.rescan: rescanShape, w.pull: pullShape, w.push: pushShape, w.pushButton: pushShape, w.showHistory: historyShape,
	} {
		icon := toolIcon(shape)
		a.SetIcon(icon)
		icon.Delete()
	}
	// The buttons are named without the ellipsis of their menu items.
	w.open.SetIconText("Open")
	w.newBranchAction.SetIconText("New Branch")
	w.checkoutAction.SetIconText("Checkout")
	w.deleteAction.SetIconText("Delete")
	w.showHistory.SetIconText("History")

	w.toolbar = qt.NewQToolBar4("Buttons", w.QWidget)
	w.toolbar.SetMovable(false)
	w.toolbar.SetToolButtonStyle(qt.ToolButtonTextUnderIcon)
	size := qt.NewQSize2(toolIconSize, toolIconSize)
	defer size.Delete()
	w.toolbar.SetIconSize(size)
	// The buttons have no tooltips. Qt gives a button its action's name as one
	// whatever is set, so the toolbar takes the event that would show it.
	noTips := qt.NewQObject2(w.toolbar.QObject)
	noTips.OnEventFilter(func(super func(*qt.QObject, *qt.QEvent) bool, watched *qt.QObject, event *qt.QEvent) bool {
		return event.Type() == qt.QEvent__ToolTip || super(watched, event)
	})
	for _, a := range []*qt.QAction{w.open, nil, w.newBranchAction, w.checkoutAction, w.deleteAction, nil, w.rescan, nil, w.pull, w.pushButton, nil, w.showHistory} {
		if a == nil {
			w.toolbar.AddSeparator()
			continue
		}
		w.toolbar.QWidget.AddAction(a)
		button := w.toolbar.WidgetForAction(a)
		button.SetMinimumWidth(toolButtonWidth)
		button.InstallEventFilter(noTips)
	}
	// Push is split: its arrow opens a menu with an option to force the push,
	// ticked with the Force push checkbox. The menu is the button's own, so that
	// the Remote menu is left as it is.
	w.forceToggle = qt.NewQAction5("Force push", w.QObject)
	w.forceToggle.SetCheckable(true)
	w.forceToggle.OnToggled(w.changes.force.SetChecked)
	push := qt.UnsafeNewQToolButton(w.toolbar.WidgetForAction(w.pushButton).UnsafePointer())
	pushMenu := qt.NewQMenu(push.QWidget)
	pushMenu.AddAction(w.forceToggle)
	push.SetMenu(pushMenu)
	push.SetPopupMode(qt.QToolButton__MenuButtonPopup)

	// The dropdowns take up the rest, at the right.
	w.toolbar.AddWidget(w.bar.widget)
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
	// Show History is the toolbar's History too: it shows the history in place
	// of the changes while it is ticked.
	w.showHistory = qt.NewQAction5("Show &History", w.QObject)
	w.showHistory.SetCheckable(true)
	historyKey := qt.NewQKeySequence2("Ctrl+H")
	defer historyKey.Delete()
	w.showHistory.SetShortcut(historyKey)
	w.showHistory.OnToggled(w.setHistoryShown)
	branch.AddAction(w.showHistory)
	branch.AddSeparator()
	w.repoOnly = append(w.repoOnly, w.action(branch, "&Merge…", "Ctrl+M", w.mergeBranch))
	w.abortMerge = w.action(branch, "&Abort Merge", "", w.doAbortMerge)
	branch.AddSeparator()
	w.continueRebase = w.action(branch, "C&ontinue Rebase", "", func() { w.rebaseStep("Continue Rebase", w.repo.ContinueRebase) })
	w.skipRebase = w.action(branch, "S&kip Commit", "", func() { w.rebaseStep("Skip Commit", w.repo.SkipRebase) })
	w.abortRebase = w.action(branch, "A&bort Rebase", "", w.doAbortRebase)
	branch.AddSeparator()
	w.discard = w.action(branch, "Discard &All Changes…", "Ctrl+Shift+D", w.discardChanges)

	commit := bar.AddMenuWithTitle("&Commit")
	w.rescan = w.action(commit, "&Rescan", "F5", w.rescanRepository)
	commit.AddSeparator()
	w.stageAll = w.action(commit, "&Stage Changes", "Ctrl+Shift+S", func() { w.stageChanges(w.status.Unstaged) })
	w.stageSelected = w.action(commit, "Stage Se&lected", "Ctrl+S", func() {
		w.stageChanges(selection(w.changes.unstaged, w.status.Unstaged))
	})
	w.unstageSelected = w.action(commit, "&Unstage Selected", "Ctrl+U", func() {
		w.unstageChanges(selection(w.changes.staged, w.status.Staged))
	})
	w.unstageAll = w.action(commit, "Unstage &All", "Ctrl+Shift+U", func() { w.unstageChanges(w.status.Staged) })
	commit.AddSeparator()
	w.commit = w.action(commit, "&Commit", "Ctrl+Return", w.commitChanges)

	stash := bar.AddMenuWithTitle("&Stash")
	w.stashAction = w.action(stash, "Stash &Changes…", "Ctrl+Shift+Z", w.askStashChanges)
	w.applyStashAction = w.action(stash, "&Apply Stash…", "", func() { w.askApplyStash(false) })
	w.popStashAction = w.action(stash, "&Pop Stash…", "", func() { w.askApplyStash(true) })
	stash.AddSeparator()
	w.dropStashAction = w.action(stash, "&Drop Stash…", "", w.askDropStash)

	remote := bar.AddMenuWithTitle("&Remote")
	fetch := w.action(remote, "&Fetch", "Ctrl+Shift+F", func() { w.run("Fetch", w.repo.Fetch, nil) })
	w.pull = w.action(remote, "Pu&ll", "Ctrl+Shift+L", func() { w.run("Pull", w.repo.Pull, nil) })
	w.repoOnly = append(w.repoOnly, fetch, w.pull)
	w.push = w.action(remote, "&Push", "Ctrl+P", func() { w.pushBranch(false) })
	w.pushButton = qt.NewQAction5("Push", w.QObject)
	w.pushButton.OnTriggered(func() { w.pushBranch(w.changes.force.IsChecked()) })
	w.forcePush = w.action(remote, "&Force Push…", "Ctrl+Shift+P", func() { w.pushBranch(true) })
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

// connect joins the widgets of the changes to what they do.
func (w *Window) connect() {
	v := w.changes
	v.rescan.OnClicked(w.rescan.Trigger)
	// Stage Changes stages everything listed, or unstages it all if it is all
	// staged.
	v.stage.OnClicked(func() {
		if w.allStaged() {
			w.unstageAll.Trigger()
		} else {
			w.stageAll.Trigger()
		}
	})
	v.commit.OnClicked(w.commit.Trigger)
	v.push.OnClicked(w.pushButton.Trigger)
	v.force.OnToggled(func(on bool) {
		w.forceToggle.SetChecked(on)
		text := "Push"
		if on {
			text = "Force Push"
		}
		v.push.SetText(text)
		w.pushButton.SetIconText(text)
	})
	v.message.OnTextChanged(w.updateActions)
	w.bar.repos.OnActivated(w.chooseListedRepository)
	w.bar.branches.OnActivated(w.chooseBranch)

	v.unstaged.OnItemSelectionChanged(func() { w.selectionChanged(v.unstaged, v.staged) })
	v.staged.OnItemSelectionChanged(func() { w.selectionChanged(v.staged, v.unstaged) })
	v.unstaged.OnItemDoubleClicked(func(item *qt.QListWidgetItem) {
		if row := v.unstaged.Row(item); !w.busy && row < len(w.status.Unstaged) {
			w.stageChanges(w.status.Unstaged[row : row+1])
		}
	})
	v.staged.OnItemDoubleClicked(func(item *qt.QListWidgetItem) {
		if row := v.staged.Row(item); !w.busy && row < len(w.status.Staged) {
			w.unstageChanges(w.status.Staged[row : row+1])
		}
	})
}

// selectionChanged shows the diff of what was chosen in a list. Only one list
// has a selection at a time, so that it is clear which diff is shown.
func (w *Window) selectionChanged(list, other *qt.QListWidget) {
	v := w.changes
	if v.updating {
		return
	}
	if len(list.SelectedItems()) > 0 && len(other.SelectedItems()) > 0 {
		v.updating = true
		other.ClearSelection()
		v.updating = false
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
	v := w.changes
	idle := w.repo != nil && !w.busy
	for _, a := range w.repoOnly {
		a.SetEnabled(idle)
	}
	message := strings.TrimSpace(v.message.ToPlainText()) != ""
	w.open.SetEnabled(!w.busy)
	w.bar.repos.SetEnabled(!w.busy && len(w.bar.paths) > 0)
	w.bar.branches.SetEnabled(idle && w.bar.branches.Count() > 0)
	w.showHistory.SetEnabled(w.repo != nil)
	w.rescan.SetEnabled(idle)
	w.stageAll.SetEnabled(idle && len(w.status.Unstaged) > 0)
	w.stageSelected.SetEnabled(idle && len(v.unstaged.SelectedItems()) > 0)
	w.unstageSelected.SetEnabled(idle && len(v.staged.SelectedItems()) > 0)
	w.unstageAll.SetEnabled(idle && len(w.status.Staged) > 0)
	// A merge can be committed with nothing staged, when its result is the
	// same as the current branch.
	w.commit.SetEnabled(idle && message && (len(w.status.Staged) > 0 || w.status.Merging))
	w.push.SetEnabled(idle)
	w.pushButton.SetEnabled(idle)
	w.forcePush.SetEnabled(idle)
	w.abortMerge.SetEnabled(idle && w.status.Merging)
	// A rebase has its own way back, Abort Rebase.
	w.discard.SetEnabled(idle && !w.status.Rebasing && (len(w.status.Unstaged)+len(w.status.Staged) > 0 || w.status.Merging))
	for _, a := range []*qt.QAction{w.continueRebase, w.skipRebase, w.abortRebase} {
		a.SetEnabled(idle && w.status.Rebasing)
	}
	// Stashes are made and used only outside a merge or rebase.
	stashable := idle && !w.status.Merging && !w.status.Rebasing
	w.stashAction.SetEnabled(stashable && len(w.status.Unstaged)+len(w.status.Staged) > 0)
	for _, a := range []*qt.QAction{w.applyStashAction, w.popStashAction, w.dropStashAction} {
		a.SetEnabled(stashable && w.stashes > 0)
	}

	v.rescan.SetEnabled(w.rescan.IsEnabled())
	// Stage Changes unstages instead when everything is staged.
	unstage := w.allStaged()
	if unstage {
		v.stage.SetEnabled(w.unstageAll.IsEnabled())
	} else {
		v.stage.SetEnabled(w.stageAll.IsEnabled())
	}
	if unstage != w.unstaging {
		w.unstaging = unstage
		text := stageText
		if unstage {
			text = unstageText
		}
		v.stage.SetText(text)
	}
	v.commit.SetEnabled(w.commit.IsEnabled())
	v.push.SetEnabled(w.pushButton.IsEnabled())
	v.force.SetEnabled(idle)
	w.forceToggle.SetEnabled(idle)
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
	// It is remembered, to be opened again from the bar and on starting, and
	// saved at once.
	changed := w.config.Remember(r.Root)
	if changed || w.config.LastRepository != r.Root {
		w.config.LastRepository = r.Root
		w.saveSettings()
	}
	w.bar.setRepositories(w.config.Repositories, r.Root)
	w.bar.setBranches(nil, "")
	w.applyStatus(git.Status{})
	w.clearHistory()
	w.changes.message.Clear()
	w.mergeFilled = false
	w.stashes = 0
	w.chooseFirst = true
	w.say("Opened " + r.Root)
	w.rescanRepository()
	return nil
}

// chooseRepository asks for a repository to open.
func (w *Window) chooseRepository() {
	// It starts beside the repository open, if there is one.
	start := ""
	if w.repo != nil {
		start = filepath.Dir(w.repo.Root)
	}
	dir := qt.QFileDialog_GetExistingDirectory3(w.QWidget, "Open Repository", start)
	if dir == "" {
		return
	}
	if err := w.openRepository(dir); err != nil {
		w.failed("Open Repository", err.Error())
	}
}

// chooseListedRepository opens the repository chosen in the bar, unless it is
// the one open. If it can't be, the one open is chosen again.
func (w *Window) chooseListedRepository(index int) {
	if index < 0 || index >= len(w.bar.paths) || w.busy {
		return
	}
	path := w.bar.paths[index]
	if w.repo != nil && path == w.repo.Root {
		return
	}
	if err := w.openRepository(path); err != nil {
		current := ""
		if w.repo != nil {
			current = w.repo.Root
		}
		w.bar.setRepositories(w.config.Repositories, current)
		w.failed("Open Repository", err.Error())
	}
}

// chooseBranch checks out the branch chosen in the bar, unless it is the
// current one. Whatever happens, the rescan after it chooses the current one
// again.
func (w *Window) chooseBranch(index int) {
	if index < 0 || w.busy || w.repo == nil {
		return
	}
	if name := w.bar.branches.ItemText(index); name != w.status.Branch {
		w.checkout(name, false)
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
	w.beginWork()
	go func() {
		s, err := r.Status()
		// The branches are listed in the bar, and stay as they were if they
		// can't be read, as do how many stashes there are.
		local, _, branchErr := r.Branches()
		stashes, stashErr := r.Stashes()
		var mergeMessage string
		if err == nil && s.Merging {
			mergeMessage = r.MergeMessage()
		}
		mainthread.Start(func() {
			w.endWork()
			if request != w.rescans || r != w.repo {
				return
			}
			if err != nil {
				w.say("Rescan failed: " + firstLine(err.Error()))
				return
			}
			if branchErr == nil {
				w.bar.setBranches(local, s.Branch)
			}
			if stashErr == nil {
				w.stashes = len(stashes)
			}
			w.applyStatus(s)
			w.chooseFirstFile()
			// The message git prepared for a merge is offered once, and not
			// again if it is cleared.
			if !s.Merging {
				w.mergeFilled = false
			} else if !w.mergeFilled {
				w.mergeFilled = true
				if strings.TrimSpace(w.changes.message.ToPlainText()) == "" {
					w.changes.message.SetPlainText(mergeMessage)
				}
			}
		})
	}()
}

// chooseFirstFile chooses the first unstaged file, showing its diff, once the
// status of a repository just opened is read, unless a file has been chosen.
func (w *Window) chooseFirstFile() {
	if !w.chooseFirst {
		return
	}
	w.chooseFirst = false
	v := w.changes
	if v.unstaged.Count() > 0 && len(v.unstaged.SelectedItems())+len(v.staged.SelectedItems()) == 0 {
		v.unstaged.SetCurrentRow(0)
	}
}

// applyStatus shows the status.
func (w *Window) applyStatus(s git.Status) {
	v := w.changes
	old := w.status
	w.status = s
	v.setList(v.unstaged, old.Unstaged, s.Unstaged)
	v.setList(v.staged, old.Staged, s.Staged)
	v.unstagedLabel.SetText(fmt.Sprintf("Unstaged Changes (%d)", len(s.Unstaged)))
	v.stagedLabel.SetText(fmt.Sprintf("Staged Changes (%d)", len(s.Staged)))
	w.showTitle()
	w.updateActions()
	w.showDiff()
}

// showDiff shows, in the background, the diff of the file chosen in a list: the
// current one if it is among those selected, or else the first.
func (w *Window) showDiff() {
	v := w.changes
	list, changes, staged := v.unstaged, w.status.Unstaged, false
	if len(list.SelectedItems()) == 0 {
		list, changes, staged = v.staged, w.status.Staged, true
	}
	chosen := selection(list, changes)
	w.diffs++
	request := w.diffs
	if len(chosen) == 0 || w.repo == nil {
		v.diffLabel.SetText("Diff")
		v.diff.show("")
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
	w.beginWork()
	go func() {
		text, err := r.Diff(change, staged)
		mainthread.Start(func() {
			w.endWork()
			if request != w.diffs {
				return
			}
			v.diffLabel.SetText(side + ": " + change.Path)
			if err != nil {
				v.diff.show(err.Error())
				return
			}
			v.diff.show(text)
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
	// It is how many are read from the next read on.
	w.history.pageSize = s.HistoryPageSize
	// A lower limit drops those opened longest ago at once.
	if w.config.LimitRepositories() {
		current := ""
		if w.repo != nil {
			current = w.repo.Root
		}
		w.bar.setRepositories(w.config.Repositories, current)
	}
	w.applyColours()
	w.applyCodeFont()
	w.saveSettings()
}

// applyColours gives the diffs, the circles of changed files and the progress
// bar the colours chosen, and paints again what shows them.
func (w *Window) applyColours() {
	c := w.config
	w.changes.diff.setColours(c.AddedColour, c.RemovedColour, c.HunkColour)
	w.history.diff.setColours(c.AddedColour, c.RemovedColour, c.HunkColour)
	changeColours = changeCodes(c.ChangeColours)
	w.progress.colour = c.ProgressColour
	for _, view := range []*qt.QAbstractItemView{w.changes.unstaged.QAbstractItemView, w.changes.staged.QAbstractItemView, w.history.files.QAbstractItemView} {
		view.Viewport().Update()
	}
	w.progress.Update()
}

// applyCodeFont gives the diffs and the commit message the font chosen.
func (w *Window) applyCodeFont() {
	font := codeFont(w.config.CodeFont, w.config.CodeFontSize)
	defer font.Delete()
	w.changes.diff.setCodeFont(font)
	w.history.diff.setCodeFont(font)
	w.changes.message.SetFont(font)
}

// saveSettings saves the settings with the window and panes as they are now.
func (w *Window) saveSettings() {
	geometry := w.NormalGeometry()
	if geometry.Width() > 0 && geometry.Height() > 0 {
		w.config.Width, w.config.Height = geometry.Width(), geometry.Height()
	}
	w.config.Maximized = w.IsMaximized()
	w.config.ForcePush = w.changes.force.IsChecked()
	if w.IsVisible() {
		w.config.Sidebar = panes(w.changes.sidebar)
		w.config.Lists = panes(w.changes.lists)
		w.config.Editor = panes(w.changes.editor)
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
