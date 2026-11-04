package ui

import (
	"slices"

	qt "github.com/mappu/miqt/qt6"

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
	page *qt.QWidget
	// sidebar splits the lists from the rest, lists splits one list from the
	// other, and editor splits the diff from the message.
	sidebar, lists, editor *qt.QSplitter

	unstagedLabel, stagedLabel, diffLabel *qt.QLabel
	unstaged, staged                      *qt.QListWidget
	diff                                  *diffView
	message                               *qt.QPlainTextEdit

	rescan, stage, commit, push *qt.QPushButton
	force                       *qt.QCheckBox

	// updating says that a list is being changed by the program, so that its
	// selection changing means nothing.
	updating bool
}

func newChangesView(config settings.Settings) *changesView {
	v := &changesView{page: qt.NewQWidget2()}

	v.unstagedLabel = heading("Unstaged Changes")
	v.stagedLabel = heading("Staged Changes")
	v.unstaged = newFileList()
	v.staged = newFileList()
	v.lists = newPaneSplitter(qt.Vertical)
	v.lists.AddWidget(labelled(v.unstagedLabel, v.unstaged.QWidget))
	v.lists.AddWidget(labelled(v.stagedLabel, v.staged.QWidget))

	v.diffLabel = heading("Diff")
	v.diffLabel.SetTextFormat(qt.PlainText)
	v.diff = newDiffView(config.AddedColour, config.RemovedColour, config.HunkColour)

	v.message = qt.NewQPlainTextEdit2()
	v.message.SetPlaceholderText("Commit message")

	v.rescan = qt.NewQPushButton3("Rescan")
	// The button is as wide as its longer text, so that nothing moves as it
	// changes between them.
	v.stage = qt.NewQPushButton3(unstageText)
	v.stage.SetMinimumWidth(v.stage.SizeHint().Width())
	v.stage.SetText(stageText)
	v.commit = qt.NewQPushButton3("Commit")
	v.push = qt.NewQPushButton3("Push")
	v.force = qt.NewQCheckBox3("Force push")
	buttons := qt.NewQVBoxLayout2()
	buttons.SetContentsMargins(0, 0, 0, 0)
	for _, b := range []*qt.QPushButton{v.rescan, v.stage, v.commit, v.push} {
		buttons.AddWidget(b.QWidget)
	}
	buttons.AddWidget(v.force.QWidget)
	buttons.AddStretch()

	bottom := qt.NewQWidget2()
	bottomLayout := qt.NewQHBoxLayout(bottom)
	bottomLayout.SetContentsMargins(0, 0, 0, 0)
	bottomLayout.AddLayout(buttons.QLayout)
	bottomLayout.AddWidget2(v.message.QWidget, 1)

	v.editor = newPaneSplitter(qt.Vertical)
	v.editor.AddWidget(labelled(v.diffLabel, v.diff.QWidget))
	v.editor.AddWidget(bottom)
	// As the window changes size, the diff takes up the difference.
	v.editor.SetStretchFactor(0, 1)

	v.sidebar = newPaneSplitter(qt.Horizontal)
	v.sidebar.AddWidget(v.lists.QWidget)
	v.sidebar.AddWidget(v.editor.QWidget)
	v.sidebar.SetStretchFactor(1, 1)
	for _, s := range []*qt.QSplitter{v.lists, v.editor, v.sidebar} {
		s.SetChildrenCollapsible(false)
	}

	layout := qt.NewQVBoxLayout(v.page)
	layout.SetContentsMargins(pageMargin, pageMargin, pageMargin, pageMargin)
	layout.AddWidget(v.sidebar.QWidget)
	return v
}

// newFileList returns a list of changed files. Several can be selected with ctrl
// and shift. Dragging moves the one file selected along with the pointer, so that
// each is shown in turn, where Qt would otherwise select all those passed over.
func newFileList() *qt.QListWidget {
	list := qt.NewQListWidget2()
	setChangeIconSize(list)
	list.SetSelectionMode(qt.QAbstractItemView__ExtendedSelection)
	list.SetUniformItemSizes(true)
	list.OnMouseMoveEvent(func(super func(*qt.QMouseEvent), event *qt.QMouseEvent) {
		if event.Buttons()&qt.LeftButton == 0 {
			super(event)
			return
		}
		if event.Modifiers() != qt.NoModifier {
			return
		}
		position := event.Pos()
		if item := list.ItemAt(position); item != nil && !(item.IsSelected() && len(list.SelectedItems()) == 1) {
			list.SetCurrentRow2(list.Row(item), qt.QItemSelectionModel__ClearAndSelect)
		}
	})
	return list
}

// setList replaces what a list shows, keeping the selection and scroll position.
// If nothing changed the list is not touched at all.
func (v *changesView) setList(list *qt.QListWidget, old, changes []git.Change) {
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
		list.AddItemWithItem(item)
		item.SetSelected(selected[c.Path])
	}
	list.VerticalScrollBar().SetValue(scroll)
}

// selection returns the changes selected in a list that shows the changes.
func selection(list *qt.QListWidget, changes []git.Change) []git.Change {
	var chosen []git.Change
	for _, item := range list.SelectedItems() {
		if row := list.Row(item); row < len(changes) {
			chosen = append(chosen, changes[row])
		}
	}
	return chosen
}
