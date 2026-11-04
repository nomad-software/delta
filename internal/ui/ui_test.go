package ui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"

	"github.com/nomad-software/delta/internal/git"
	"github.com/nomad-software/delta/internal/settings"
)

// TestMain runs the tests on a goroutine of their own while Qt's event loop
// runs on the main thread, as it must.
func TestMain(m *testing.M) {
	os.Setenv("QT_QPA_PLATFORM", "offscreen")
	qt.NewQApplication(os.Args[:1])
	qt.QGuiApplication_SetQuitOnLastWindowClosed(false)
	code := 0
	go func() {
		code = m.Run()
		mainthread.Start(qt.QCoreApplication_Quit)
	}()
	qt.QApplication_Exec()
	os.Exit(code)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// runGitOutFails runs git in the folder where it is meant to fail, as a merge
// that stops with a conflict.
func runGitOutFails(dir string, args ...string) {
	exec.Command("git", append([]string{"-C", dir}, args...)...).Run()
}

// runGitOut runs git in the folder and returns what it wrote.
func runGitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// hasButton says whether a push button with the text is in the object, at any
// depth.
func hasButton(object *qt.QObject, text string) bool {
	for _, child := range object.Children() {
		if child.Inherits("QPushButton") && qt.UnsafeNewQPushButton(child.UnsafePointer()).Text() == text {
			return true
		}
		if hasButton(child, text) {
			return true
		}
	}
	return false
}

func writeFile(t *testing.T, dir, name, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newRepo returns a folder with a repository with one commit, of a file "file",
// isolated from the configuration of whoever runs the tests.
func newRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, who := range []string{"AUTHOR", "COMMITTER"} {
		t.Setenv("GIT_"+who+"_NAME", "Test")
		t.Setenv("GIT_"+who+"_EMAIL", "test@example.com")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	writeFile(t, dir, "file", "one\ntwo\nthree\n")
	runGit(t, dir, "add", "file")
	runGit(t, dir, "commit", "-m", "first")
	return dir
}

// testWindow is a window whose questions are answered, and failures noted.
type testWindow struct {
	*Window
	path     string
	answer   bool
	asked    []string
	failures []string
}

func newTestWindow(t *testing.T, repo string) *testWindow {
	t.Helper()
	return newTestWindowAt(t, repo, filepath.Join(t.TempDir(), "settings.json"))
}

// newTestWindowAt is newTestWindow with its settings kept at the path.
func newTestWindowAt(t *testing.T, repo, path string) *testWindow {
	t.Helper()
	tw := &testWindow{path: path, answer: true}
	mainthread.Wait(func() {
		tw.Window = NewWindow(tw.path)
		tw.confirm = func(_, text string) bool {
			tw.asked = append(tw.asked, text)
			return tw.answer
		}
		tw.failed = func(_, text string) { tw.failures = append(tw.failures, text) }
		tw.Start(repo)
	})
	// The repository is looked for in the background.
	waitFor(t, "the repository to be looked for", func() bool { return tw.finding == 0 })
	t.Cleanup(func() {
		// It is closed but not deleted, as the answer to a command still running
		// comes back to it, as it can in the program between closing and leaving.
		mainthread.Wait(func() { tw.Close() })
	})
	return tw
}

// on runs the function on the main thread.
func on(fn func()) { mainthread.Wait(fn) }

// mapPoint returns where the point x, y of the widget is in another, which holds
// it.
func mapPoint(widget, in *qt.QWidget, x, y int) (int, int) {
	point := qt.NewQPoint2(x, y)
	defer point.Delete()
	at := widget.MapTo2(in, point)
	return at.X(), at.Y()
}

// openRepository opens the repository that holds the folder at once, rather
// than looking for it in the background as the window does, dropping any it is
// looking for.
func (w *Window) openRepository(dir string) error {
	r, err := git.Open(dir)
	if err != nil {
		return err
	}
	w.useRepository(r)
	return nil
}

// waitFor waits until the condition, tested on the main thread, holds.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		var ok bool
		on(func() { ok = cond() })
		if ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting: %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// listTexts returns the texts of a list.
func listTexts(list *qt.QListWidget) []string {
	var texts []string
	for i := range list.Count() {
		texts = append(texts, list.Item(i).Text())
	}
	return texts
}

// listCodes returns the codes of changes the icons of a list's rows stand for,
// 0 for a blank.
func listCodes(list *qt.QListWidget) []byte {
	var codes []byte
	for i := range list.Count() {
		key := list.Item(i).Icon().CacheKey()
		code := byte('!')
		for c, icon := range changeIcons {
			if icon.CacheKey() == key {
				code = c
			}
		}
		codes = append(codes, code)
	}
	return codes
}

func (w *Window) idleWith(unstaged, staged int) bool {
	return !w.busy && w.changes.unstaged.Count() == unstaged && w.changes.staged.Count() == staged
}

func TestLayout(t *testing.T) {
	w := newTestWindow(t, newRepo(t))
	on(func() {
		// The toolbar is apemail's size, with its buttons in groups.
		var buttons []string
		for _, a := range w.toolbar.Actions() {
			if a.IsSeparator() {
				buttons = append(buttons, "|")
				continue
			}
			// The dropdowns after the buttons are not buttons.
			if !w.toolbar.WidgetForAction(a).Inherits("QToolButton") {
				continue
			}
			buttons = append(buttons, a.IconText())
			if width := w.toolbar.WidgetForAction(a).MinimumWidth(); width != toolButtonWidth {
				t.Errorf("%q is at least %d wide", a.IconText(), width)
			}
			// Its tooltip is never shown: the event asking for it is taken.
			button := w.toolbar.WidgetForAction(a)
			at := qt.NewQPoint2(button.Width()/2, button.Height()/2)
			tip := qt.NewQHelpEvent(qt.QEvent__ToolTip, at, button.MapToGlobalWithQPoint(at))
			if qt.QCoreApplication_SendEvent(button.QObject, tip.QEvent) && qt.QToolTip_IsVisible() {
				t.Errorf("%q shows a tooltip", a.IconText())
			}
			if !qt.QCoreApplication_SendEvent(button.QObject, tip.QEvent) {
				t.Errorf("%q's tooltip event was not taken", a.IconText())
			}
			tip.Delete()
			at.Delete()
			if a.Icon().IsNull() {
				t.Errorf("%q has no icon", a.IconText())
			}
		}
		if want := []string{"Open", "|", "New Branch", "Checkout", "Finish", "|", "Rescan", "|", "Pull", "Push", "|", "History"}; !slices.Equal(buttons, want) {
			t.Errorf("buttons %q", buttons)
		}
		if size := w.toolbar.IconSize(); size.Width() != 32 || size.Height() != 32 || w.toolbar.ToolButtonStyle() != qt.ToolButtonTextUnderIcon || w.toolbar.IsMovable() {
			t.Errorf("toolbar of %dx%d, style %v", size.Width(), size.Height(), w.toolbar.ToolButtonStyle())
		}
		// Push's arrow opens a menu to force the push, and only Push has one.
		for _, a := range w.toolbar.Actions() {
			if a.IsSeparator() || !w.toolbar.WidgetForAction(a).Inherits("QToolButton") {
				continue
			}
			button := qt.UnsafeNewQToolButton(w.toolbar.WidgetForAction(a).UnsafePointer())
			menu := button.Menu()
			if a.UnsafePointer() != w.pushButton.UnsafePointer() {
				if menu != nil {
					t.Errorf("%q has a menu", a.IconText())
				}
				continue
			}
			if button.PopupMode() != qt.QToolButton__MenuButtonPopup || menu == nil || len(menu.Actions()) != 1 || menu.Actions()[0].UnsafePointer() != w.forceToggle.UnsafePointer() {
				t.Errorf("Push's mode is %v, menu %v", button.PopupMode(), menu)
			}
		}
		if !w.showHistory.IsCheckable() || w.showHistory.IsChecked() || w.historyShown() {
			t.Error("the history is shown to start with, or can't be")
		}
		var menus []string
		for _, a := range w.MenuBar().Actions() {
			menus = append(menus, strings.ReplaceAll(a.Text(), "&", ""))
		}
		if want := []string{"File", "Branch", "Commit", "Stash", "Remote", "Help"}; !slices.Equal(menus, want) {
			t.Errorf("menus %v", menus)
		}
		// Staging is on S, as unstaging is on U.
		for a, want := range map[*qt.QAction]string{w.stageAll: "Ctrl+Shift+S", w.stageSelected: "Ctrl+S", w.unstageSelected: "Ctrl+U", w.unstageAll: "Ctrl+Shift+U"} {
			if got := a.Shortcut().ToString(); got != want {
				t.Errorf("%q is on %q, not %q", a.Text(), got, want)
			}
		}
		// Pushing is in the Remote menu, not the Commit menu.
		for a, want := range map[*qt.QAction][]string{w.push: {"Remote"}, w.forcePush: {"Remote"}} {
			var in []string
			for _, o := range a.AssociatedObjects() {
				if o.Inherits("QMenu") {
					in = append(in, strings.ReplaceAll(qt.UnsafeNewQMenu(o.UnsafePointer()).Title(), "&", ""))
				}
			}
			if !slices.Equal(in, want) {
				t.Errorf("%q is in the menus %q", a.Text(), in)
			}
		}
		for _, b := range []struct {
			button *qt.QPushButton
			text   string
		}{{w.changes.rescan, "Rescan"}, {w.changes.stage, "Stage Changes"}, {w.changes.commit, "Commit"}, {w.changes.push, "Push"}} {
			if b.button.Text() != b.text {
				t.Errorf("button %q", b.button.Text())
			}
		}
		if w.changes.lists.Widget(0).IsAncestorOf(w.changes.staged.QWidget) || !w.changes.lists.Widget(0).IsAncestorOf(w.changes.unstaged.QWidget) {
			t.Error("the unstaged list is not above the staged one")
		}
	})
}

func TestPanesLook(t *testing.T) {
	w := newTestWindow(t, newRepo(t))
	on(func() {
		for _, label := range []*qt.QLabel{w.changes.unstagedLabel, w.changes.stagedLabel, w.changes.diffLabel} {
			if !label.Font().Bold() {
				t.Errorf("%q is not bold", label.Text())
			}
		}
		// Only dots are drawn on a handle, at its centre.
		handle := w.changes.sidebar.Handle(1)
		if w.changes.sidebar.HandleWidth() != paneHandleWidth || handle.Width() != paneHandleWidth {
			t.Errorf("handle is %d wide", handle.Width())
		}
		image := handle.Grab().ToImage()
		window := w.Palette().ColorWithCr(qt.QPalette__Window).Rgb()
		cx, cy := image.Width()/2, image.Height()/2
		if image.Pixel(0, cy) != window {
			t.Error("the edge of the handle is painted")
		}
		if image.Pixel(cx, cy) == window {
			t.Error("no dot at the centre of the handle")
		}
	})
}

func TestStatusBar(t *testing.T) {
	w := newTestWindow(t, newRepo(t))
	waitFor(t, "the branch", func() bool { return w.where.Text() == "main" })
	on(func() {
		bar := w.StatusBar()
		if !bar.IsVisible() || !bar.IsSizeGripEnabled() {
			t.Errorf("visible %v, grip %v", bar.IsVisible(), bar.IsSizeGripEnabled())
		}
		w.say("A message")
		bar.Layout().Activate()
		for _, label := range []*qt.QLabel{w.where, w.said} {
			effect := qt.UnsafeNewQGraphicsOpacityEffect(label.GraphicsEffect().UnsafePointer())
			if effect.Opacity() != statusOpacity {
				t.Errorf("%q has opacity %v", label.Text(), effect.Opacity())
			}
		}
		if !w.where.IsVisible() || !w.said.IsVisible() {
			t.Error("a message hid the text beside it")
		}
		if w.where.X() > bar.Width()/4 || w.said.X() < w.where.X()+w.where.Width() {
			t.Errorf("branch at %d, message at %d of %d", w.where.X(), w.said.X(), bar.Width())
		}
		if w.said.Alignment()&qt.AlignRight != 0 {
			t.Error("the message is aligned right")
		}
	})
}

func TestStageCommitFlow(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "file", "one\nTWO\nthree\n")
	writeFile(t, dir, "new", "new\n")
	w := newTestWindow(t, dir)
	waitFor(t, "two unstaged files", func() bool { return w.idleWith(2, 0) })

	on(func() {
		if got := listTexts(w.changes.unstaged); !slices.Equal(got, []string{"file", "new"}) || !slices.Equal(listCodes(w.changes.unstaged), []byte{'M', git.Untracked}) {
			t.Errorf("unstaged %q", got)
		}
		if w.commit.IsEnabled() {
			t.Error("commit enabled with nothing staged")
		}
		// Choosing a file shows its diff.
		w.changes.unstaged.SetCurrentRow(0)
	})
	waitFor(t, "the diff", func() bool { return strings.Contains(w.changes.diff.parsed.Text, "+TWO") })
	on(func() {
		if w.changes.diffLabel.Text() != "Unstaged: file" {
			t.Errorf("label %q", w.changes.diffLabel.Text())
		}
		// The button stages all that is listed.
		w.changes.stage.Click()
	})
	waitFor(t, "all staged", func() bool { return w.idleWith(0, 2) })

	// Choosing a staged file shows the staged diff. Its selection survived.
	on(func() {
		w.changes.staged.SetCurrentRow(1)
	})
	waitFor(t, "the staged diff", func() bool { return w.changes.diffLabel.Text() == "Staged: new" })

	// Selected files are unstaged, and staged again, alone.
	on(func() { w.unstageSelected.Trigger() })
	waitFor(t, "one unstaged", func() bool { return w.idleWith(1, 1) })
	on(func() {
		w.changes.unstaged.SetCurrentRow(0)
		w.stageSelected.Trigger()
	})
	waitFor(t, "both staged", func() bool { return w.idleWith(0, 2) })

	on(func() {
		w.changes.message.SetPlainText("  ")
		if w.changes.commit.IsEnabled() {
			t.Error("commit enabled with a blank message")
		}
		w.changes.message.SetPlainText("A message")
		if !w.changes.commit.IsEnabled() {
			t.Error("commit disabled")
		}
		w.changes.commit.Click()
	})
	waitFor(t, "the commit", func() bool { return w.idleWith(0, 0) })
	on(func() {
		if w.changes.message.ToPlainText() != "" {
			t.Error("the message was not cleared")
		}
		if len(w.failures) != 0 {
			t.Errorf("failures %v", w.failures)
		}
	})
	out, _ := exec.Command("git", "-C", dir, "log", "-1", "--format=%s").Output()
	if strings.TrimSpace(string(out)) != "A message" {
		t.Errorf("last commit %q", out)
	}
}

func TestStageButtonUnstagesWhenAllStaged(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "file", "changed\n")
	writeFile(t, dir, "new", "new\n")
	w := newTestWindow(t, dir)
	waitFor(t, "two unstaged", func() bool { return w.idleWith(2, 0) })
	on(func() {
		if w.changes.stage.Text() != stageText || !w.changes.stage.IsEnabled() {
			t.Errorf("button %q, enabled %v", w.changes.stage.Text(), w.changes.stage.IsEnabled())
		}
		w.changes.stage.Click()
	})
	waitFor(t, "all staged", func() bool { return w.idleWith(0, 2) })
	on(func() {
		if w.changes.stage.Text() != unstageText || !w.changes.stage.IsEnabled() {
			t.Errorf("button %q, enabled %v", w.changes.stage.Text(), w.changes.stage.IsEnabled())
		}
		w.changes.stage.Click()
	})
	waitFor(t, "all unstaged", func() bool { return w.idleWith(2, 0) })

	// With some of each it stages the rest.
	on(func() {
		w.changes.unstaged.SetCurrentRow(0)
		w.stageSelected.Trigger()
	})
	waitFor(t, "one of each", func() bool { return w.idleWith(1, 1) })
	on(func() {
		if w.changes.stage.Text() != stageText {
			t.Errorf("button %q", w.changes.stage.Text())
		}
		w.changes.stage.Click()
	})
	waitFor(t, "all staged again", func() bool { return w.idleWith(0, 2) })

	// With nothing changed there is nothing for it to do.
	on(func() {
		w.changes.message.SetPlainText("Message")
		w.commit.Trigger()
	})
	waitFor(t, "the commit", func() bool { return w.idleWith(0, 0) })
	on(func() {
		if w.changes.stage.Text() != stageText || w.changes.stage.IsEnabled() {
			t.Errorf("button %q, enabled %v", w.changes.stage.Text(), w.changes.stage.IsEnabled())
		}
	})
}

func TestFailedCommitKeepsMessage(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "file", "changed\n")
	hook := filepath.Join(dir, ".git", "hooks", "pre-commit")
	os.MkdirAll(filepath.Dir(hook), 0o755)
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho hook says no >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	w := newTestWindow(t, dir)
	waitFor(t, "one unstaged", func() bool { return w.idleWith(1, 0) })
	on(func() { w.stageAll.Trigger() })
	waitFor(t, "one staged", func() bool { return w.idleWith(0, 1) })
	on(func() {
		w.changes.message.SetPlainText("Keep me")
		w.commit.Trigger()
	})
	waitFor(t, "the failure", func() bool { return len(w.failures) == 1 && !w.busy })
	on(func() {
		if !strings.Contains(w.failures[0], "hook says no") {
			t.Errorf("failure %q", w.failures[0])
		}
		if w.changes.message.ToPlainText() != "Keep me" {
			t.Error("the message was lost")
		}
	})
}

func TestForcePush(t *testing.T) {
	dir := newRepo(t)
	bare := t.TempDir()
	runGit(t, bare, "init", "--bare", "-b", "main")
	runGit(t, dir, "remote", "add", "origin", bare)

	w := newTestWindow(t, dir)
	waitFor(t, "the branch", func() bool { return w.status.Branch == "main" })

	// The first push sets the upstream.
	on(func() { w.changes.push.Click() })
	waitFor(t, "the upstream", func() bool { return w.status.Upstream == "origin/main" })

	writeFile(t, dir, "file", "rewritten\n")
	runGit(t, dir, "commit", "-a", "--amend", "-m", "rewritten")
	on(func() { w.rescan.Trigger() })
	waitFor(t, "diverged", func() bool { return w.status.Behind == 1 })

	// Without force it is refused.
	on(func() { w.changes.push.Click() })
	waitFor(t, "the refusal", func() bool { return len(w.failures) == 1 && !w.busy })

	// The option of the toolbar's Push ticks the checkbox, and both buttons
	// say they force. Saying no does nothing.
	on(func() {
		w.forceToggle.Trigger()
		if !w.changes.force.IsChecked() || w.changes.push.Text() != "Force Push" || w.pushButton.IconText() != "Force Push" {
			t.Errorf("checkbox %v, buttons %q, %q", w.changes.force.IsChecked(), w.changes.push.Text(), w.pushButton.IconText())
		}
		w.answer = false
		w.changes.push.Click()
		if w.busy || len(w.asked) != 1 {
			t.Errorf("busy %v, asked %v", w.busy, w.asked)
		}
		// Unticking the checkbox unticks the option, and ticking it again
		// ticks it.
		w.changes.force.SetChecked(false)
		if w.forceToggle.IsChecked() || w.pushButton.IconText() != "Push" {
			t.Errorf("option %v, toolbar %q", w.forceToggle.IsChecked(), w.pushButton.IconText())
		}
		w.changes.force.SetChecked(true)
		if !w.forceToggle.IsChecked() {
			t.Error("the option is not ticked with the checkbox")
		}
		// The Remote menu's Push, on Ctrl+P, never forces: it is refused, as
		// the branch has diverged, without asking.
		w.push.Trigger()
	})
	waitFor(t, "the menu's push to be refused", func() bool { return !w.busy && len(w.failures) == 2 })
	on(func() {
		if len(w.asked) != 1 || !w.changes.force.IsChecked() {
			t.Errorf("asked %q, force ticked %v", w.asked, w.changes.force.IsChecked())
		}
		w.answer = true
		w.pushButton.Trigger()
	})
	waitFor(t, "the force push", func() bool { return !w.busy && w.status.Behind == 0 && w.status.Ahead == 0 })
	on(func() {
		if w.changes.force.IsChecked() || w.forceToggle.IsChecked() || w.changes.push.Text() != "Push" || w.pushButton.IconText() != "Push" {
			t.Error("force is still on after pushing")
		}
		if len(w.asked) != 2 {
			t.Errorf("asked %q", w.asked)
		}
	})
}

// TestForcePushIsRemembered checks that whether Force push is ticked is saved
// on closing, and ticked again on starting, with the toolbar's option and the
// buttons saying so.
func TestForcePushIsRemembered(t *testing.T) {
	dir := newRepo(t)
	w := newTestWindow(t, dir)
	on(func() {
		if w.changes.force.IsChecked() || w.forceToggle.IsChecked() {
			t.Error("force push is ticked to start with")
		}
		w.forceToggle.Trigger()
		w.Close()
	})
	if !settings.Load(w.path).ForcePush {
		t.Fatal("force push was not saved")
	}
	again := newTestWindowAt(t, dir, w.path)
	on(func() {
		if !again.changes.force.IsChecked() || !again.forceToggle.IsChecked() || again.pushButton.IconText() != "Force Push" || again.changes.push.Text() != "Force Push" {
			t.Errorf("checkbox %v, option %v, buttons %q %q", again.changes.force.IsChecked(), again.forceToggle.IsChecked(), again.pushButton.IconText(), again.changes.push.Text())
		}
		again.changes.force.SetChecked(false)
		again.Close()
	})
	if settings.Load(w.path).ForcePush {
		t.Error("unticking force push was not saved")
	}
}

// TestDiscardAllChanges checks that discarding asks first, saying what will be
// lost, does nothing if declined, and else leaves the branch as committed.
func TestDiscardAllChanges(t *testing.T) {
	dir := newRepo(t)
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) && w.status.Branch == "main" })
	on(func() {
		if w.discard.IsEnabled() {
			t.Error("can discard with nothing changed")
		}
	})
	writeFile(t, dir, "file", "changed\n")
	writeFile(t, dir, "staged", "staged\n")
	runGit(t, dir, "add", "staged")
	writeFile(t, dir, "untracked", "new\n")
	on(w.rescanRepository)
	waitFor(t, "the changes", func() bool { return w.idleWith(2, 1) })
	on(func() {
		if !w.discard.IsEnabled() {
			t.Fatal("can't discard")
		}
		w.answer = false
		w.discard.Trigger()
		if w.busy || len(w.asked) != 1 {
			t.Fatalf("busy %v, asked %q", w.busy, w.asked)
		}
		want := "Discard all changes to main?\n\n1 changed file will be put back as last committed, and 2 new files will be deleted. Files ignored by git are kept.\n\nThis can't be undone."
		if w.asked[0] != want {
			t.Errorf("asked %q", w.asked[0])
		}
	})
	if _, err := os.Stat(filepath.Join(dir, "untracked")); err != nil {
		t.Fatal("declining deleted a file")
	}
	on(func() {
		w.answer = true
		w.discard.Trigger()
	})
	waitFor(t, "the discard", func() bool { return !w.busy && w.idleWith(0, 0) })
	if data, _ := os.ReadFile(filepath.Join(dir, "file")); string(data) != "one\ntwo\nthree\n" {
		t.Errorf("file is %q", data)
	}
	on(func() {
		if w.discard.IsEnabled() {
			t.Error("can discard again with nothing changed")
		}
	})
}

// TestHistoryPageSizeSetting checks that the history reads as many commits at a
// time as the settings say, from starting and once changed.
func TestHistoryPageSizeSetting(t *testing.T) {
	dir := historyRepo(t, 5)
	path := filepath.Join(t.TempDir(), "settings.json")
	s := settings.Defaults()
	s.HistoryPageSize = 50
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	w := newTestWindowAt(t, dir, path)
	on(func() {
		if w.history.pageSize != 50 {
			t.Errorf("reads %d at a time", w.history.pageSize)
		}
		s.HistoryPageSize = 700
		w.applySettings(s)
		if w.history.pageSize != 700 || settings.Load(path).HistoryPageSize != 700 {
			t.Errorf("reads %d at a time once changed, saved %d", w.history.pageSize, settings.Load(path).HistoryPageSize)
		}
	})
}

func TestBranchOperations(t *testing.T) {
	dir := newRepo(t)
	w := newTestWindow(t, dir)
	waitFor(t, "the branch", func() bool { return w.status.Branch == "main" })
	on(func() { w.createBranch("topic", "", true) })
	waitFor(t, "topic", func() bool { return w.status.Branch == "topic" && !w.busy })
	on(func() {
		if title := w.WindowTitle(); !strings.Contains(title, "(topic)") {
			t.Errorf("title %q", title)
		}
		w.checkout("main", false)
	})
	waitFor(t, "main", func() bool { return w.status.Branch == "main" && !w.busy })

	// A branch with commits of its own is deleted only after a second answer.
	runGit(t, dir, "switch", "topic")
	writeFile(t, dir, "file", "work\n")
	runGit(t, dir, "commit", "-am", "work")
	runGit(t, dir, "switch", "main")
	on(func() {
		w.answer = false
		w.removeBranch("topic")
		w.answer = true
		w.removeBranch("topic")
	})
	waitFor(t, "the deletion", func() bool {
		local, _, _ := w.repo.Branches()
		return !w.busy && !slices.Contains(local, "topic")
	})
	on(func() {
		if len(w.asked) != 3 {
			t.Errorf("asked %q", w.asked)
		}
	})
}

// TestDeletingABranchRunsInTheBackground checks that git is not run on the main
// thread, which would hold up the window, and that nothing else can start
// meanwhile. A branch with nothing of its own is deleted after one answer.
// TestCheckoutDialog checks that every local branch is offered, in order of
// name, with the current one chosen, which can't be checked out again, and the
// remote ones after them, in order too, only once asked for, which fetches
// first, keeping the branch chosen. A repository with
// one branch has it in the list, which was empty when the current branch was
// left out.
func TestCheckoutDialog(t *testing.T) {
	texts := func(c *qt.QComboBox) []string {
		var all []string
		for i := range c.Count() {
			all = append(all, c.ItemText(i))
		}
		return all
	}
	fetches := 0
	fetch := func() ([]string, error) {
		fetches++
		return []string{"origin/topic", "origin/Beta", "origin/main"}, nil
	}
	var d *formDialog
	var branch *qt.QComboBox
	var withRemote *qt.QCheckBox
	on(func() {
		d, branch, withRemote = newCheckoutDialog(nil, "main", []string{"topic", "main", "Zed", "apple"}, []string{"origin/old"}, fetch)
		if withRemote.Text() != "Include remote branches" || withRemote.IsChecked() {
			t.Fatal("no checkbox for the remote branches, or it starts ticked")
		}
		if got := texts(branch); !slices.Equal(got, []string{"apple", "main", "topic", "Zed"}) || branch.CurrentText() != "main" || d.ok.IsEnabled() {
			t.Errorf("local %q, chosen %q, OK %v", got, branch.CurrentText(), d.ok.IsEnabled())
		}
		branch.SetCurrentIndex(2)
		if !d.ok.IsEnabled() {
			t.Error("can't check out another branch")
		}
		withRemote.SetChecked(true)
		if branch.IsEnabled() || d.ok.IsEnabled() {
			t.Error("the list can be used while fetching")
		}
	})
	waitFor(t, "the fetch", func() bool { return branch.IsEnabled() })
	on(func() {
		if got := texts(branch); !slices.Equal(got, []string{"apple", "main", "topic", "Zed", "origin/Beta", "origin/main", "origin/topic"}) || branch.CurrentText() != "topic" {
			t.Errorf("with remote %q, chosen %q", got, branch.CurrentText())
		}
		branch.SetCurrentIndex(5)
		withRemote.SetChecked(false)
		if got := branch.CurrentText(); got != "main" || d.ok.IsEnabled() {
			t.Errorf("chose %q once the remote ones went, OK %v", got, d.ok.IsEnabled())
		}
		// Ticking it again lists what was fetched, without fetching again.
		withRemote.SetChecked(true)
		if got := texts(branch); len(got) != 7 || fetches != 1 {
			t.Errorf("again %q after %d fetches", got, fetches)
		}
		d.DeleteLater()

		// The only branch is there, though there is nothing else to choose.
		one, only, _ := newCheckoutDialog(nil, "master", []string{"master"}, nil, fetch)
		defer one.DeleteLater()
		if got := texts(only); !slices.Equal(got, []string{"master"}) || one.ok.IsEnabled() {
			t.Errorf("one branch: %q, OK %v", got, one.ok.IsEnabled())
		}
	})

	// A failed fetch says so, and lists the remote branches already known.
	var failing *qt.QComboBox
	var failingBox *qt.QCheckBox
	on(func() {
		var fd *formDialog
		fd, failing, failingBox = newCheckoutDialog(nil, "main", []string{"main"}, []string{"origin/old"}, func() ([]string, error) {
			return nil, errors.New("could not resolve host")
		})
		t.Cleanup(func() { on(fd.DeleteLater) })
		failingBox.SetChecked(true)
	})
	waitFor(t, "the failed fetch", func() bool { return failing.IsEnabled() })
	on(func() {
		if got := texts(failing); !slices.Equal(got, []string{"main", "origin/old"}) {
			t.Errorf("after a failed fetch %q", got)
		}
	})
}

// TestCheckoutFetch checks that the checkout dialog's fetch, run in the
// background, gives the remote branches, has the progress bar show it, and notes
// that it fetched on the main thread, where the window reads it, perhaps while
// the fetch is still going on: noted in the background, it was a race, which
// go test -race finds.
func TestCheckoutFetch(t *testing.T) {
	dir := newRepo(t)
	bare := t.TempDir()
	runGit(t, bare, "init", "-q", "--bare", "-b", "main")
	runGit(t, dir, "remote", "add", "origin", bare)
	runGit(t, dir, "push", "-q", "origin", "main")
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.status.Branch == "main" && w.working == 0 })
	fetched := false
	var fetch func() ([]string, error)
	on(func() { fetch = w.checkoutFetch(&fetched) })
	type answer struct {
		remote []string
		err    error
	}
	answers := make(chan answer, 1)
	go func() {
		remote, err := fetch()
		answers <- answer{remote, err}
	}()
	// The main thread reads it while the fetch may still be going on.
	waitFor(t, "the fetch to be noted", func() bool { return fetched })
	got := <-answers
	if got.err != nil || !slices.Equal(got.remote, []string{"origin/main"}) {
		t.Errorf("remote %q, %v", got.remote, got.err)
	}
	waitFor(t, "the count of work", func() bool { return w.working == 0 })
}

// TestFirstTextFieldHasTheFocus checks that a dialog opens with the cursor in
// its first field of text, as in New Branch's name, not on OK, which the focus
// went to as it is made first, nor on a list before it, as in Rename Branch.
func TestFirstTextFieldHasTheFocus(t *testing.T) {
	var d *formDialog
	var name *qt.QLineEdit
	on(func() {
		d = newFormDialog(nil, "Rename Branch")
		d.addChoice("Branch", []string{"main", "topic"})
		name = d.addText("New name", "", notEmpty)
		d.addText("Another", "", notEmpty)
		d.Show()
		d.ActivateWindow()
	})
	t.Cleanup(func() { on(func() { d.Hide(); d.DeleteLater() }) })
	waitFor(t, "the dialog to be active", func() bool { return d.IsActiveWindow() })
	waitFor(t, "the name to have the focus", name.HasFocus)
}

// TestCheckoutFocusesTheBranches checks that the checkout dialog opens with the
// list of branches focused, to choose one by keyboard straight away.
func TestCheckoutFocusesTheBranches(t *testing.T) {
	var d *formDialog
	var branch *qt.QComboBox
	on(func() {
		d, branch, _ = newCheckoutDialog(nil, "main", []string{"main", "topic"}, nil, nil)
		d.Show()
		d.ActivateWindow()
	})
	t.Cleanup(func() { on(func() { d.Hide(); d.DeleteLater() }) })
	waitFor(t, "the dialog to be active", func() bool { return d.IsActiveWindow() })
	waitFor(t, "the branches to have the focus", branch.HasFocus)
}

// TestFormFieldsFillTheDialog checks that a dialog's fields are as wide as it
// is, as a list of branches was too short on a style that keeps them narrow.
func TestFormFieldsFillTheDialog(t *testing.T) {
	on(func() {
		d, branch, _ := newCheckoutDialog(nil, "main", []string{"main"}, nil, nil)
		defer d.DeleteLater()
		d.Show()
		if policy := d.form.FieldGrowthPolicy(); policy != qt.QFormLayout__AllNonFixedFieldsGrow {
			t.Errorf("fields grow by %v", policy)
		}
		if branch.Width() < d.Width()/2 {
			t.Errorf("the list is %d wide in a dialog %d wide", branch.Width(), d.Width())
		}
		d.Hide()
	})
}

func TestDeletingABranchRunsInTheBackground(t *testing.T) {
	dir := newRepo(t)
	runGit(t, dir, "branch", "topic")
	w := newTestWindow(t, dir)
	waitFor(t, "the branch", func() bool { return w.status.Branch == "main" && !w.busy })
	on(func() {
		w.answer = true
		w.removeBranch("topic")
		if !w.busy || w.repoOnly[0].IsEnabled() {
			t.Errorf("busy %v, actions enabled %v, straight after", w.busy, w.repoOnly[0].IsEnabled())
		}
	})
	waitFor(t, "the deletion", func() bool {
		local, _, _ := w.repo.Branches()
		return !w.busy && !slices.Contains(local, "topic")
	})
	on(func() {
		if len(w.asked) != 1 {
			t.Errorf("asked %q", w.asked)
		}
		if got := w.said.Text(); got != "Deleted the branch topic" {
			t.Errorf("said %q", got)
		}
	})
}

// TestKeepingAnUnmergedBranch checks that declining the second question, once git
// has refused, keeps the branch and says so.
func TestKeepingAnUnmergedBranch(t *testing.T) {
	dir := newRepo(t)
	runGit(t, dir, "switch", "-c", "topic")
	writeFile(t, dir, "file", "work\n")
	runGit(t, dir, "commit", "-am", "work")
	runGit(t, dir, "switch", "main")
	w := newTestWindow(t, dir)
	waitFor(t, "the branch", func() bool { return w.status.Branch == "main" && !w.busy })
	var answers int
	on(func() {
		w.confirm = func(string, string) bool {
			answers++
			return answers == 1
		}
		w.removeBranch("topic")
	})
	waitFor(t, "the second question", func() bool { return answers == 2 && !w.busy })
	on(func() {
		local, _, _ := w.repo.Branches()
		if !slices.Contains(local, "topic") {
			t.Error("the branch is gone")
		}
		if got := w.said.Text(); got != "Kept the branch topic" {
			t.Errorf("said %q", got)
		}
	})
}

func TestMergePrefillsMessage(t *testing.T) {
	dir := newRepo(t)
	runGit(t, dir, "switch", "-c", "topic")
	writeFile(t, dir, "file", "theirs\n")
	runGit(t, dir, "commit", "-am", "theirs")
	runGit(t, dir, "switch", "main")
	writeFile(t, dir, "file", "ours\n")
	runGit(t, dir, "commit", "-am", "ours")
	exec.Command("git", "-C", dir, "merge", "topic").Run()

	w := newTestWindow(t, dir)
	waitFor(t, "the merge", func() bool { return w.status.Merging })
	waitFor(t, "the message", func() bool { return strings.Contains(w.changes.message.ToPlainText(), "Merge branch 'topic'") })
	on(func() {
		if !w.abortMerge.IsEnabled() {
			t.Error("abort is disabled")
		}
		if got := listTexts(w.changes.unstaged); !slices.Equal(got, []string{"file"}) || !slices.Equal(listCodes(w.changes.unstaged), []byte{git.Conflict}) {
			t.Errorf("unstaged %q", got)
		}
		w.changes.unstaged.SetCurrentRow(0)
	})
	waitFor(t, "the conflict's diff", func() bool { return strings.Contains(w.changes.diff.parsed.Text, "<<<<<<<") })
	on(func() { w.abortMerge.Trigger() })
	waitFor(t, "the abort", func() bool { return !w.status.Merging && !w.busy })
	on(func() {
		if w.changes.message.ToPlainText() != "" {
			t.Error("the message was left behind")
		}
	})
}

// colourOnLine returns the colour of the text of a line of the diff, at the
// offset, as a colour name, or "" for none of its own.
func colourOnLine(v *diffView, line, offset int) string {
	cursor := qt.NewQTextCursor2(v.Document())
	defer cursor.Delete()
	cursor.SetPosition(v.Document().FindBlockByNumber(line).Position() + offset)
	format := cursor.CharFormat()
	if format.Foreground().Style() == qt.NoBrush {
		return ""
	}
	return format.Foreground().Color().Name()
}

func TestDiffColours(t *testing.T) {
	on(func() {
		v := newDiffView("#00ff00", "#ff0000", "#808080")
		defer v.Delete()
		v.show("diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1 +1 @@ func\n-old\n+new\n same\n")
		if !strings.HasPrefix(v.ToPlainText(), "@@ -1 +1 @@") {
			t.Errorf("the header is shown: %q", v.ToPlainText())
		}
		// The text is coloured as the settings have it, end to end.
		for _, offset := range []int{1, 4} {
			if got := colourOnLine(v, 2, offset); got != "#00ff00" {
				t.Errorf("added line %s at %d", got, offset)
			}
			if got := colourOnLine(v, 1, offset); got != "#ff0000" {
				t.Errorf("removed line %s at %d", got, offset)
			}
		}
		if got := colourOnLine(v, 3, 2); got != "" {
			t.Errorf("unchanged line %s", got)
		}
		// A hunk's range is in its colour, and what follows it as usual.
		if got := colourOnLine(v, 0, 5); got != "#808080" {
			t.Errorf("range is %s", got)
		}
		if got := colourOnLine(v, 0, len("@@ -1 +1 @@ fu")); got != "" {
			t.Errorf("text after the range is %s", got)
		}
		// No line has a background.
		for line := range v.Document().BlockCount() {
			if style := v.Document().FindBlockByNumber(line).BlockFormat().Background().Style(); style != qt.NoBrush {
				t.Errorf("line %d has a background of style %v", line, style)
			}
		}

		// A change of colour shows at once, with the text as it was.
		v.setColours("#0000ff", "#ff0000", "#123456")
		if got := colourOnLine(v, 2, 1); got != "#0000ff" {
			t.Errorf("recoloured line %s", got)
		}
		if got := colourOnLine(v, 0, 5); got != "#123456" {
			t.Errorf("recoloured range %s", got)
		}
		if !strings.Contains(v.ToPlainText(), "+new") {
			t.Error("the text was lost")
		}
	})
}

func TestLargeDiffDrawsQuickly(t *testing.T) {
	var b strings.Builder
	b.WriteString("diff --git a/f b/f\n@@ -1 +1 @@\n")
	for i := range 100000 {
		fmt.Fprintf(&b, "+line %d of a long new file\n", i)
	}
	on(func() {
		v := newDiffView("#00ff00", "#ff0000", "#808080")
		defer v.Delete()
		start := time.Now()
		v.show(b.String())
		took := time.Since(start)
		t.Logf("100000 lines drawn in %v", took)
		if took > 5*time.Second {
			t.Errorf("took %v", took)
		}
	})
}

func TestThemes(t *testing.T) {
	on(func() {
		desktop := qt.QApplication_Style().Name()
		base := func() string { return qt.QGuiApplication_Palette().ColorWithCr(qt.QPalette__Base).Name() }
		applyTheme(settings.ThemeDark)
		if base() != "#1f1f1f" || !strings.EqualFold(qt.QApplication_Style().Name(), "Fusion") {
			t.Errorf("dark: %s, %s", base(), qt.QApplication_Style().Name())
		}
		applyTheme(settings.ThemeLight)
		if base() != "#ffffff" {
			t.Errorf("light: %s", base())
		}
		applyTheme("")
		if qt.QApplication_Style().Name() != desktop {
			t.Errorf("style %s after following the system, not %s", qt.QApplication_Style().Name(), desktop)
		}
	})
}

func TestCodeFontIsApplied(t *testing.T) {
	w := newTestWindow(t, newRepo(t))
	on(func() {
		s := w.config
		s.CodeFont, s.CodeFontSize = "Some Font", 21
		w.applySettings(s)
		for _, widget := range []*qt.QWidget{w.changes.diff.QWidget, w.history.diff.QWidget, w.changes.message.QWidget} {
			font := widget.Font()
			if font.Family() != "Some Font" || font.PointSize() != 21 {
				t.Errorf("font %q, %d", font.Family(), font.PointSize())
			}
		}
		want := float64(tabWidth * w.changes.diff.FontMetrics().HorizontalAdvance(" "))
		if got := w.changes.diff.TabStopDistance(); got != want {
			t.Errorf("tab is %v wide, not %v", got, want)
		}
	})
	got := settings.Load(w.path)
	if got.CodeFont != "Some Font" || got.CodeFontSize != 21 {
		t.Errorf("saved %q, %d", got.CodeFont, got.CodeFontSize)
	}
	// A new window opens with it.
	again := newTestWindowAt(t, "", w.path)
	on(func() {
		if font := again.changes.diff.Font(); font.Family() != "Some Font" || font.PointSize() != 21 {
			t.Errorf("reopened with %q, %d", font.Family(), font.PointSize())
		}
	})
}

// testHistoryMenu checks that the Branch menu's Show History, on Ctrl+H, is the
// toolbar's History, and is ticked while the history is shown.
func testHistoryMenu(t *testing.T) {
	dir := newRepo(t)
	w := newTestWindow(t, dir)
	on(func() {
		var in []string
		for _, o := range w.showHistory.AssociatedObjects() {
			if o.Inherits("QMenu") {
				in = append(in, strings.ReplaceAll(qt.UnsafeNewQMenu(o.UnsafePointer()).Title(), "&", ""))
			}
		}
		if !slices.Contains(in, "Branch") || w.showHistory.Text() != "Show &History" || w.showHistory.Shortcut().ToString() != "Ctrl+H" {
			t.Errorf("in %q as %q on %q", in, w.showHistory.Text(), w.showHistory.Shortcut().ToString())
		}
		w.showHistory.Trigger()
		if !w.historyShown() || !w.showHistory.IsChecked() {
			t.Error("Show History did not show the history")
		}
		w.showHistory.Trigger()
		if w.historyShown() {
			t.Error("Show History did not hide the history")
		}
	})
}

// TestHistoryButton checks that the History button shows the history in place
// of the changes, and the changes again.
func TestHistoryButton(t *testing.T) {
	t.Run("menu", testHistoryMenu)
	dir := newRepo(t)
	writeFile(t, dir, "file", "changed\n")
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.idleWith(1, 0) })
	on(func() {
		w.showHistory.Trigger()
		if !w.historyShown() || !w.history.page.IsVisible() || w.changes.page.IsVisible() {
			t.Errorf("history shown %v", w.historyShown())
		}
	})
	waitFor(t, "the commits", func() bool { return w.history.commits.TopLevelItemCount() == 2 })
	on(func() {
		w.showHistory.Trigger()
		if w.historyShown() || !w.changes.page.IsVisible() {
			t.Errorf("history still shown %v", w.historyShown())
		}
	})
}

func TestSizesAreSaved(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "file", "changed\n")
	w := newTestWindow(t, dir)
	on(func() {
		w.Resize(1000, 700)
		w.changes.sidebar.SetSizes([]int{222, 760})
		w.changes.lists.SetSizes([]int{111, 400})
		w.changes.editor.SetSizes([]int{333, 250})
	})
	// The event loop has its turn to lay them out.
	waitFor(t, "the size", func() bool { return w.Width() == 1000 })
	var want settings.Settings
	on(func() {
		want.Sidebar, want.Lists, want.Editor = panes(w.changes.sidebar), panes(w.changes.lists), panes(w.changes.editor)
		w.Close()
	})
	got := settings.Load(w.path)
	if got.Width != 1000 || got.Height != 700 {
		t.Errorf("window %dx%d", got.Width, got.Height)
	}
	if got.Sidebar != want.Sidebar || got.Lists != want.Lists || got.Editor != want.Editor {
		t.Errorf("panes %v %v %v, want %v %v %v", got.Sidebar, got.Lists, got.Editor, want.Sidebar, want.Lists, want.Editor)
	}
	if got.Sidebar[0] != 222 {
		t.Errorf("sidebar %v", got.Sidebar)
	}

	// A new window opens with them.
	again := newTestWindowAt(t, dir, w.path)
	waitFor(t, "the layout", func() bool { return again.changes.sidebar.Sizes()[0] != 0 })
	on(func() {
		if again.Width() != 1000 || panes(again.changes.sidebar)[0] != 222 {
			t.Errorf("reopened at %d wide, sidebar %v", again.Width(), panes(again.changes.sidebar))
		}
	})
}

// TestNoRepository checks that started outside a repository, with none named,
// none is opened, not even one a settings file of an older version remembers.
func TestNoRepository(t *testing.T) {
	outside := t.TempDir()
	remembered := newRepo(t)
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, filepath.Dir(path), "settings.json", fmt.Sprintf(`{"repository": %q}`, remembered))
	t.Chdir(outside)
	w := newTestWindowAt(t, "", path)
	on(func() {
		if w.repo != nil {
			t.Fatalf("opened %q", w.repo.Root)
		}
		if w.rescan.IsEnabled() || w.push.IsEnabled() || w.changes.commit.IsEnabled() {
			t.Error("actions are enabled with no repository")
		}
		if err := w.openRepository(outside); err == nil {
			t.Error("a folder that is no repository was opened")
		}
	})
}

// screenshotRepo returns a repository with branches and merges to draw.
func screenshotRepo(t *testing.T) string {
	t.Helper()
	dir := newRepo(t)
	commit := func(name, subject string) {
		writeFile(t, dir, name, subject+"\n")
		runGit(t, dir, "add", name)
		runGit(t, dir, "commit", "-m", subject)
	}
	commit("readme", "Add a readme")
	runGit(t, dir, "switch", "-c", "parser")
	commit("parser", "Start the parser")
	runGit(t, dir, "switch", "-c", "errors")
	commit("errors", "Report parse errors")
	runGit(t, dir, "switch", "main")
	commit("licence", "Add a licence")
	runGit(t, dir, "switch", "parser")
	commit("tests", "Test the parser")
	runGit(t, dir, "switch", "main")
	commit("build", "Add a build script")
	runGit(t, dir, "merge", "--no-edit", "parser")
	runGit(t, dir, "merge", "--no-edit", "errors")
	commit("docs", "Document the parser")
	runGit(t, dir, "tag", "v1.0.0")
	return dir
}

// TestScreenshot draws the window to the path in DELTA_SCREENSHOT, in the
// theme in DELTA_THEME, showing what DELTA_VIEW says: the changes, or the
// history.
func TestScreenshot(t *testing.T) {
	path := os.Getenv("DELTA_SCREENSHOT")
	if path == "" {
		t.Skip("no path given")
	}
	// The bar shows the name of the repository's folder.
	dir := filepath.Join(t.TempDir(), "example")
	if err := os.Rename(screenshotRepo(t), dir); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "file", "one\nTWO\nthree\nfour\n")
	writeFile(t, dir, "staged.go", "package main\n")
	runGit(t, dir, "add", "staged.go")
	writeFile(t, dir, "untracked.txt", "new\n")
	w := newTestWindow(t, dir)
	waitFor(t, "the files", func() bool { return w.idleWith(2, 1) })
	on(func() {
		if os.Getenv("DELTA_VIEW") == "history" {
			w.showHistory.SetChecked(true)
		}
		applyTheme(os.Getenv("DELTA_THEME"))
		w.changes.unstaged.SetCurrentRow(0)
		w.changes.message.SetPlainText("Make the file shout")
		w.say("")
	})
	waitFor(t, "the diff", func() bool { return strings.Contains(w.changes.diff.parsed.Text, "+TWO") })
	time.Sleep(200 * time.Millisecond)
	on(func() {
		if !w.Grab().Save(path) {
			t.Error("not saved")
		}
		applyTheme("")
	})
}

// historyRepo returns a repository with commits of a file each after the first.
func historyRepo(t *testing.T, commits int) string {
	t.Helper()
	dir := newRepo(t)
	for i := range commits {
		name := fmt.Sprintf("file%d", i)
		writeFile(t, dir, name, "text of "+name+"\n")
		runGit(t, dir, "add", name)
		runGit(t, dir, "commit", "-m", "Commit "+name)
	}
	return dir
}

// treeRow returns the texts of a row of the commits.
func treeRow(h *historyView, row int) []string {
	item := h.commits.TopLevelItem(row)
	return []string{item.Text(colSubject), item.Text(colAuthor), item.Text(colDate), item.Text(colCommit)}
}

// TestStartsOnTheChanges checks that the history is not shown by itself, even
// with nothing to commit, until it is asked for.
func TestStartsOnTheChanges(t *testing.T) {
	dir := historyRepo(t, 1)
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.status.Branch == "main" && !w.busy })
	time.Sleep(100 * time.Millisecond)
	on(func() {
		if w.historyShown() || w.showHistory.IsChecked() {
			t.Error("the history is shown")
		}
		if w.history.commits.TopLevelItemCount() != 0 {
			t.Error("the history was read before it was on show")
		}
		w.Close()
	})
	// The panes of the history, never shown, are saved as they were.
	if got, want := settings.Load(w.path), settings.Defaults(); got.History != want.History {
		t.Errorf("history panes saved as %v", got.History)
	}
}

func TestChangesShownWithChanges(t *testing.T) {
	for name, change := range map[string]func(t *testing.T, dir string){
		"unstaged":  func(t *testing.T, dir string) { writeFile(t, dir, "file", "changed\n") },
		"untracked": func(t *testing.T, dir string) { writeFile(t, dir, "new", "new\n") },
		"staged": func(t *testing.T, dir string) {
			writeFile(t, dir, "new", "new\n")
			runGit(t, dir, "add", "new")
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := newRepo(t)
			change(t, dir)
			w := newTestWindow(t, dir)
			waitFor(t, "the status", func() bool { return w.status.Branch == "main" })
			on(func() {
				if w.historyShown() {
					t.Error("the history is shown")
				}
				if w.history.commits.TopLevelItemCount() != 0 {
					t.Error("the history was read before it was on show")
				}
			})
		})
	}
}

func TestOpeningAnotherRepositoryStaysOnTheChanges(t *testing.T) {
	dirty := newRepo(t)
	writeFile(t, dirty, "file", "changed\n")
	clean := newRepo(t)
	w := newTestWindow(t, dirty)
	waitFor(t, "the status", func() bool { return w.idleWith(1, 0) })
	on(func() {
		if err := w.openRepository(clean); err != nil {
			t.Fatal(err)
		}
	})
	waitFor(t, "the other repository", func() bool { return w.idleWith(0, 0) && w.status.Branch == "main" })
	on(func() {
		if w.historyShown() {
			t.Error("the history is shown")
		}
	})
}

func TestHistoryShowsCommits(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "a", strings.Repeat("a line\n", 200))
	writeFile(t, dir, "b", "b\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "Two files\n\nThe body")
	w := newTestWindow(t, dir)
	h := w.history
	waitFor(t, "the status", func() bool { return w.status.Branch == "main" })
	on(func() {
		if !h.page.IsAncestorOf(h.commits.QWidget) || !h.split.IsAncestorOf(h.files.QWidget) || !h.detail.IsAncestorOf(h.diff.QWidget) {
			t.Error("the panes are not in place")
		}
		w.showHistory.SetChecked(true)
	})
	waitFor(t, "the commits", func() bool { return h.commits.TopLevelItemCount() == 2 })
	// The newest is chosen, with its files listed, and the first of them chosen
	// and its diff alone shown, as in the changes.
	waitFor(t, "the files", func() bool { return h.files.Count() == 2 && h.diff.raw != "" })
	commit := ""
	on(func() {
		if row := treeRow(h, 0); !strings.Contains(row[0], "Two files") || !strings.Contains(row[0], "main") || row[1] != "Test" || row[2] == "" || row[3] == "" {
			t.Errorf("row %q", row)
		}
		commit = treeRow(h, 0)[3]
		if got := listTexts(h.files); !slices.Equal(got, []string{"a", "b"}) || !slices.Equal(listCodes(h.files), []byte{'A', 'A'}) {
			t.Errorf("files %q", got)
		}
		if h.files.CurrentRow() != 0 || h.diffLabel.Text() != "Commit "+commit+": a" {
			t.Errorf("row %d chosen, label %q", h.files.CurrentRow(), h.diffLabel.Text())
		}
		if text := h.diff.parsed.Text; !strings.Contains(text, "+a") || strings.Contains(text, "+b") || strings.Contains(text, "diff --git") || strings.Contains(text, "+++") {
			t.Errorf("diff %q", text)
		}
		if h.fileLabel.Text() != "Files (2)" {
			t.Errorf("label %q", h.fileLabel.Text())
		}
		// Choosing another file shows its diff in place of the first's.
		h.files.SetCurrentRow(1)
	})
	waitFor(t, "the second file", func() bool { return h.diffLabel.Text() == "Commit "+commit+": b" })
	on(func() {
		if text := h.diff.parsed.Text; !strings.Contains(text, "+b") || strings.Contains(text, "+a") {
			t.Errorf("diff %q", text)
		}
		// Choosing another commit shows that one's first file.
		h.commits.SetCurrentItem(h.commits.TopLevelItem(1))
	})
	waitFor(t, "the first commit", func() bool {
		return h.files.Count() == 1 && strings.HasSuffix(h.diffLabel.Text(), ": file") && strings.Contains(h.diff.parsed.Text, "+one")
	})
}

func TestHistoryFollowsChanges(t *testing.T) {
	dir := historyRepo(t, 1)
	w := newTestWindow(t, dir)
	h := w.history
	on(func() { w.showHistory.SetChecked(true) })
	waitFor(t, "the commits", func() bool { return h.commits.TopLevelItemCount() == 2 })
	waitFor(t, "the choice", func() bool { return h.chosen() != nil })
	var kept string
	on(func() {
		h.commits.SetCurrentItem(h.commits.TopLevelItem(1))
		kept = h.chosen().Hash
	})

	// A commit made elsewhere shows on a rescan, with the same commit chosen.
	writeFile(t, dir, "later", "later\n")
	runGit(t, dir, "add", "later")
	runGit(t, dir, "commit", "-m", "Later")
	on(func() { w.rescan.Trigger() })
	waitFor(t, "the new commit", func() bool { return h.commits.TopLevelItemCount() == 3 })
	on(func() {
		if h.chosen() == nil || h.chosen().Hash != kept {
			t.Errorf("chosen %+v, not %s", h.chosen(), kept)
		}
	})

	// One made while the changes are on show shows on coming back.
	writeFile(t, dir, "file", "changed\n")
	on(func() {
		w.showHistory.SetChecked(false)
		w.rescan.Trigger()
	})
	waitFor(t, "the change", func() bool { return w.idleWith(1, 0) })
	on(func() { w.stageAll.Trigger() })
	waitFor(t, "staged", func() bool { return w.idleWith(0, 1) })
	on(func() {
		w.changes.message.SetPlainText("From the changes")
		w.commit.Trigger()
	})
	waitFor(t, "committed", func() bool { return w.idleWith(0, 0) })
	on(func() {
		if h.commits.TopLevelItemCount() != 3 {
			t.Error("the history was read while not on show")
		}
		w.showHistory.SetChecked(true)
	})
	waitFor(t, "the commit from the changes", func() bool {
		return h.commits.TopLevelItemCount() == 4 && strings.Contains(treeRow(h, 0)[0], "From the changes")
	})
}

func TestHistoryShowsUncommittedChanges(t *testing.T) {
	dir := historyRepo(t, 3)
	writeFile(t, dir, "file", "changed\n")
	writeFile(t, dir, "untracked", "untracked\n")
	w := newTestWindow(t, dir)
	h := w.history
	on(func() {
		h.pageSize = 2
		w.showHistory.SetChecked(true)
	})
	// It comes first and is chosen, on top of the first page of real commits.
	waitFor(t, "the rows", func() bool { return h.commits.TopLevelItemCount() == 3 && !h.loading })
	waitFor(t, "its files", func() bool { return h.files.Count() == 2 && h.diff.raw != "" })
	on(func() {
		if row := treeRow(h, 0); row[0] != "Uncommitted changes" || row[3] != "" {
			t.Errorf("row %q", row)
		}
		if !h.commits.TopLevelItem(0).Font(colSubject).Italic() {
			t.Error("it is not in italics")
		}
		if got := listTexts(h.files); !slices.Equal(got, []string{"file", "untracked"}) || !slices.Equal(listCodes(h.files), []byte{'M', git.Untracked}) {
			t.Errorf("files %q", got)
		}
		if !strings.Contains(h.diff.parsed.Text, "+changed") || strings.Contains(h.diff.parsed.Text, "+untracked") {
			t.Errorf("diff %q", h.diff.parsed.Text)
		}
		if h.diffLabel.Text() != "Uncommitted: file" {
			t.Errorf("label %q", h.diffLabel.Text())
		}
		// It joins the graph to the commit it is on.
		if got, want := h.graph.rows[0].Bottom, h.graph.rows[1].Top; len(got) != 1 || len(want) != 1 || got[0].To != want[0].From {
			t.Errorf("not joined: %v, %v", got, want)
		}
		// Reading more doesn't count it as a commit.
		w.loadMoreHistory()
	})
	waitFor(t, "the next page", func() bool { return h.commits.TopLevelItemCount() == 5 && !h.loading })

	// New changes to it show on a rescan, with the file chosen kept and its
	// diff read again.
	writeFile(t, dir, "later", "later\n")
	writeFile(t, dir, "file", "changed again\n")
	on(func() { w.rescan.Trigger() })
	waitFor(t, "the new file", func() bool {
		return h.files.Count() == 3 && h.diffLabel.Text() == "Uncommitted: file" && strings.Contains(h.diff.parsed.Text, "+changed again")
	})

	// Once it is committed the row has gone.
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "All of it")
	on(func() { w.rescan.Trigger() })
	waitFor(t, "the commit", func() bool {
		return !h.loading && strings.Contains(treeRow(h, 0)[0], "All of it") && h.list[0].Hash != git.Uncommitted
	})
}

func TestHistoryReadsMoreAtTheEnd(t *testing.T) {
	dir := historyRepo(t, 4)
	w := newTestWindow(t, dir)
	h := w.history
	on(func() {
		h.pageSize = 2
		w.showHistory.SetChecked(true)
	})
	waitFor(t, "the first page", func() bool { return h.commits.TopLevelItemCount() == 2 && !h.loading })
	for _, want := range []int{4, 5} {
		on(func() { w.loadMoreHistory() })
		waitFor(t, "the next page", func() bool { return h.commits.TopLevelItemCount() == want && !h.loading })
	}
	on(func() {
		if h.more {
			t.Error("more is expected after the last page")
		}
		w.loadMoreHistory()
		if h.loading {
			t.Error("read past the end")
		}
		for i, c := range h.list {
			if !strings.Contains(treeRow(h, i)[0], c.Subject) {
				t.Errorf("row %d is %q", i, treeRow(h, i))
			}
		}
	})
}

func TestGraphIsDrawn(t *testing.T) {
	dir := newRepo(t)
	runGit(t, dir, "switch", "-c", "topic")
	writeFile(t, dir, "theirs", "theirs\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "theirs")
	runGit(t, dir, "switch", "main")
	writeFile(t, dir, "ours", "ours\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "ours")
	runGit(t, dir, "merge", "--no-edit", "topic")

	w := newTestWindow(t, dir)
	h := w.history
	on(func() {
		applyTheme(settings.ThemeLight)
		w.showHistory.SetChecked(true)
	})
	waitFor(t, "the commits", func() bool { return h.commits.TopLevelItemCount() == 4 })
	on(func() {
		if len(h.graph.rows) != 4 || h.layout.Width() != 2 {
			t.Fatalf("%d rows, %d lanes", len(h.graph.rows), h.layout.Width())
		}
		if got, want := h.commits.Header().SectionSize(colGraph), graphWidth(2); got != want {
			t.Errorf("column %d wide, not %d", got, want)
		}
		// The merge's node shows, in the colour of its lane, in a row with room,
		// with the list not focused, as the highlight of its current row would
		// show through.
		h.files.SetFocus()
		h.commits.ClearSelection()
		item := h.commits.TopLevelItem(0)
		rect := h.commits.VisualItemRect(item)
		if rect.Height() < graphRow {
			t.Errorf("row is %d high", rect.Height())
		}
		image := h.commits.Viewport().Grab().ToImage()
		got := image.PixelColor(graphMargin, rect.Top()+rect.Height()/2).Name()
		if got != graphColours[0] {
			t.Errorf("node is %s, not %s", got, graphColours[0])
		}
		applyTheme("")
	})
}

func TestHistoryOfEmptyAndOtherRepository(t *testing.T) {
	empty := t.TempDir()
	dir := historyRepo(t, 1)
	runGit(t, empty, "init", "-b", "main")
	w := newTestWindow(t, dir)
	h := w.history
	on(func() { w.showHistory.SetChecked(true) })
	waitFor(t, "the commits", func() bool { return h.files.Count() == 1 })
	on(func() {
		if err := w.openRepository(empty); err != nil {
			t.Fatal(err)
		}
	})
	waitFor(t, "nothing shown", func() bool {
		return w.repo.Root != "" && !h.loading && h.commits.TopLevelItemCount() == 0 && h.files.Count() == 0 && h.diff.parsed.Text == ""
	})
}

func TestHistoryPanesAreSaved(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "file", "changed\n")
	// Never shown, they keep what they were.
	w := newTestWindow(t, dir)
	on(func() { w.Close() })
	if got, want := settings.Load(w.path), settings.Defaults(); got.History != want.History || got.HistoryDetail != want.HistoryDetail {
		t.Errorf("panes %v %v", got.History, got.HistoryDetail)
	}

	again := newTestWindowAt(t, dir, w.path)
	on(func() {
		again.showHistory.SetChecked(true)
		again.history.split.SetSizes([]int{200, 450})
		again.history.detail.SetSizes([]int{150, 900})
	})
	var want [2]settings.Panes
	waitFor(t, "the layout", func() bool { return again.history.split.Sizes()[0] == 200 })
	on(func() {
		want = [2]settings.Panes{panes(again.history.split), panes(again.history.detail)}
		again.Close()
	})
	got := settings.Load(w.path)
	if got.History != want[0] || got.HistoryDetail != want[1] {
		t.Errorf("saved %v %v, want %v", got.History, got.HistoryDetail, want)
	}
}

func TestAboutBox(t *testing.T) {
	on(func() {
		icon := newAppIcon()
		defer icon.Delete()
		if icon.IsNull() {
			t.Error("the icon is empty")
		}
		d := newAboutDialog(nil, icon)
		defer d.Delete()
		if d.WindowTitle() != "About Delta" {
			t.Errorf("title %q", d.WindowTitle())
		}
	})
}

func TestFormDialogValidates(t *testing.T) {
	on(func() {
		d := newFormDialog(nil, "Title")
		defer d.Delete()
		other := d.addText("Other", "filled", notEmpty)
		edit := d.addText("Name", "", notEmpty)
		if d.ok.IsEnabled() {
			t.Error("OK is enabled with nothing typed")
		}
		edit.SetText("  ")
		if d.ok.IsEnabled() {
			t.Error("OK is enabled with only spaces typed")
		}
		edit.SetText("name")
		if !d.ok.IsEnabled() {
			t.Error("OK is disabled")
		}
		// Every field counts, whichever was typed in last.
		other.SetText("")
		if d.ok.IsEnabled() {
			t.Error("OK is enabled with another field empty")
		}
	})
}

// sendMouse sends a mouse event to the middle of a row of a list.
func sendMouse(list *qt.QListWidget, kind qt.QEvent__Type, row int, button, buttons qt.MouseButton, modifiers qt.KeyboardModifier) {
	rect := list.VisualItemRect(list.Item(row))
	position := qt.NewQPointF3(float64(rect.Left()+rect.Width()/2), float64(rect.Top()+rect.Height()/2))
	defer position.Delete()
	event := qt.NewQMouseEvent(kind, position, button, buttons, modifiers)
	defer event.Delete()
	qt.QCoreApplication_SendEvent(list.Viewport().QObject, event.QEvent)
}

func TestDraggingShowsEachFile(t *testing.T) {
	dir := newRepo(t)
	for _, name := range []string{"a", "b", "c"} {
		writeFile(t, dir, name, "text of "+name+"\n")
	}
	w := newTestWindow(t, dir)
	waitFor(t, "the files", func() bool { return w.idleWith(3, 0) })
	list := w.changes.unstaged
	press := func(row int, modifiers qt.KeyboardModifier) {
		sendMouse(list, qt.QEvent__MouseButtonPress, row, qt.LeftButton, qt.LeftButton, modifiers)
	}
	release := func(row int, modifiers qt.KeyboardModifier) {
		sendMouse(list, qt.QEvent__MouseButtonRelease, row, qt.LeftButton, qt.NoButton, modifiers)
	}
	selected := func() []int {
		var rows []int
		for _, item := range list.SelectedItems() {
			rows = append(rows, list.Row(item))
		}
		slices.Sort(rows)
		return rows
	}
	on(func() { press(0, qt.NoModifier) })
	waitFor(t, "the first file", func() bool { return strings.Contains(w.changes.diff.parsed.Text, "text of a") })
	for row, name := range []string{"a", "b", "c"} {
		on(func() { sendMouse(list, qt.QEvent__MouseMove, row, qt.NoButton, qt.LeftButton, qt.NoModifier) })
		waitFor(t, "the file dragged over", func() bool { return strings.Contains(w.changes.diff.parsed.Text, "text of "+name) })
		on(func() {
			if got := selected(); !slices.Equal(got, []int{row}) {
				t.Errorf("over row %d, selected %v", row, got)
			}
			if w.changes.diffLabel.Text() != "Unstaged: "+name {
				t.Errorf("label %q", w.changes.diffLabel.Text())
			}
		})
	}
	// And back again, and it stays one file on letting go.
	on(func() {
		sendMouse(list, qt.QEvent__MouseMove, 1, qt.NoButton, qt.LeftButton, qt.NoModifier)
		release(1, qt.NoModifier)
		if got := selected(); !slices.Equal(got, []int{1}) {
			t.Errorf("after the drag, selected %v", got)
		}

		// Ctrl and shift choose several, by clicks.
		press(0, qt.ControlModifier)
		release(0, qt.ControlModifier)
		if got := selected(); !slices.Equal(got, []int{0, 1}) {
			t.Errorf("after ctrl+click, selected %v", got)
		}
		press(2, qt.ShiftModifier)
		release(2, qt.ShiftModifier)
		if got := selected(); len(got) < 2 || !slices.Contains(got, 2) {
			t.Errorf("after shift+click, selected %v", got)
		}
		// A drag with ctrl held doesn't lose them.
		before := selected()
		sendMouse(list, qt.QEvent__MouseMove, 0, qt.NoButton, qt.LeftButton, qt.ControlModifier)
		if got := selected(); !slices.Equal(got, before) {
			t.Errorf("after ctrl+drag, selected %v, not %v", got, before)
		}
	})
}

// New Branch takes a name git would refuse as it stands, as it is corrected on
// OK, and refuses only one that can't be: OK is enabled while the name, made fit
// for a branch, can be one.
func TestNewBranchName(t *testing.T) {
	w := newTestWindow(t, newRepo(t))
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })
	on(func() {
		d, name, _, _ := w.newBranchDialog([]string{"main"}, nil)
		defer d.Delete()
		for text, ok := range map[string]bool{
			"":              false,
			"topic":         true,
			"my new branch": true,
			"what? ~now~":   true,
			"~^:?*":         false,
			"@":             false,
			"HEAD":          false,
		} {
			name.SetText(text)
			if d.ok.IsEnabled() != ok {
				t.Errorf("%q: OK enabled %v", text, d.ok.IsEnabled())
			}
		}
	})
	// What is made is the name corrected.
	on(func() { w.createBranch(git.SanitiseBranchName("my new branch"), "", true) })
	waitFor(t, "the branch", func() bool { return w.idleWith(0, 0) && w.status.Branch == "my-new-branch" })
}

// The files of the staged and unstaged lists have no tooltips; the history's
// still say what happened to each in words.
func TestChangeListsHaveNoTooltips(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "file", "changed\n")
	writeFile(t, dir, "staged", "new\n")
	runGit(t, dir, "add", "staged")
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.idleWith(1, 1) })
	on(func() {
		for _, list := range []*qt.QListWidget{w.changes.unstaged, w.changes.staged} {
			if tip := list.Item(0).ToolTip(); tip != "" {
				t.Errorf("%q has the tooltip %q", list.Item(0).Text(), tip)
			}
		}
		w.history.setFiles([]git.Change{{Code: 'M', Path: "file"}})
		if tip := w.history.files.Item(0).ToolTip(); tip != "Modified" {
			t.Errorf("the history's file has the tooltip %q", tip)
		}
	})
}

// Discard All Changes has a shortcut, and no two of the window's actions share
// one.
func TestShortcuts(t *testing.T) {
	w := newTestWindow(t, newRepo(t))
	on(func() {
		if key := w.discard.Shortcut().ToString(); key != "Ctrl+Shift+D" {
			t.Errorf("Discard All Changes is on %q", key)
		}
		seen := map[string]string{}
		// The window's actions are its children.
		for _, child := range w.Children() {
			if !child.Inherits("QAction") {
				continue
			}
			a := qt.UnsafeNewQAction(child.UnsafePointer())
			key := a.Shortcut().ToString()
			if key == "" {
				continue
			}
			if other, ok := seen[key]; ok {
				t.Errorf("%q and %q are both on %s", other, a.Text(), key)
			}
			seen[key] = a.Text()
		}
		if len(seen) < 15 {
			t.Errorf("only %d shortcuts found", len(seen))
		}
	})
}
