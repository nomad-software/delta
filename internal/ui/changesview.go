package ui

import (
	"slices"

	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtgui"
	"github.com/nomad-software/qute/qtwidgets"

	"github.com/nomad-software/delta/internal/git"
	"github.com/nomad-software/delta/internal/settings"
)

// The texts of the stage button, which unstages when everything is staged.
const (
	stageText   = "Stage Changes"
	unstageText = "Unstage Changes"
)

// changesView is what the window shows unless the history is on show: the
// unstaged and staged files in a sidebar, and the diff above the message and its
// buttons, as in git gui.
type changesView struct {
	page *qtwidgets.QWidget
	// sidebar splits the lists from the rest, lists splits one list from the
	// other, and editor splits the diff from the message.
	sidebar, lists, editor *qtwidgets.QSplitter

	unstagedLabel, stagedLabel, diffLabel *qtwidgets.QLabel
	unstaged, staged                      *qtwidgets.QListWidget
	diff                                  *diffView
	message                               *qtwidgets.QPlainTextEdit

	rescan, stage, commit, push *qtwidgets.QPushButton
	force                       *qtwidgets.QCheckBox

	// updating says that a list is being changed by the program, so that its
	// selection changing means nothing.
	updating bool
}

func newChangesView(config settings.Settings) *changesView {
	v := &changesView{page: qtwidgets.NewQWidget(nil)}

	v.unstagedLabel = heading("Unstaged Changes")
	v.stagedLabel = heading("Staged Changes")
	v.unstaged = newFileList()
	v.staged = newFileList()
	v.lists = newPaneSplitter(qtcore.Vertical)
	v.lists.AddWidget(labelled(v.unstagedLabel, v.unstaged))
	v.lists.AddWidget(labelled(v.stagedLabel, v.staged))

	v.diffLabel = heading("Diff")
	v.diffLabel.SetTextFormat(qtcore.PlainText)
	v.diff = newDiffView(config.AddedColour, config.RemovedColour, config.HunkColour)

	v.message = qtwidgets.NewQPlainTextEdit(nil)
	v.message.SetPlaceholderText("Commit message")

	v.rescan = qtwidgets.NewQPushButtonWithText("Rescan", nil)
	// The button is as wide as its longer text, so that nothing moves as it
	// changes between them.
	v.stage = qtwidgets.NewQPushButtonWithText(unstageText, nil)
	v.stage.SetMinimumWidth(v.stage.SizeHint().Width)
	v.stage.SetText(stageText)
	v.commit = qtwidgets.NewQPushButtonWithText("Commit", nil)
	v.push = qtwidgets.NewQPushButtonWithText("Push", nil)
	v.force = qtwidgets.NewQCheckBoxWithText("Force push", nil)
	buttons := qtwidgets.NewQVBoxLayout(nil)
	buttons.SetContentsMargins(qtcore.QMargins{})
	for _, b := range []*qtwidgets.QPushButton{v.rescan, v.stage, v.commit, v.push} {
		buttons.AddWidget(b)
	}
	buttons.AddWidget(v.force)
	buttons.AddStretch()

	bottom := qtwidgets.NewQWidget(nil)
	bottomLayout := qtwidgets.NewQHBoxLayout(bottom)
	bottomLayout.SetContentsMargins(qtcore.QMargins{})
	bottomLayout.AddLayout(buttons)
	bottomLayout.AddWidgetFull(v.message, 1, 0)

	v.editor = newPaneSplitter(qtcore.Vertical)
	v.editor.AddWidget(labelled(v.diffLabel, v.diff))
	v.editor.AddWidget(bottom)
	// As the window changes size, the diff takes up the difference.
	v.editor.SetStretchFactor(0, 1)

	v.sidebar = newPaneSplitter(qtcore.Horizontal)
	v.sidebar.AddWidget(v.lists)
	v.sidebar.AddWidget(v.editor)
	v.sidebar.SetStretchFactor(1, 1)
	for _, s := range []*qtwidgets.QSplitter{v.lists, v.editor, v.sidebar} {
		s.SetChildrenCollapsible(false)
	}

	layout := qtwidgets.NewQVBoxLayout(v.page)
	layout.SetContentsMargins(margins(pageMargin))
	layout.AddWidget(v.sidebar)
	return v
}

// fileList is a list of changed files. Several can be selected with ctrl and
// shift. Dragging moves the one file selected along with the pointer, so that
// each is shown in turn, where Qt would otherwise select all those passed over.
type fileList struct{ qtwidgets.QListWidgetBase }

// newFileList returns a list of changed files.
func newFileList() *qtwidgets.QListWidget {
	list := &fileList{}
	qtwidgets.NewQListWidgetWith(list, nil)
	setChangeIconSize(list.QListWidget)
	list.SetSelectionMode(qtwidgets.QAbstractItemViewExtendedSelection)
	list.SetUniformItemSizes(true)
	return list.QListWidget
}

// MouseMoveEvent chooses the one file under the pointer while dragging.
func (list *fileList) MouseMoveEvent(event *qtgui.QMouseEvent) {
	if event.Buttons()&qtcore.LeftButton == 0 {
		list.Super().MouseMoveEvent(event)
		return
	}
	if event.Modifiers() != qtcore.NoModifier {
		return
	}
	position := event.Position().ToPoint()
	if item := list.ItemAt(position); item != nil && !(item.IsSelected() && len(list.SelectedItems()) == 1) {
		list.SetCurrentRowWith(list.Row(item), qtcore.QItemSelectionModelClearAndSelect)
	}
}

// setList replaces what a list shows, keeping the selection and scroll position.
// If nothing changed the list is not touched at all.
func (v *changesView) setList(list *qtwidgets.QListWidget, old, changes []git.Change) {
	if slices.Equal(old, changes) {
		return
	}
	selected := map[string]bool{}
	for _, item := range list.SelectedItems() {
		if row := list.Row(item); row < len(old) {
			selected[old[row].Path] = true
		}
	}
	scroll := list.VerticalScrollBar().Value()
	v.updating = true
	defer func() { v.updating = false }()
	list.Clear()
	for _, c := range changes {
		item := newChangeItem(c)
		list.AddItem(item)
		item.SetSelected(selected[c.Path])
	}
	list.VerticalScrollBar().SetValue(scroll)
}

// selection returns the changes selected in a list that shows the changes.
func selection(list *qtwidgets.QListWidget, changes []git.Change) []git.Change {
	var chosen []git.Change
	for _, item := range list.SelectedItems() {
		if row := list.Row(item); row < len(changes) {
			chosen = append(chosen, changes[row])
		}
	}
	return chosen
}
