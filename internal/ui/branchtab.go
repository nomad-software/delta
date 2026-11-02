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

// branchTab is the first tab: the unstaged and staged files in a sidebar, and
// the diff above the message and its buttons, as in git gui.
type branchTab struct {
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

func newBranchTab(config settings.Settings) *branchTab {
	t := &branchTab{page: qt.NewQWidget2()}

	t.unstagedLabel = heading("Unstaged Changes")
	t.stagedLabel = heading("Staged Changes")
	t.unstaged = newFileList()
	t.staged = newFileList()
	t.lists = newPaneSplitter(qt.Vertical)
	t.lists.AddWidget(labelled(t.unstagedLabel, t.unstaged.QWidget))
	t.lists.AddWidget(labelled(t.stagedLabel, t.staged.QWidget))

	t.diffLabel = heading("Diff")
	t.diffLabel.SetTextFormat(qt.PlainText)
	t.diff = newDiffView(config.AddedColour, config.RemovedColour, config.HunkColour)

	t.message = qt.NewQPlainTextEdit2()
	t.message.SetPlaceholderText("Commit message")

	t.rescan = qt.NewQPushButton3("Rescan")
	// The button is as wide as its longer text, so that nothing moves as it
	// changes between them.
	t.stage = qt.NewQPushButton3(unstageText)
	t.stage.SetMinimumWidth(t.stage.SizeHint().Width())
	t.stage.SetText(stageText)
	t.commit = qt.NewQPushButton3("Commit")
	t.push = qt.NewQPushButton3("Push")
	t.force = qt.NewQCheckBox3("Force push")
	buttons := qt.NewQVBoxLayout2()
	buttons.SetContentsMargins(0, 0, 0, 0)
	for _, b := range []*qt.QPushButton{t.rescan, t.stage, t.commit, t.push} {
		buttons.AddWidget(b.QWidget)
	}
	buttons.AddWidget(t.force.QWidget)
	buttons.AddStretch()

	bottom := qt.NewQWidget2()
	bottomLayout := qt.NewQHBoxLayout(bottom)
	bottomLayout.SetContentsMargins(0, 0, 0, 0)
	bottomLayout.AddLayout(buttons.QLayout)
	bottomLayout.AddWidget2(t.message.QWidget, 1)

	t.editor = newPaneSplitter(qt.Vertical)
	t.editor.AddWidget(labelled(t.diffLabel, t.diff.QWidget))
	t.editor.AddWidget(bottom)
	// As the window changes size, the diff takes up the difference.
	t.editor.SetStretchFactor(0, 1)

	t.sidebar = newPaneSplitter(qt.Horizontal)
	t.sidebar.AddWidget(t.lists.QWidget)
	t.sidebar.AddWidget(t.editor.QWidget)
	t.sidebar.SetStretchFactor(1, 1)
	for _, s := range []*qt.QSplitter{t.lists, t.editor, t.sidebar} {
		s.SetChildrenCollapsible(false)
	}

	layout := qt.NewQVBoxLayout(t.page)
	layout.SetContentsMargins(8, 8, 8, 8)
	layout.AddWidget(t.sidebar.QWidget)
	return t
}

// newFileList returns a list of changed files. Several can be selected with ctrl
// and shift. Dragging moves the one file selected along with the pointer, so that
// each is shown in turn, where Qt would otherwise select all those passed over.
func newFileList() *qt.QListWidget {
	list := qt.NewQListWidget2()
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
func (t *branchTab) setList(list *qt.QListWidget, old, changes []git.Change) {
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
	t.updating = true
	defer func() { t.updating = false }()
	list.Clear()
	for _, c := range changes {
		item := qt.NewQListWidgetItem2(describe(c))
		item.SetToolTip(explain(c))
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
