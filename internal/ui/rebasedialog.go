package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	qt "github.com/mappu/miqt/qt6"

	"github.com/nomad-software/delta/internal/git"
	"github.com/nomad-software/delta/internal/settings"
)

// The columns of the plan of a rebase.
const (
	colAction = iota
	colStepSubject
	colStepCommit
)

// A commit's message is shown in a tooltip of at most tipLines lines of at most
// tipWidth characters, and its subject in the list cut at subjectWidth: however
// long a message is, the list stays quick and a tooltip stays on the screen.
const (
	tipLines     = 20
	tipWidth     = 100
	subjectWidth = 300
)

// rebaseDialog asks how to rewrite commits: the plan of an interactive rebase,
// a commit to a row, oldest first, with a list of what to do with each, and
// below it the message of the one chosen. Commits are dragged into
// their order. The rows stay where they are, each with its own list, as Qt
// would delete the list of a row it moved: what moves is the commits shown on
// them.
type rebaseDialog struct {
	*qt.QDialog
	// original is the plan as it starts, everything picked in order, and steps
	// the plan as it is now.
	original, steps []git.Step

	list *qt.QTreeWidget
	// actions are the lists of actions, a row's own.
	actions []*qt.QComboBox
	message *qt.QPlainTextEdit
	note    *qt.QLabel
	start   *qt.QPushButton

	// updating says that the program is changing a widget, so that its
	// signals mean nothing.
	updating bool
	// finished says that the dialog has gone, so that what was to happen
	// later has nowhere to go.
	finished bool
	// width and height are the dialog's size when it was closed, for the
	// settings.
	width, height int
	// messageUntil is until when the focus is sent back to the message if it
	// goes elsewhere, after Reword is chosen (editMessage).
	messageUntil time.Time
}

// newRebaseDialog returns the dialog of the commits, at the size it was last,
// or else most of the size of its parent.
func newRebaseDialog(parent *qt.QWidget, config settings.Settings, commits []git.Commit) *rebaseDialog {
	d := &rebaseDialog{QDialog: qt.NewQDialog(parent)}
	d.SetWindowTitle(rebaseTitle)
	switch {
	case config.RebaseWidth > 0:
		d.Resize(config.RebaseWidth, config.RebaseHeight)
	case parent != nil:
		d.Resize(parent.Width()*4/5, parent.Height()*4/5)
	}
	for _, c := range commits {
		d.original = append(d.original, git.Step{Commit: c, Action: git.Pick})
	}
	d.steps = slices.Clone(d.original)

	d.list = qt.NewQTreeWidget2()
	d.list.SetHeaderLabels([]string{"Action", "Subject", "Commit"})
	d.list.SetRootIsDecorated(false)
	d.list.SetUniformRowHeights(true)
	d.list.SetAllColumnsShowFocus(true)
	d.list.SetSelectionMode(qt.QAbstractItemView__ExtendedSelection)
	d.list.SetTextElideMode(qt.ElideRight)
	// Qt draws where a drag would drop, between rows only, as no row takes a
	// drop onto itself, and the drop itself is handled here (dropAt).
	d.list.SetDragEnabled(true)
	d.list.SetDragDropMode(qt.QAbstractItemView__InternalMove)
	d.list.SetDefaultDropAction(qt.MoveAction)
	d.list.SetDropIndicatorShown(true)
	header := d.list.Header()
	header.SetStretchLastSection(false)
	header.SetSectionResizeMode2(colStepSubject, qt.QHeaderView__Stretch)
	header.ResizeSection(colStepCommit, 90)
	for row := range d.steps {
		item := qt.NewQTreeWidgetItem()
		item.SetFlags(qt.ItemIsSelectable | qt.ItemIsEnabled | qt.ItemIsDragEnabled)
		d.list.AddTopLevelItem(item)
		combo := d.newActionList(row)
		item.SetSizeHint(colAction, combo.SizeHint())
		d.list.SetItemWidget(item, colAction, combo.QWidget)
		d.actions = append(d.actions, combo)
		d.showStep(row)
	}
	if len(d.actions) > 0 {
		header.ResizeSection(colAction, d.actions[0].SizeHint().Width())
	}
	top := labelled(heading("Commits, oldest first"), d.list.QWidget)

	font := codeFont(config.CodeFont, config.CodeFontSize)
	defer font.Delete()
	d.message = qt.NewQPlainTextEdit2()
	d.message.SetFont(font)

	split := newPaneSplitter(qt.Vertical)
	split.AddWidget(top)
	split.AddWidget(labelled(heading("Message"), d.message.QWidget))
	split.SetStretchFactor(1, 1)
	split.SetChildrenCollapsible(false)

	d.note = qt.NewQLabel2()
	d.note.SetTextFormat(qt.PlainText)
	d.note.SetWordWrap(true)
	box := qt.NewQDialogButtonBox2()
	d.start = box.AddButton2("Start Rebase", qt.QDialogButtonBox__AcceptRole)
	box.AddButtonWithButton(qt.QDialogButtonBox__Cancel)
	box.OnAccepted(d.Accept)
	box.OnRejected(d.Reject)
	bottom := qt.NewQHBoxLayout2()
	bottom.AddWidget2(d.note.QWidget, 1)
	bottom.AddWidget(box.QWidget)

	layout := qt.NewQVBoxLayout(d.QWidget)
	layout.AddWidget2(split.QWidget, 1)
	layout.AddLayout(bottom.QLayout)

	d.connect()
	if len(d.steps) > 0 {
		d.list.SetCurrentItem(d.list.TopLevelItem(0))
	}
	d.check()
	return d
}

// connect joins the widgets to what they do.
func (d *rebaseDialog) connect() {
	d.list.OnCurrentItemChanged(func(*qt.QTreeWidgetItem, *qt.QTreeWidgetItem) {
		if !d.updating {
			d.showCurrent()
		}
	})
	d.list.OnFocusInEvent(func(super func(*qt.QFocusEvent), event *qt.QFocusEvent) {
		super(event)
		d.reclaimFocus()
	})
	d.list.OnDropEvent(func(_ func(*qt.QDropEvent), event *qt.QDropEvent) {
		d.dropAt(int(event.Position().Y()))
		// Ignored, the drag is not taken as a move, after which Qt would
		// remove the rows dragged.
		event.Ignore()
		d.list.Viewport().Update()
	})
	d.message.OnTextChanged(func() {
		row := d.currentRow()
		if d.updating || row < 0 || d.steps[row].Action != git.Reword {
			return
		}
		d.steps[row].Message = d.message.ToPlainText()
		d.check()
	})
}

// newActionList returns the list of actions of a row. Only choosing in it
// applies an action, not the program showing the row's. The wheel changes it
// only once it has been clicked, so that scrolling over the rows scrolls them.
func (d *rebaseDialog) newActionList(row int) *qt.QComboBox {
	combo := qt.NewQComboBox2()
	for _, a := range git.Actions {
		combo.AddItem(actionName(a))
	}
	combo.SetFocusPolicy(qt.StrongFocus)
	combo.OnWheelEvent(func(super func(*qt.QWheelEvent), event *qt.QWheelEvent) {
		if combo.HasFocus() {
			super(event)
		} else {
			event.Ignore()
		}
	})
	// Using a row's list chooses its row, so that what is below is that
	// commit's: a commit reworded has its new message written there, which
	// would otherwise be another commit's, and read only.
	combo.OnFocusInEvent(func(super func(*qt.QFocusEvent), event *qt.QFocusEvent) {
		super(event)
		if d.reclaimFocus() {
			return
		}
		d.chooseOnly(row)
	})
	combo.OnActivated(func(index int) {
		d.chooseOnly(row)
		d.setRowAction(row, git.Actions[index])
		d.showCurrent()
		d.check()
		if git.Actions[index] == git.Reword {
			d.editMessage()
		}
	})
	return combo
}

// run waits for an answer, and says whether it was to start the rebase.
func (d *rebaseDialog) run() bool {
	accepted := d.Exec() == int(qt.QDialog__Accepted)
	d.width, d.height = d.Width(), d.Height()
	d.finished = true
	d.DeleteLater()
	return accepted
}

// actionName returns how an action is shown.
func actionName(a git.Action) string {
	return upperFirst(string(a))
}

// showStep shows a step of the plan on its row. A commit dropped is struck out.
func (d *rebaseDialog) showStep(row int) {
	s := d.steps[row]
	item := d.list.TopLevelItem(row)
	d.actions[row].SetCurrentIndex(slices.Index(git.Actions, s.Action))
	item.SetText(colStepSubject, excerpt(s.Commit.Subject, 1, subjectWidth))
	item.SetText(colStepCommit, s.Commit.Short)
	tip := excerpt(s.Commit.Message, tipLines, tipWidth)
	for column := range 3 {
		item.SetToolTip(column, tip)
		font := qt.NewQFont5(item.Font(column))
		font.SetStrikeOut(s.Action == git.Drop)
		item.SetFont(column, font)
		font.Delete()
	}
}

// excerpt returns the start of the text, as much of it as fits in so many lines
// of so many characters, with an ellipsis wherever something is left out.
func excerpt(text string, lines, width int) string {
	all := strings.Split(strings.TrimSpace(text), "\n")
	kept := all[:min(len(all), lines)]
	for i, line := range kept {
		line = strings.TrimRight(line, "\r")
		if runes := []rune(line); len(runes) > width {
			line = string(runes[:width-1]) + "…"
		}
		kept[i] = line
	}
	if len(all) > lines {
		kept = append(kept, "…")
	}
	return strings.Join(kept, "\n")
}

// selectedRows returns the rows chosen, in order.
func (d *rebaseDialog) selectedRows() []int {
	var rows []int
	for _, item := range d.list.SelectedItems() {
		rows = append(rows, d.list.IndexOfTopLevelItem(item))
	}
	slices.Sort(rows)
	return rows
}

// currentRow returns the row whose message and diff are shown, or -1.
func (d *rebaseDialog) currentRow() int {
	item := d.list.CurrentItem()
	if item == nil {
		return -1
	}
	return d.list.IndexOfTopLevelItem(item)
}

// chooseOnly makes the row the one chosen, and current, unless it already is.
func (d *rebaseDialog) chooseOnly(row int) {
	if d.currentRow() == row && slices.Equal(d.selectedRows(), []int{row}) {
		return
	}
	d.list.ClearSelection()
	d.list.SetCurrentItem(d.list.TopLevelItem(row))
}

// messageGrace is how long after Reword is chosen the focus is sent back to
// the message if it goes to the list.
const messageGrace = time.Second

// editMessage puts the cursor at the end of the message, to be changed. Closing
// a list's popup puts the focus back where it was after the list says what was
// chosen, and on Wayland only once the compositor says so, which can be later
// still, and onto the list of commits: so it is done once what is happening now
// has finished, and again whenever the focus goes to the list soon after.
func (d *rebaseDialog) editMessage() {
	d.messageUntil = time.Now().Add(messageGrace)
	timer := qt.NewQTimer2(d.QObject)
	timer.SetSingleShot(true)
	timer.OnTimeout(func() {
		timer.DeleteLater()
		if !d.finished && d.message.IsEnabled() {
			d.message.SetFocus()
			d.message.MoveCursor(qt.QTextCursor__End)
		}
	})
	timer.Start(0)
}

// reclaimFocus sends the focus back to the message, and says so, if Reword was
// chosen a moment ago.
func (d *rebaseDialog) reclaimFocus() bool {
	if time.Now().After(d.messageUntil) || !d.message.IsEnabled() {
		return false
	}
	d.messageUntil = time.Time{}
	d.message.SetFocus()
	d.message.MoveCursor(qt.QTextCursor__End)
	return true
}

// setRowAction gives the commit on a row the action. A commit reworded starts
// with the message it has.
func (d *rebaseDialog) setRowAction(row int, a git.Action) {
	s := &d.steps[row]
	if a == git.Reword && s.Action != git.Reword {
		s.Message = s.Commit.Message
	}
	s.Action = a
	d.showStep(row)
}

// dropAt moves the commits chosen to where a drag of them is dropped, at a
// height in the list: before the row there if in its top half, else after it,
// as Qt draws where it would go, and at the end if below every row.
func (d *rebaseDialog) dropAt(y int) {
	to := len(d.steps)
	if item := d.list.ItemAt2(1, y); item != nil {
		to = d.list.IndexOfTopLevelItem(item)
		if y >= d.list.VisualItemRect(item).Center().Y() {
			to++
		}
	}
	if rows := d.selectedRows(); len(rows) > 0 {
		d.moveRows(rows, to)
	}
}

// moveRows moves the commits on the rows together, in order, to before the row
// to, or the end if it is past the last, keeping them chosen.
func (d *rebaseDialog) moveRows(rows []int, to int) {
	current := d.currentRow()
	steps, now := moved(d.steps, rows, to)
	if slices.EqualFunc(steps, d.steps, func(a, b git.Step) bool { return a.Commit.Hash == b.Commit.Hash }) {
		return
	}
	d.steps = steps
	d.updating = true
	for row := range d.steps {
		d.showStep(row)
	}
	d.list.ClearSelection()
	if i := slices.Index(rows, current); i >= 0 {
		current = now[i]
	}
	d.list.SetCurrentItem3(d.list.TopLevelItem(current), 0, qt.QItemSelectionModel__NoUpdate)
	for _, row := range now {
		d.list.TopLevelItem(row).SetSelected(true)
	}
	d.updating = false
	d.showCurrent()
	d.check()
}

// moved returns the steps with those on the rows taken out and put back
// together, in order, before the step that was on the row to, or at the end if
// it is past the last, and the rows they are on then.
func moved(steps []git.Step, rows []int, to int) ([]git.Step, []int) {
	var taken, rest []git.Step
	before := 0
	for i, s := range steps {
		if !slices.Contains(rows, i) {
			rest = append(rest, s)
			continue
		}
		taken = append(taken, s)
		if i < to {
			before++
		}
	}
	at := min(to, len(steps)) - before
	now := make([]int, len(taken))
	for i := range now {
		now[i] = at + i
	}
	return slices.Concat(rest[:at], taken, rest[at:]), now
}

// showCurrent shows the message of the current row. Only a commit reworded has
// a message that can be changed.
func (d *rebaseDialog) showCurrent() {
	row := d.currentRow()
	d.updating = true
	defer func() { d.updating = false }()
	if row < 0 {
		d.message.Clear()
		return
	}
	s := d.steps[row]
	reword := s.Action == git.Reword
	text := s.Commit.Message
	if reword {
		text = s.Message
	}
	d.message.SetEnabled(reword)
	if d.message.ToPlainText() != text {
		d.message.SetPlainText(text)
	}
}

// check enables starting the rebase once the plan changes something and git can
// follow it, saying what is wrong with it if not.
func (d *rebaseDialog) check() {
	problem := planProblem(d.steps)
	changed := !slices.EqualFunc(d.steps, d.original, func(a, b git.Step) bool {
		return a.Commit.Hash == b.Commit.Hash && a.Action == b.Action
	})
	switch {
	case problem != "":
		d.note.SetText(problem)
	case !changed:
		d.note.SetText("Choose what to do with each commit, and drag them into the order to apply them in.")
	default:
		d.note.SetText(planSummary(d.steps))
	}
	d.start.SetEnabled(changed && problem == "")
}

// planProblem says what stops git from following a plan, or nothing.
func planProblem(steps []git.Step) string {
	for _, s := range steps {
		if s.Action == git.Drop {
			continue
		}
		if s.Action == git.Squash || s.Action == git.Fixup {
			return fmt.Sprintf("Commit %s can't be squashed or fixed up: no commit kept comes before it.", s.Commit.Short)
		}
		break
	}
	for _, s := range steps {
		if s.Action == git.Reword && strings.TrimSpace(s.Message) == "" {
			return fmt.Sprintf("Commit %s needs a message to be reworded.", s.Commit.Short)
		}
	}
	return ""
}

// planSummary says what a plan does, by how many commits each action other than
// picking applies to, or that it only reorders them.
func planSummary(steps []git.Step) string {
	var parts []string
	for _, a := range git.Actions[1:] {
		n := 0
		for _, s := range steps {
			if s.Action == a {
				n++
			}
		}
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%d to %s", n, a))
		}
	}
	if len(parts) == 0 {
		return "The commits will be reordered."
	}
	return strings.Join(parts, ", ") + "."
}
