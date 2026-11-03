package ui

import (
	"testing"
	"time"

	qt "github.com/mappu/miqt/qt6"
)

// spinnerShown says whether the spinner is on show in the toolbar.
func (w *Window) spinnerShown() bool {
	actions := w.toolbar.Actions()
	last := actions[len(actions)-1]
	return w.toolbar.WidgetForAction(last).UnsafePointer() == w.spinner.UnsafePointer() && last.IsVisible()
}

// TestSpinner checks that the spinner, at the right of the toolbar, shows only
// while work goes on for longer than a moment, spinning, and goes once the last
// of it is done.
func TestSpinner(t *testing.T) {
	dir := newRepo(t)
	w := newTestWindow(t, dir)
	waitFor(t, "the work of starting", func() bool { return w.working == 0 && !w.busy })
	on(func() {
		if w.spinnerShown() || w.spinner.text.Text() != "Working" {
			t.Errorf("shown %v with %q", w.spinnerShown(), w.spinner.text.Text())
		}
		// Work done in a moment never shows it.
		w.beginWork()
		w.endWork()
	})
	time.Sleep(2 * spinnerDelay * time.Millisecond)
	on(func() {
		if w.spinnerShown() {
			t.Error("quick work showed the spinner")
		}
	})

	// A command that takes a while shows it, spinning, until it is done.
	release := make(chan struct{})
	on(func() {
		w.background("Slow", func() (string, error) {
			<-release
			return "", nil
		}, func(string, error) {})
	})
	waitFor(t, "the spinner", w.spinnerShown)
	on(func() {
		if !w.spinner.spinning() || w.spinner.icon.Pixmap(qt.ReturnByValue).IsNull() {
			t.Error("the spinner is not spinning")
		}
		// Two at once: it stays until both are done.
		w.beginWork()
	})
	close(release)
	waitFor(t, "the command", func() bool { return !w.busy })
	on(func() {
		if !w.spinnerShown() {
			t.Error("the spinner went with work still going on")
		}
		w.endWork()
	})
	waitFor(t, "the rescan after it", func() bool { return w.working == 0 })
	on(func() {
		if w.spinnerShown() || w.spinner.spinning() {
			t.Error("the spinner is still there")
		}
	})
}

// TestWorkPairs checks that every piece of background work that starts ends,
// those whose answers are dropped too, as the spinner would otherwise stay.
func TestWorkPairs(t *testing.T) {
	dir := historyRepo(t, 3)
	writeFile(t, dir, "file", "changed\n")
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.idleWith(1, 0) })
	on(func() {
		// Rescans, diffs and histories, each asked for again before the last
		// answers, so that answers are dropped.
		w.showHistory.SetChecked(true)
		for range 3 {
			w.rescanRepository()
			w.showDiff()
			w.loadHistory()
		}
		w.tab.unstaged.SetCurrentRow(0)
	})
	waitFor(t, "everything to be done", func() bool { return w.working == 0 && !w.history.loading })
	on(func() {
		if w.spinnerShown() {
			t.Error("the spinner stayed")
		}
	})
}
