package ui

import (
	"fmt"
	"strings"
	"testing"

	qt "github.com/mappu/miqt/qt6"

	"github.com/nomad-software/delta/internal/git"
)

// releaseRepo returns a repository on main, tagged v1.2.3, with an upstream in
// a bare remote, and the remote.
func releaseRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := newRepo(t)
	bare := t.TempDir()
	runGit(t, bare, "init", "-q", "--bare", "-b", "main")
	runGit(t, dir, "remote", "add", "origin", bare)
	runGit(t, dir, "push", "-q", "-u", "origin", "main")
	runGit(t, dir, "tag", "v1.2.3")
	runGit(t, dir, "commit", "-q", "--allow-empty", "-m", "more")
	return dir, bare
}

// spinners returns the dialog's spin boxes: major, minor and patch.
func spinners(d *qt.QDialog) []*qt.QSpinBox {
	return children(d.QObject, "QSpinBox", qt.UnsafeNewQSpinBox)
}

// spunVersion returns the version the spin boxes are set to.
func spunVersion(s []*qt.QSpinBox) string {
	return fmt.Sprintf("%d.%d.%d", s[0].Value(), s[1].Value(), s[2].Value())
}

// labelTexts returns the texts of the dialog's labels, joined.
func labelTexts(d *qt.QDialog) string {
	var said []string
	for _, l := range children(d.QObject, "QLabel", qt.UnsafeNewQLabel) {
		said = append(said, l.Text())
	}
	return strings.Join(said, "|")
}

// Tag Release, in the Remote menu after a separator, on Ctrl+T, reads the
// latest version and starts at the next patch release; any of its parts can be
// changed, but none to below what is above the latest. It tags and
// pushes the one set, annotated, with the branch.
func TestTagRelease(t *testing.T) {
	dir, bare := releaseRepo(t)
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) && w.tagAction.IsEnabled() })
	on(func() {
		if key := w.tagAction.Shortcut().ToString(); key != "Ctrl+T" || w.tagAction.Text() != "&Tag Release…" {
			t.Errorf("%q on %q", w.tagAction.Text(), key)
		}
		for _, o := range w.tagAction.AssociatedObjects() {
			if !o.Inherits("QMenu") {
				continue
			}
			actions := qt.UnsafeNewQMenu(o.UnsafePointer()).Actions()
			for i, a := range actions {
				if a.UnsafePointer() == w.tagAction.UnsafePointer() && (i == 0 || !actions[i-1].IsSeparator()) {
					t.Error("Tag Release has no separator above it")
				}
			}
		}
	})
	// Cancelled, nothing is done.
	answer(t, w.tagAction.Trigger, func(d *qt.QDialog) { d.Reject() })
	answer(t, w.tagAction.Trigger, func(d *qt.QDialog) {
		ok := children(d.QObject, "QPushButton", qt.UnsafeNewQPushButton)[0]
		s := spinners(d)
		if len(s) != 3 || s[0].Value() != 1 || s[1].Value() != 2 || s[2].Value() != 4 || !ok.IsEnabled() {
			t.Fatalf("starts at %d.%d.%d", s[0].Value(), s[1].Value(), s[2].Value())
		}
		// One row, New, of the prefix and the three side by side.
		for i := 1; i < 3; i++ {
			if s[i].Y() != s[0].Y() || s[i].X() <= s[i-1].X() {
				t.Errorf("spin box %d is at %d,%d after %d,%d", i, s[i].X(), s[i].Y(), s[i-1].X(), s[i-1].Y())
			}
		}
		if said := labelTexts(d); !strings.Contains(said, "|New|v|") {
			t.Errorf("said %q", said)
		}
		if said := labelTexts(d); !strings.Contains(said, "v1.2.3|") || !strings.Contains(said, "origin") || strings.Contains(said, "already") {
			t.Errorf("said %q", said)
		}
		// No spin box goes below what would be above the latest: not the
		// patch to v1.2.3, nor the minor to v1.1, nor the major to v0.
		for i, want := range []int{1, 2, 4} {
			if s[i].SetValue(0); s[i].Value() != want {
				t.Errorf("spin box %d went to %d", i, s[i].Value())
			}
		}
		// Raised, the parts after it can go to 0; lowered again, they are
		// raised to what is above the latest.
		s[0].SetValue(2)
		s[1].SetValue(0)
		s[2].SetValue(0)
		if spunVersion(s) != "2.0.0" || !ok.IsEnabled() {
			t.Errorf("at %s, OK %v", spunVersion(s), ok.IsEnabled())
		}
		s[0].SetValue(1)
		if spunVersion(s) != "1.2.4" {
			t.Errorf("lowered to %s", spunVersion(s))
		}
		// More than one part changed.
		s[1].SetValue(3)
		s[2].SetValue(0)
		if spunVersion(s) != "1.3.0" || !ok.IsEnabled() {
			t.Errorf("at %s, OK %v", spunVersion(s), ok.IsEnabled())
		}
		if said := labelTexts(d); strings.Contains(said, "1.3.0") {
			t.Errorf("the version is said again: %q", said)
		}
		// The message is the last field of text, after the spin boxes' own.
		fields := texts(d)
		fields[len(fields)-1].SetText("Minor release")
		d.Accept()
	})
	waitFor(t, "the release", func() bool { return w.idle() && strings.Contains(runGitOut(t, bare, "tag", "--list"), "v1.3.0") })
	if got := runGitOut(t, dir, "tag", "-l", "--format=%(objecttype) %(contents:subject)", "v1.3.0"); got != "tag Minor release\n" {
		t.Errorf("tagged %q", got)
	}
	if runGitOut(t, bare, "rev-parse", "main") != runGitOut(t, dir, "rev-parse", "HEAD") {
		t.Error("the branch was not pushed")
	}
	waitFor(t, "the message", func() bool { return strings.Contains(w.said.Text(), "Tagged v1.3.0") })

	// Now HEAD is tagged, which is noted, with changes not committed.
	writeFile(t, dir, "file", "changed\n")
	on(w.rescanRepository)
	waitFor(t, "the rescan", func() bool { return w.idleWith(1, 0) })
	answer(t, w.tagAction.Trigger, func(d *qt.QDialog) {
		said := labelTexts(d)
		if !strings.Contains(said, "tagged v1.3.0 already") || !strings.Contains(said, "not committed") {
			t.Errorf("said %q", said)
		}
		if got := spunVersion(spinners(d)); got != "1.3.1" {
			t.Errorf("starts at %s", got)
		}
		d.Reject()
	})
	waitFor(t, "the dialog to go", func() bool { return w.idle() && w.working == 0 })
	if len(w.failures) != 0 {
		t.Errorf("failures %q", w.failures)
	}
}

// A release with nowhere to go is told of, without a dialog; one refused by the
// remote is told of, leaving no tag.
func TestTagReleaseFails(t *testing.T) {
	w := newTestWindow(t, newRepo(t))
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) && w.tagAction.IsEnabled() })
	on(w.tagAction.Trigger)
	waitFor(t, "the failure", func() bool { return w.idle() && w.working == 0 && len(w.failures) == 1 })
	if !strings.Contains(w.failures[0], "no remote") {
		t.Errorf("failures %q", w.failures)
	}

	dir, bare := releaseRepo(t)
	other := t.TempDir()
	runGit(t, other, "clone", "-q", bare, ".")
	runGit(t, other, "commit", "-q", "--allow-empty", "-m", "theirs")
	runGit(t, other, "push", "-q", "origin", "main")
	w = newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) && w.tagAction.IsEnabled() })
	answer(t, w.tagAction.Trigger, func(d *qt.QDialog) { d.Accept() })
	waitFor(t, "the failure", func() bool { return w.idle() && w.working == 0 && len(w.failures) == 1 })
	if !strings.Contains(w.failures[0], "deleted again") || runGitOut(t, dir, "tag", "--list") != "v1.2.3\n" {
		t.Errorf("failures %q, tags %q", w.failures, runGitOut(t, dir, "tag", "--list"))
	}
}

// The dialog's spin boxes share its width, growing with it as the message does,
// and it stays the height its contents need, leaving no gap when made taller.
func TestTagDialogResizes(t *testing.T) {
	on(func() {
		rel := git.Release{Remote: "origin", Latest: git.Version{Prefix: "v", Major: 1, Minor: 2, Patch: 3}, Found: true}
		d, _, message := newTagDialog(nil, rel, true)
		d.Show()
		qt.QCoreApplication_ProcessEvents()
		s := spinners(d.QDialog)
		before := s[0].Width()
		d.Resize(800, 500)
		qt.QCoreApplication_ProcessEvents()
		if d.Width() != 800 || d.Height() >= 500 || d.Height() != d.Layout().HeightForWidth(800) {
			t.Errorf("%d×%d", d.Width(), d.Height())
		}
		if s[0].Width() <= before || s[1].Width() != s[0].Width() || s[2].Width() < s[0].Width()-1 || s[2].Width() > s[0].Width()+1 {
			t.Errorf("spin boxes %d, %d and %d wide, from %d", s[0].Width(), s[1].Width(), s[2].Width(), before)
		}
		if end, _ := mapPoint(s[2].QWidget, d.QWidget, s[2].Width(), 0); end != message.X()+message.Width() {
			t.Errorf("the spin boxes end at %d, the message at %d", end, message.X()+message.Width())
		}
		d.Close()
	})
}
