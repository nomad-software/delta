package ui

import (
	"os"
	"testing"

	qt "github.com/mappu/miqt/qt6"

	"github.com/nomad-software/delta/internal/instance"
)

// The window is brought back when the program is started again, from being
// minimised or hidden, keeping it maximised if it was, with the desktop's token
// in the environment for Qt.
func TestActivate(t *testing.T) {
	t.Setenv("XDG_ACTIVATION_TOKEN", "")
	w := newTestWindow(t, newRepo(t))
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })
	on(func() {
		w.SetWindowState(qt.WindowMaximized | qt.WindowMinimized)
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
	on(func() {
		w.changes.message.SetPlainText("kept")
		w.Activate(instance.Request{Folder: first})
		if w.repo.Root != first || w.changes.message.ToPlainText() != "kept" {
			t.Error("the repository open was opened again")
		}
		w.Activate(instance.Request{Folder: outside})
		if w.repo.Root != first || len(w.failures) != 0 {
			t.Errorf("a folder started in, in no repository: %q, %q", w.repo.Root, w.failures)
		}
		w.Activate(instance.Request{Folder: outside, Named: true})
		if w.repo.Root != first || len(w.failures) != 1 {
			t.Errorf("a folder named, in no repository: %q, %q", w.repo.Root, w.failures)
		}
		w.Activate(instance.Request{Folder: other})
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
		w.Activate(instance.Request{Folder: first})
		if w.repo.Root != other {
			t.Error("opened while a command runs")
		}
	})
	close(release)
	waitFor(t, "the command", func() bool { return !w.busy })
}
