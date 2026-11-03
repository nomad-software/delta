package ui

import (
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"

	qt "github.com/mappu/miqt/qt6"

	"github.com/nomad-software/delta/internal/git"
	"github.com/nomad-software/delta/internal/settings"
)

// commitFiles commits a file of each name, with the name as its message.
func commitFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		writeFile(t, dir, name, name+"\n")
		runGit(t, dir, "add", name)
		runGit(t, dir, "commit", "-q", "-m", name)
	}
}

// logSubjects returns the subjects of HEAD's commits, oldest first.
func logSubjects(t *testing.T, dir string) []string {
	t.Helper()
	r, err := git.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	commits, err := r.Log(100, 0)
	if err != nil {
		t.Fatal(err)
	}
	var subjects []string
	for _, c := range slices.Backward(commits) {
		subjects = append(subjects, c.Subject)
	}
	return subjects
}

// newTestRebaseDialog returns a dialog of the commits from the one with the
// subject up to HEAD, and their base.
func newTestRebaseDialog(t *testing.T, dir, subject string) (*rebaseDialog, string) {
	t.Helper()
	r, err := git.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	commits, err := r.Log(100, 0)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(commits, func(c git.Commit) bool { return c.Subject == subject })
	plan, base, err := r.RebaseCommits(commits[i].Hash)
	if err != nil {
		t.Fatal(err)
	}
	var d *rebaseDialog
	on(func() { d = newRebaseDialog(nil, settings.Defaults(), plan) })
	t.Cleanup(func() {
		on(func() {
			d.finished = true
			d.DeleteLater()
		})
	})
	return d, base
}

// choose selects the rows, the first of them current.
func (d *rebaseDialog) choose(rows ...int) {
	d.list.ClearSelection()
	d.list.SetCurrentItem(d.list.TopLevelItem(rows[0]))
	for _, row := range rows {
		d.list.TopLevelItem(row).SetSelected(true)
	}
}

// setActionOf chooses the action in the list of the row, as a person does.
func (d *rebaseDialog) setActionOf(row int, a git.Action) {
	d.actions[row].Activated(slices.Index(git.Actions, a))
}

// planText returns the plan as action and subject, a step at a time.
func planText(steps []git.Step) []string {
	var text []string
	for _, s := range steps {
		text = append(text, string(s.Action)+" "+s.Commit.Subject)
	}
	return text
}

func TestRebaseDialogPlan(t *testing.T) {
	dir := newRepo(t)
	commitFiles(t, dir, "a", "b", "c", "d")
	d, _ := newTestRebaseDialog(t, dir, "a")
	on(func() {
		if d.start.IsEnabled() {
			t.Error("can start with nothing changed")
		}
		// Two at once move together, and stay chosen.
		d.choose(2, 3)
		d.moveRows([]int{2, 3}, 0)
		if got := planText(d.steps); !slices.Equal(got, []string{"pick c", "pick d", "pick a", "pick b"}) {
			t.Errorf("moved up %q", got)
		}
		if rows := d.selectedRows(); !slices.Equal(rows, []int{0, 1}) {
			t.Errorf("chosen %v", rows)
		}
		d.moveRows([]int{0, 1}, 3)
		if got := planText(d.steps); !slices.Equal(got, []string{"pick a", "pick c", "pick d", "pick b"}) {
			t.Errorf("moved down %q", got)
		}
		if !d.start.IsEnabled() {
			t.Errorf("can't start a reordering: %q", d.note.Text())
		}

		// Nothing comes before the first commit kept to fold it into.
		d.setActionOf(0, git.Drop)
		d.setActionOf(1, git.Fixup)
		if d.start.IsEnabled() || !strings.Contains(d.note.Text(), "squashed or fixed up") {
			t.Errorf("fixing up the first commit kept: %v %q", d.start.IsEnabled(), d.note.Text())
		}
		if !d.list.TopLevelItem(0).Font(colStepSubject).StrikeOut() {
			t.Error("a dropped commit is not struck out")
		}
		d.setActionOf(0, git.Pick)

		// A commit reworded starts with its message, which can't be emptied.
		d.setActionOf(2, git.Reword)
		if !d.message.IsEnabled() || d.message.ToPlainText() != "d" {
			t.Errorf("rewording: read only %v, %q", !d.message.IsEnabled(), d.message.ToPlainText())
		}
		d.message.SetPlainText("  ")
		if d.start.IsEnabled() || !strings.Contains(d.note.Text(), "needs a message") {
			t.Errorf("an empty message: %v %q", d.start.IsEnabled(), d.note.Text())
		}
		d.message.SetPlainText("new d")
		if !d.start.IsEnabled() || d.note.Text() != "1 to fixup, 1 to reword." {
			t.Errorf("ready: %v %q", d.start.IsEnabled(), d.note.Text())
		}
		// Another commit's message is only shown.
		d.choose(3)
		if d.message.IsEnabled() || d.message.ToPlainText() != "b" {
			t.Errorf("picked: read only %v, %q", !d.message.IsEnabled(), d.message.ToPlainText())
		}
		if got := d.steps[2].Message; got != "new d" {
			t.Errorf("the new message is %q", got)
		}
	})
}

// TestRebaseDialogCutsLongMessages checks that a long message is shown in the
// list as the start of its first line, and in full in the message pane.
func TestRebaseDialogCutsLongMessages(t *testing.T) {
	dir := newRepo(t)
	long := strings.Repeat("word ", 200)
	body := strings.Repeat("a line of the body\n", 100)
	writeFile(t, dir, "a", "a\n")
	runGit(t, dir, "add", "a")
	runGit(t, dir, "commit", "-q", "-m", long, "-m", body)
	d, _ := newTestRebaseDialog(t, dir, strings.TrimSpace(long))
	on(func() {
		item := d.list.TopLevelItem(0)
		if got := []rune(item.Text(colStepSubject)); len(got) != subjectWidth || got[len(got)-1] != '…' {
			t.Errorf("subject of %d characters", len(got))
		}
		tip := strings.Split(item.ToolTip(colStepSubject), "\n")
		if len(tip) != tipLines+1 || tip[len(tip)-1] != "…" || len([]rune(tip[0])) != tipWidth {
			t.Errorf("tooltip of %d lines, the first of %d characters", len(tip), len([]rune(tip[0])))
		}
		if got := d.message.ToPlainText(); !strings.HasSuffix(got, "a line of the body") || !strings.HasPrefix(got, "word word") {
			t.Errorf("message pane %q", got)
		}
	})
}

// TestRebaseDialogDragAndLists checks that commits dragged are dropped where Qt
// shows they would go, keeping each row's list of actions, and that each list
// gives only its own row an action.
func TestRebaseDialogDragAndLists(t *testing.T) {
	dir := newRepo(t)
	commitFiles(t, dir, "a", "b", "c", "d")
	d, _ := newTestRebaseDialog(t, dir, "a")
	on(func() {
		d.Resize(800, 600)
		d.Show()
		rect := func(row int) *qt.QRect { return d.list.VisualItemRect(d.list.TopLevelItem(row)) }

		// Into the top half of a row goes before it, the bottom half after.
		d.choose(3)
		d.dropAt(rect(1).Top() + 1)
		if got := planText(d.steps); !slices.Equal(got, []string{"pick a", "pick d", "pick b", "pick c"}) {
			t.Errorf("dropped before b %q", got)
		}
		d.choose(0, 2)
		d.dropAt(rect(3).Bottom() - 1)
		if got := planText(d.steps); !slices.Equal(got, []string{"pick d", "pick c", "pick a", "pick b"}) {
			t.Errorf("dropped after c %q", got)
		}
		if rows := d.selectedRows(); !slices.Equal(rows, []int{2, 3}) || d.currentRow() != 2 {
			t.Errorf("chosen %v, current %d", rows, d.currentRow())
		}
		// Below every row is the end.
		d.choose(0)
		d.dropAt(rect(3).Bottom() + 50)
		if got := planText(d.steps); !slices.Equal(got, []string{"pick c", "pick a", "pick b", "pick d"}) {
			t.Errorf("dropped at the end %q", got)
		}

		// A row's list gives only that row the action, whatever is chosen,
		// and the list follows the commit when it moves.
		d.choose(0, 1)
		d.actions[3].Activated(slices.Index(git.Actions, git.Drop))
		if got := planText(d.steps); !slices.Equal(got, []string{"pick c", "pick a", "pick b", "drop d"}) {
			t.Errorf("after the list %q", got)
		}
		d.choose(3)
		d.moveRows([]int{3}, 2)
		if got := d.actions[2].CurrentIndex(); git.Actions[got] != git.Drop {
			t.Errorf("the list of the moved commit says %s", git.Actions[got])
		}
		if got := d.actions[3].CurrentIndex(); git.Actions[got] != git.Pick {
			t.Errorf("the list of the commit moved past says %s", git.Actions[got])
		}
		d.setActionOf(0, git.Edit)
		d.setActionOf(1, git.Edit)
		if got := planText(d.steps); !slices.Equal(got, []string{"edit c", "edit a", "drop d", "pick b"}) {
			t.Errorf("after the lists %q", got)
		}
		for row, combo := range d.actions {
			if d.list.ItemWidget(d.list.TopLevelItem(row), colAction) == nil || combo.CurrentText() != actionName(d.steps[row].Action) {
				t.Errorf("row %d shows %q", row, combo.CurrentText())
			}
		}
		d.Hide()
	})
}

// TestRewordFromARowsList checks that choosing Reword in a row's list, with
// another row chosen, shows that commit's message to be written, as there would
// otherwise be nowhere to write it.
func TestRewordFromARowsList(t *testing.T) {
	dir := newRepo(t)
	commitFiles(t, dir, "a", "b", "c")
	d, base := newTestRebaseDialog(t, dir, "a")
	on(func() {
		d.Show()
		d.ActivateWindow()
	})
	waitFor(t, "the dialog to be active", func() bool { return d.IsActiveWindow() })
	on(func() {
		d.choose(0)
		d.actions[2].Activated(slices.Index(git.Actions, git.Reword))
		if d.currentRow() != 2 || !slices.Equal(d.selectedRows(), []int{2}) {
			t.Errorf("current %d, chosen %v", d.currentRow(), d.selectedRows())
		}
		if !d.message.IsEnabled() || d.message.ToPlainText() != "c" {
			t.Errorf("message: read only %v, %q", !d.message.IsEnabled(), d.message.ToPlainText())
		}
		d.message.SetPlainText("c reworded\n\nWith a body.")
		if got := d.steps[2].Message; got != "c reworded\n\nWith a body." || !d.start.IsEnabled() {
			t.Errorf("message %q, can start %v", got, d.start.IsEnabled())
		}
		// Clicking another row's list chooses that row.
		d.actions[1].SetFocus()
		if d.currentRow() != 1 || d.message.ToPlainText() != "b" {
			t.Errorf("after clicking b's list: current %d, message %q", d.currentRow(), d.message.ToPlainText())
		}
	})
	var steps []git.Step
	on(func() { steps = slices.Clone(d.steps) })
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.status.Branch == "main" && !w.busy })
	on(func() { w.startRebase(base, steps) })
	waitFor(t, "the rebase", func() bool { return !w.busy && strings.HasPrefix(w.said.Text(), "Interactive Rebase done") })
	if got := logSubjects(t, dir); !slices.Equal(got, []string{"first", "a", "b", "c reworded"}) {
		t.Errorf("subjects %q", got)
	}
}

// pressKey sends a key, with its text, to a widget, as typing does.
func pressKey(w *qt.QWidget, key qt.Key, text string) {
	for _, kind := range []qt.QEvent__Type{qt.QEvent__KeyPress, qt.QEvent__KeyRelease} {
		event := qt.NewQKeyEvent3(kind, int(key), qt.NoModifier, text)
		qt.QCoreApplication_SendEvent(w.QObject, event.QEvent)
		event.Delete()
	}
}

// typeText types the text into whatever has the focus.
func typeText(text string) {
	for _, r := range text {
		pressKey(qt.QApplication_FocusWidget(), qt.Key(unicode.ToUpper(r)), string(r))
	}
}

// TestRewordThroughThePopup chooses Reword the way a person does, in the popup
// of a row's list, and then types, without clicking anywhere first: the typing
// must go into that commit's message, which closing the popup used to take the
// focus away from.
func TestRewordThroughThePopup(t *testing.T) {
	dir := newRepo(t)
	commitFiles(t, dir, "a", "b", "c")
	d, base := newTestRebaseDialog(t, dir, "a")
	on(func() {
		d.Show()
		d.ActivateWindow()
	})
	waitFor(t, "the dialog to be active", func() bool { return d.IsActiveWindow() })
	combo := d.actions[1]
	on(func() {
		combo.SetFocus()
		combo.ShowPopup()
	})
	waitFor(t, "the popup", func() bool { return combo.View().IsVisible() })
	on(func() {
		// From Pick, Reword is two down, after Fixup.
		pressKey(combo.View().QWidget, qt.Key_Down, "")
		pressKey(combo.View().QWidget, qt.Key_Down, "")
		pressKey(combo.View().QWidget, qt.Key_Return, "\r")
	})
	waitFor(t, "the message to have the focus", func() bool {
		return !combo.View().IsVisible() && d.message.HasFocus()
	})
	// On Wayland the focus comes back later still, to the list of commits,
	// and to the list of actions, and is sent on to the message.
	on(d.list.SetFocus)
	waitFor(t, "the message to have the focus back from the list", d.message.HasFocus)
	on(func() {
		if !d.message.IsEnabled() || d.currentRow() != 1 || d.steps[1].Action != git.Reword {
			t.Errorf("read only %v, current %d, %s", !d.message.IsEnabled(), d.currentRow(), d.steps[1].Action)
		}
		typeText(" changed")
	})
	var steps []git.Step
	on(func() {
		if got := d.steps[1].Message; got != "b changed" {
			t.Errorf("the new message is %q", got)
		}
		steps = slices.Clone(d.steps)
		d.Hide()
	})
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.status.Branch == "main" && !w.busy })
	on(func() { w.startRebase(base, steps) })
	waitFor(t, "the rebase", func() bool { return !w.busy && strings.HasPrefix(w.said.Text(), "Interactive Rebase done") })
	if got := logSubjects(t, dir); !slices.Equal(got, []string{"first", "a", "b changed", "c"}) {
		t.Errorf("subjects %q", got)
	}
}

// TestOnlyRewordEdits checks that only a commit reworded has a message that
// can be changed, and that the list can have the focus again once the moment
// after choosing Reword has passed.
func TestOnlyRewordEdits(t *testing.T) {
	dir := newRepo(t)
	commitFiles(t, dir, "a", "b")
	d, _ := newTestRebaseDialog(t, dir, "a")
	on(func() {
		d.Show()
		d.ActivateWindow()
	})
	waitFor(t, "the dialog to be active", func() bool { return d.IsActiveWindow() })
	on(func() { d.setActionOf(0, git.Reword) })
	waitFor(t, "the message to have the focus", func() bool { return d.message.HasFocus() })
	on(func() {
		d.messageUntil = time.Time{}
		d.list.SetFocus()
		if !d.list.HasFocus() {
			t.Error("the focus can't go to the list")
		}
		for _, a := range []git.Action{git.Pick, git.Edit, git.Squash, git.Fixup, git.Drop} {
			d.setActionOf(1, a)
			if d.message.IsEnabled() || d.message.ToPlainText() != "b" {
				t.Errorf("%s: editable %v, %q", a, d.message.IsEnabled(), d.message.ToPlainText())
			}
		}
		d.Hide()
	})
}

// TestRebaseDialogSize checks that the dialog opens at the size it was last
// closed at, or else at most of its parent's.
func TestRebaseDialogSize(t *testing.T) {
	on(func() {
		parent := qt.NewQWidget2()
		defer parent.Delete()
		parent.Resize(1000, 800)
		config := settings.Defaults()
		d := newRebaseDialog(parent, config, nil)
		if d.Width() != 800 || d.Height() != 640 {
			t.Errorf("first opened at %dx%d", d.Width(), d.Height())
		}
		d.Resize(700, 500)
		d.Reject()
		d.finished = true
		config.RebaseWidth, config.RebaseHeight = d.Width(), d.Height()
		d.DeleteLater()
		d = newRebaseDialog(parent, config, nil)
		defer d.DeleteLater()
		if d.Width() != 700 || d.Height() != 500 {
			t.Errorf("opened again at %dx%d", d.Width(), d.Height())
		}
	})
}

func TestMoved(t *testing.T) {
	steps := make([]git.Step, 5)
	for i := range steps {
		steps[i].Commit.Subject = string(rune('a' + i))
	}
	for _, c := range []struct {
		rows   []int
		to     int
		want   string
		chosen []int
	}{
		{[]int{3}, 1, "adbce", []int{1}},
		{[]int{0}, 5, "bcdea", []int{4}},
		{[]int{0}, 9, "bcdea", []int{4}},
		{[]int{1, 3}, 0, "bdace", []int{0, 1}},
		{[]int{1, 3}, 5, "acebd", []int{3, 4}},
		{[]int{1, 3}, 3, "acbde", []int{2, 3}},
		{[]int{2}, 2, "abcde", []int{2}},
		{[]int{2}, 3, "abcde", []int{2}},
	} {
		got, chosen := moved(steps, c.rows, c.to)
		var text string
		for _, s := range got {
			text += s.Commit.Subject
		}
		if text != c.want || !slices.Equal(chosen, c.chosen) {
			t.Errorf("moving %v to %d: %s on %v, want %s on %v", c.rows, c.to, text, chosen, c.want, c.chosen)
		}
	}
}

func TestExcerpt(t *testing.T) {
	for _, c := range []struct{ text, want string }{
		{"short", "short"},
		{"  one\r\ntwo\n", "one\ntwo"},
		{"abcdefgh", "abcd…"},
		{"ééééééé", "éééé…"},
		{"1\n2\n3\n4", "1\n2\n3\n…"},
	} {
		if got := excerpt(c.text, 3, 5); got != c.want {
			t.Errorf("excerpt(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

func TestInteractiveRebase(t *testing.T) {
	dir := newRepo(t)
	commitFiles(t, dir, "a", "b", "c")
	d, base := newTestRebaseDialog(t, dir, "a")
	var steps []git.Step
	on(func() {
		d.setActionOf(1, git.Edit)
		d.setActionOf(2, git.Drop)
		steps = slices.Clone(d.steps)
	})
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.status.Branch == "main" && !w.busy })
	on(func() {
		// Declining to drop leaves it all alone.
		w.answer = false
		w.startRebase(base, steps)
		if w.busy || len(w.asked) != 1 {
			t.Errorf("busy %v, asked %q", w.busy, w.asked)
		}
		w.answer = true
		w.startRebase(base, steps)
	})
	waitFor(t, "the stop", func() bool { return !w.busy && w.status.Rebasing })
	on(func() {
		if !strings.Contains(w.where.Text(), "(rebasing 2 of 3)") || !strings.HasPrefix(w.said.Text(), "Stopped to edit a commit") {
			t.Errorf("status bar %q, %q", w.where.Text(), w.said.Text())
		}
		if !w.continueRebase.IsEnabled() || !w.abortRebase.IsEnabled() || !w.skipRebase.IsEnabled() {
			t.Error("can't go on with the rebase")
		}
		if len(w.failures) != 0 {
			t.Errorf("failures %q", w.failures)
		}
	})
	writeFile(t, dir, "b", "changed\n")
	on(w.rescanRepository)
	waitFor(t, "the change", func() bool { return w.idleWith(1, 0) })
	on(func() { w.stageChanges(w.status.Unstaged) })
	waitFor(t, "staging", func() bool { return w.idleWith(0, 1) })
	on(w.continueRebase.Trigger)
	waitFor(t, "the end", func() bool { return !w.busy && !w.status.Rebasing && w.idleWith(0, 0) })
	on(func() {
		if w.continueRebase.IsEnabled() || strings.Contains(w.where.Text(), "rebasing") {
			t.Errorf("still rebasing: %q", w.where.Text())
		}
		if !strings.HasPrefix(w.said.Text(), "Continue Rebase done.") {
			t.Errorf("said %q", w.said.Text())
		}
	})
	if got := logSubjects(t, dir); !slices.Equal(got, []string{"first", "a", "b"}) {
		t.Errorf("subjects %q", got)
	}
}

// TestRebaseConflictStops checks that a rebase stopped at a conflict says how to
// go on, and that aborting it puts the branch back.
func TestRebaseConflictStops(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "file", "two\n")
	runGit(t, dir, "commit", "-qam", "two")
	writeFile(t, dir, "file", "three\n")
	runGit(t, dir, "commit", "-qam", "three")
	d, base := newTestRebaseDialog(t, dir, "two")
	var steps []git.Step
	on(func() {
		d.moveRows([]int{1}, 0)
		steps = slices.Clone(d.steps)
	})
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.status.Branch == "main" && !w.busy })
	on(func() { w.startRebase(base, steps) })
	waitFor(t, "the stop", func() bool { return !w.busy && w.status.Rebasing && len(w.status.Unstaged) == 1 })
	on(func() {
		if len(w.failures) != 1 || !strings.Contains(w.failures[0], "CONFLICT") || !strings.Contains(w.failures[0], "Continue Rebase") {
			t.Errorf("failures %q", w.failures)
		}
		if w.said.Text() != "Interactive Rebase stopped" {
			t.Errorf("said %q", w.said.Text())
		}
		w.doAbortRebase()
	})
	waitFor(t, "the abort", func() bool { return !w.busy && !w.status.Rebasing })
	if got := logSubjects(t, dir); !slices.Equal(got, []string{"first", "two", "three"}) {
		t.Errorf("subjects %q", got)
	}
}

// TestRebaseNeedsACleanTree checks that a rebase is not offered over changes
// not yet committed, nor from the row standing for them.
func TestRebaseNeedsACleanTree(t *testing.T) {
	dir := newRepo(t)
	commitFiles(t, dir, "a")
	writeFile(t, dir, "a", "changed\n")
	w := newTestWindow(t, dir)
	waitFor(t, "the change", func() bool { return w.idleWith(1, 0) })
	on(func() {
		menu := w.commitMenu(git.UncommittedCommit(""))
		defer menu.DeleteLater()
		if actions := menu.Actions(); len(actions) != 1 || actions[0].IsEnabled() || actions[0].Text() != "&Rebase from here…" {
			t.Error("can rebase from the uncommitted changes, or the menu says otherwise")
		}
		head, _ := w.repo.Log(1, 0)
		w.rebaseFrom(head[0])
		if len(w.failures) != 1 || !strings.Contains(w.failures[0], "clean work tree") {
			t.Errorf("failures %q", w.failures)
		}
	})
}
