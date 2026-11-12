package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/nomad-software/delta/internal/instance"
	"github.com/nomad-software/qute/qtcore"
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
