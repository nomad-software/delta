package ui

import (
	"testing"
	"time"

	"github.com/nomad-software/delta/internal/settings"
)

// progressShown says whether the progress bar is on show.
func (w *Window) progressShown() bool {
	return w.progress.IsVisible()
}

// TestProgress checks that the progress bar, under the toolbar and the width
// of the window, shows only while work goes on for longer than a moment, busy
// rather than counting, and goes once the last of it is done; and that the
// panes under it stay put as it does.
func TestProgress(t *testing.T) {
	dir := newRepo(t)
	w := newTestWindow(t, dir)
	waitFor(t, "the work of starting", func() bool { return w.working == 0 && !w.busy })
	var top int
	on(func() {
		if w.progressShown() {
			t.Error("shown to start with")
		}
		bar := w.progress.QWidget
		x, y := mapPoint(bar, w.QWidget, 0, 0)
		if x != 0 || bar.Width() != w.Width() || w.progress.Minimum() != 0 || w.progress.Maximum() != 0 || w.progress.IsTextVisible() {
			t.Errorf("at %d, %d wide in %d, range %d to %d", x, bar.Width(), w.Width(), w.progress.Minimum(), w.progress.Maximum())
		}
		if _, below := mapPoint(w.toolbar.QWidget, w.QWidget, 0, w.toolbar.Height()); y != below {
			t.Errorf("at %d, under a toolbar ending at %d", y, below)
		}
		_, top = mapPoint(w.views.QWidget, w.QWidget, 0, 0)
		// Work done in a moment never shows it.
		w.beginWork()
		w.endWork()
	})
	time.Sleep(2 * progressDelay * time.Millisecond)
	on(func() {
		if w.progressShown() {
			t.Error("quick work showed the progress bar")
		}
	})

	// A command that takes a while shows it until it is done.
	release := make(chan struct{})
	on(func() {
		w.background("Slow", func() (string, error) {
			<-release
			return "", nil
		}, func(string, error) {})
	})
	waitFor(t, "the progress bar", w.progressShown)
	var was int
	on(func() {
		if !w.progress.moving() {
			t.Error("the progress bar is not moving")
		}
		was, _ = w.progress.chunk()
		if _, moved := mapPoint(w.views.QWidget, w.QWidget, 0, 0); moved != top {
			t.Errorf("the panes moved from %d to %d", top, moved)
		}
		// Two at once: it stays until both are done.
		w.beginWork()
	})
	waitFor(t, "the segment to move", func() bool { now, _ := w.progress.chunk(); return now != was })
	close(release)
	waitFor(t, "the command", func() bool { return !w.busy })
	on(func() {
		if !w.progressShown() {
			t.Error("the progress bar went with work still going on")
		}
		w.endWork()
	})
	waitFor(t, "the rescan after it", func() bool { return w.working == 0 })
	on(func() {
		if w.progressShown() || w.progress.moving() {
			t.Error("the progress bar is still there")
		}
	})
}

// TestWorkPairs checks that every piece of background work that starts ends,
// those whose answers are dropped too, as the progress bar would otherwise stay.
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
		w.changes.unstaged.SetCurrentRow(0)
	})
	waitFor(t, "everything to be done", func() bool { return w.working == 0 && !w.history.loading })
	on(func() {
		if w.progressShown() {
			t.Error("the progress bar stayed")
		}
	})
}

// The segment comes in at the left, crosses, and goes out at the right, never
// beyond the bar.
func TestProgressChunk(t *testing.T) {
	on(func() {
		p := newProgress()
		defer p.Delete()
		p.Resize(400, progressHeight)
		for phase, want := range map[float64][2]int{
			0:   {0, 0},
			0.1: {0, 50},
			0.5: {150, 250},
			0.9: {350, 400},
			1:   {400, 400},
		} {
			p.phase = phase
			if left, right := p.chunk(); left != want[0] || right != want[1] {
				t.Errorf("at %v, %d to %d, not %v", phase, left, right, want)
			}
		}
	})
}

// The progress bar's segment is in its colour in any theme.
func TestProgressColour(t *testing.T) {
	on(func() {
		defer applyTheme("")
		for _, theme := range []string{settings.ThemeLight, settings.ThemeDark} {
			applyTheme(theme)
			p := newProgress()
			p.colour = settings.Magenta
			p.Resize(400, progressHeight)
			p.phase = 0.5
			image := p.Grab().ToImage()
			left, right := p.chunk()
			if got := image.PixelColor((left+right)/2, progressHeight/2).Name(); got != settings.Magenta {
				t.Errorf("%s: the segment is %s", theme, got)
			}
			p.Delete()
		}
	})
}
