package ui

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/nomad-software/delta/internal/instance"
	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtwidgets"
)

// The window is brought back when the program is started again, from being
// minimised or hidden, keeping it maximised if it was, with the desktop's token
// in the environment for Qt.
func TestActivate(t *testing.T) {
	t.Setenv("XDG_ACTIVATION_TOKEN", "")
	w := newTestWindow(t, newRepo(t))
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })
	on(func() {
		w.SetWindowState(qtcore.WindowMaximized | qtcore.WindowMinimized)
		w.Activate(instance.Request{Token: "token-1"})
		if w.IsMinimized() || !w.IsMaximized() || !w.IsVisible() {
			t.Errorf("minimised: state %v, visible %v", w.WindowState(), w.IsVisible())
		}
		if got := os.Getenv("XDG_ACTIVATION_TOKEN"); got != "token-1" {
			t.Errorf("token %q", got)
		}
		w.Hide()
		w.Activate(instance.Request{})
		if !w.IsVisible() {
			t.Error("hidden: not shown")
		}
	})
}

// The repository a copy started later would have opened is opened, unless it
// is the one open; a folder named that is in none is told of, and the folder it
// was started in is ignored if it is in none.
func TestActivateOpens(t *testing.T) {
	first, other := namedRepo(t, "first"), namedRepo(t, "other")
	outside := t.TempDir()
	w := newTestWindow(t, first)
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })
	// Each is looked for in the background, and then opened or not.
	activate := func(r instance.Request) {
		on(func() { w.Activate(r) })
		waitFor(t, "the folder to be looked for", func() bool { return w.finding == 0 })
	}
	on(func() { w.changes.message.SetPlainText("kept") })
	activate(instance.Request{Folder: first})
	on(func() {
		if w.repo.Root != first || w.changes.message.ToPlainText() != "kept" {
			t.Error("the repository open was opened again")
		}
	})
	activate(instance.Request{Folder: outside})
	on(func() {
		if w.repo.Root != first || len(w.failures) != 0 {
			t.Errorf("a folder started in, in no repository: %q, %q", w.repo.Root, w.failures)
		}
	})
	activate(instance.Request{Folder: outside, Named: true})
	on(func() {
		if w.repo.Root != first || len(w.failures) != 1 {
			t.Errorf("a folder named, in no repository: %q, %q", w.repo.Root, w.failures)
		}
	})
	activate(instance.Request{Folder: other})
	on(func() {
		if w.repo.Root != other {
			t.Errorf("open %q", w.repo.Root)
		}
	})
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })
	// Not while a command runs.
	release := make(chan struct{})
	on(func() {
		w.background("Slow", func() (string, error) {
			<-release
			return "", nil
		}, func(string, error) {})
	})
	activate(instance.Request{Folder: first})
	on(func() {
		if w.repo.Root != other || !strings.Contains(w.said.Text(), "a command is running") {
			t.Errorf("opened while a command runs, or said %q", w.said.Text())
		}
	})
	close(release)
	waitFor(t, "the command", func() bool { return !w.busy })
}

// A folder opened from File > Open, or named on starting, is looked for in the
// background, during which a command may start; then it is not opened, as the
// command's answer would go to it, as from the bar or Activate.
func TestOpenFolderNotWhileBusy(t *testing.T) {
	first, other := namedRepo(t, "first"), namedRepo(t, "other")
	w := newTestWindow(t, first)
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })
	release := make(chan struct{})
	on(func() {
		w.background("Slow", func() (string, error) {
			<-release
			return "", nil
		}, func(string, error) {})
		w.openFolder(other)
	})
	waitFor(t, "the folder to be looked for", func() bool { return w.finding == 0 })
	on(func() {
		if w.repo.Root != first || !strings.Contains(w.said.Text(), "a command is running") {
			t.Errorf("opened %q while a command runs, or said %q", w.repo.Root, w.said.Text())
		}
	})
	close(release)
	waitFor(t, "the command", func() bool { return !w.busy })
	on(func() { w.openFolder(other) })
	waitFor(t, "the folder to be looked for", func() bool { return w.finding == 0 })
	on(func() {
		if w.repo.Root != other {
			t.Errorf("open %q", w.repo.Root)
		}
	})
}

// A repository found while a dialog is open is not opened, as what is chosen in
// the dialog is for the one open, and would go to the other once answered.
func TestOpenFolderNotWhileADialogIsOpen(t *testing.T) {
	first, other := namedRepo(t, "first"), namedRepo(t, "other")
	w := newTestWindow(t, first)
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })
	answer(t, func() { qtwidgets.NewQDialog(w).Exec() }, func(*qtwidgets.QDialog) {
		w.Activate(instance.Request{Folder: other, Named: true})
	})
	waitFor(t, "the folder to be looked for", func() bool { return w.finding == 0 })
	on(func() {
		if w.repo.Root != first || !strings.Contains(w.said.Text(), "a dialog is open") {
			t.Errorf("opened %q while a dialog is open, or said %q", w.repo.Root, w.said.Text())
		}
		qtwidgets.QApplicationActiveModalWidget().Close()
	})
	waitFor(t, "the dialog to close", func() bool { return qtwidgets.QApplicationActiveModalWidget() == nil })
	on(func() { w.Activate(instance.Request{Folder: other, Named: true}) })
	waitFor(t, "the folder to be looked for", func() bool { return w.finding == 0 })
	on(func() {
		if w.repo.Root != other {
			t.Errorf("open %q", w.repo.Root)
		}
	})
}

// A repository chosen in the bar, then dropped for the one open being asked
// for, as on starting the program again in it, leaves the bar on the one open.
func TestBarChoiceDroppedForTheOneOpen(t *testing.T) {
	first, other := namedRepo(t, "first"), namedRepo(t, "other")
	w := newTestWindow(t, other)
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })
	on(func() { w.openFolder(first) })
	waitFor(t, "the status", func() bool { return w.repo.Root == first && w.idleWith(0, 0) })
	on(func() {
		at := slices.Index(w.bar.paths, other)
		w.bar.repos.SetCurrentIndex(at)
		w.chooseListedRepository(at)
		w.Activate(instance.Request{Folder: first})
	})
	waitFor(t, "the folders to be looked for", func() bool { return w.finding == 0 })
	on(func() {
		if got := w.bar.repos.CurrentIndex(); w.repo.Root != first || w.bar.paths[got] != first {
			t.Errorf("open %q, the bar on %q", w.repo.Root, w.bar.paths[got])
		}
	})
}
