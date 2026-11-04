package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	qt "github.com/mappu/miqt/qt6"

	"github.com/nomad-software/delta/internal/settings"
)

// namedRepo is newRepo in a folder of the name.
func namedRepo(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.Rename(newRepo(t), dir); err != nil {
		t.Fatal(err)
	}
	// git names it by its real path, as where temporary folders are may be a
	// link, as on macOS.
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

// repoNames returns the names the bar lists the repositories by.
func (w *testWindow) repoNames() []string {
	var names []string
	for i := range w.bar.repos.Count() {
		names = append(names, w.bar.repos.ItemText(i))
	}
	return names
}

// Each repository opened is remembered, and saved at once, once however often
// it is opened, starting in it included; the bar lists them by the names of
// their folders, in order, choosing the one open.
func TestRepositoriesRemembered(t *testing.T) {
	beta, alpha := namedRepo(t, "beta"), namedRepo(t, "Alpha")
	t.Chdir(beta)
	w := newTestWindow(t, "")
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })
	if got := settings.Load(w.path).Repositories; !slices.Equal(got, []string{beta}) {
		t.Fatalf("saved %q on starting in one", got)
	}
	on(func() {
		for _, dir := range []string{alpha, beta, filepath.Join(alpha, ".")} {
			if err := w.openRepository(dir); err != nil {
				t.Fatal(err)
			}
		}
		if names := w.repoNames(); !slices.Equal(names, []string{"Alpha", "beta"}) {
			t.Errorf("listed %q", names)
		}
		if w.bar.repos.CurrentText() != "Alpha" || !w.bar.repos.IsEnabled() {
			t.Errorf("chosen %q, enabled %v", w.bar.repos.CurrentText(), w.bar.repos.IsEnabled())
		}
	})
	if got := settings.Load(w.path).Repositories; !slices.Equal(got, []string{beta, alpha}) {
		t.Errorf("saved %q", got)
	}
	// A new window lists them before any is open.
	other := newTestWindowAt(t, alpha, w.path)
	on(func() {
		if names := other.repoNames(); !slices.Equal(names, []string{"Alpha", "beta"}) {
			t.Errorf("a new window listed %q", names)
		}
	})
}

// Folders of the same name are listed by it, in order of their whole paths,
// which are their tooltips.
func TestByFolder(t *testing.T) {
	got := byFolder([]string{"/z/delta", "/b/Zeta", "/a/delta", "/c/alpha"})
	if want := []string{"/c/alpha", "/a/delta", "/z/delta", "/b/Zeta"}; !slices.Equal(got, want) {
		t.Errorf("got %q", got)
	}
}

// Choosing a repository in the bar opens it; choosing one that is no longer
// there says so and keeps the one open chosen.
func TestChooseRepository(t *testing.T) {
	one, two := namedRepo(t, "one"), namedRepo(t, "two")
	w := newTestWindow(t, one)
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })
	on(func() {
		if err := w.openRepository(two); err != nil {
			t.Fatal(err)
		}
	})
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })
	on(func() {
		w.bar.repos.Activated(slices.Index(w.repoNames(), "one"))
		if w.repo.Root != one {
			t.Errorf("open %q", w.repo.Root)
		}
	})
	if err := os.RemoveAll(two); err != nil {
		t.Fatal(err)
	}
	on(func() {
		w.bar.repos.Activated(slices.Index(w.repoNames(), "two"))
		if w.repo.Root != one || w.bar.repos.CurrentText() != "one" || len(w.failures) != 1 {
			t.Errorf("open %q, chosen %q, failures %q", w.repo.Root, w.bar.repos.CurrentText(), w.failures)
		}
	})
}

// The bar lists the local branches in order, choosing the current one, and
// choosing another checks it out.
func TestChooseBranch(t *testing.T) {
	dir := newRepo(t)
	runGit(t, dir, "branch", "zebra")
	runGit(t, dir, "branch", "Apple")
	w := newTestWindow(t, dir)
	waitFor(t, "the branches", func() bool { return w.idleWith(0, 0) && w.bar.branches.Count() == 3 })
	on(func() {
		var names []string
		for i := range w.bar.branches.Count() {
			names = append(names, w.bar.branches.ItemText(i))
		}
		if !slices.Equal(names, []string{"Apple", "main", "zebra"}) || w.bar.branches.CurrentText() != "main" || !w.bar.branches.IsEnabled() {
			t.Errorf("listed %q, chosen %q", names, w.bar.branches.CurrentText())
		}
		w.bar.branches.Activated(2)
	})
	waitFor(t, "the checkout", func() bool {
		return w.idleWith(0, 0) && w.status.Branch == "zebra" && w.bar.branches.CurrentText() == "zebra"
	})
	// On a detached HEAD, none is chosen.
	runGit(t, dir, "switch", "--detach", "main")
	on(w.rescanRepository)
	waitFor(t, "the detached HEAD", func() bool { return w.idleWith(0, 0) && w.bar.branches.CurrentIndex() == -1 })
}

// Before the first commit, git lists no branches, but the current one is.
func TestUnbornBranchListed(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "start")
	w := newTestWindow(t, dir)
	waitFor(t, "the branch", func() bool { return w.idleWith(0, 0) && w.bar.branches.CurrentText() == "start" })
}

// The dropdowns are the last of the toolbar, at the right, and centred on its
// height. Both are on show in a window of the default size, rather than in the
// toolbar's menu of what does not fit.
func TestRepoBarInToolbar(t *testing.T) {
	w := newTestWindow(t, namedRepo(t, "short"))
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })
	on(func() {
		actions := w.toolbar.Actions()
		if w.toolbar.WidgetForAction(actions[len(actions)-1]).UnsafePointer() != w.bar.widget.UnsafePointer() {
			t.Fatal("the dropdowns are not last")
		}
		history := w.toolbar.WidgetForAction(w.showHistory)
		for _, c := range []*qt.QComboBox{w.bar.repos, w.bar.branches} {
			if !c.IsVisible() {
				t.Errorf("%q is not on show", c.CurrentText())
				continue
			}
			x, y := mapPoint(c.QWidget, w.toolbar.QWidget, 0, c.Height()/2)
			if middle := w.toolbar.Height() / 2; y < middle-2 || y > middle+2 {
				t.Errorf("%q is centred at %d of %d", c.CurrentText(), y, w.toolbar.Height())
			}
			if x < history.X()+history.Width() {
				t.Errorf("%q is left of History", c.CurrentText())
			}
			if c.Width() != repoComboLeast {
				t.Errorf("%q is %d wide", c.CurrentText(), c.Width())
			}
		}
		if right, _ := mapPoint(w.bar.widget, w.toolbar.QWidget, w.bar.widget.Width(), 0); w.toolbar.Width()-right > 16 {
			t.Errorf("the dropdowns end %d short of the right", w.toolbar.Width()-right)
		}
	})
}

// A dropdown grows with the names it lists, up to repoComboMost, and narrows
// where the toolbar has no room for it, down to repoComboLeast, before it
// leaves the toolbar.
func TestRepoComboWidth(t *testing.T) {
	long := namedRepo(t, strings.Repeat("long-name-", 10))
	w := newTestWindow(t, namedRepo(t, "short"))
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })
	width := func(window int) int {
		var got int
		on(func() {
			w.Resize(window, 700)
			w.toolbar.Layout().Activate()
			got = w.bar.repos.Width()
			if !w.bar.repos.IsVisible() {
				got = 0
			}
		})
		return got
	}
	if got := width(2000); got != repoComboLeast {
		t.Errorf("short names %d wide", got)
	}
	on(func() {
		if err := w.openRepository(long); err != nil {
			t.Fatal(err)
		}
	})
	if got := width(2000); got != repoComboMost {
		t.Errorf("a long name %d wide in a wide window", got)
	}
	if got := width(settings.Defaults().Width); got <= repoComboLeast || got >= repoComboMost {
		t.Errorf("a long name %d wide in a window of the default width", got)
	}
	if got := width(1100); got != 0 {
		t.Errorf("%d wide in a narrow window, which has no room", got)
	}
}

// Started with no repository named, outside any, the one last opened is opened
// again; one inside a repository opens that instead; and one last opened that
// is gone is said to be, leaving none open.
func TestReopensLastRepository(t *testing.T) {
	last, here := namedRepo(t, "last"), namedRepo(t, "here")
	outside := t.TempDir()
	path := filepath.Join(t.TempDir(), "settings.json")
	first := newTestWindowAt(t, last, path)
	waitFor(t, "the status", func() bool { return first.idleWith(0, 0) })
	if got := settings.Load(path).LastRepository; got != last {
		t.Fatalf("saved %q as the last", got)
	}

	t.Chdir(outside)
	again := newTestWindowAt(t, "", path)
	on(func() {
		if again.repo == nil || again.repo.Root != last {
			t.Errorf("opened %v", again.repo)
		}
	})

	t.Chdir(here)
	inside := newTestWindowAt(t, "", path)
	on(func() {
		if inside.repo == nil || inside.repo.Root != here {
			t.Errorf("opened %v inside another", inside.repo)
		}
	})
	if got := settings.Load(path).LastRepository; got != here {
		t.Errorf("saved %q as the last", got)
	}

	if err := os.RemoveAll(here); err != nil {
		t.Fatal(err)
	}
	t.Chdir(outside)
	gone := newTestWindowAt(t, "", path)
	on(func() {
		if gone.repo != nil || !strings.Contains(gone.said.Text(), here) || len(gone.failures) != 0 {
			t.Errorf("opened %v, said %q, failures %q", gone.repo, gone.said.Text(), gone.failures)
		}
	})
}

// On opening a repository, its first unstaged file is chosen, showing its
// diff, but not on later rescans, nor a staged one.
func TestFirstUnstagedChosen(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "file", "changed\n")
	writeFile(t, dir, "a-new-file", "new\n")
	w := newTestWindow(t, dir)
	waitFor(t, "the first file", func() bool {
		return w.idleWith(2, 0) && w.changes.unstaged.CurrentRow() == 0 && len(w.changes.unstaged.SelectedItems()) == 1
	})
	waitFor(t, "its diff", func() bool { return w.changes.diffLabel.Text() == "Unstaged: "+w.status.Unstaged[0].Path })
	on(func() {
		w.changes.unstaged.ClearSelection()
		w.rescanRepository()
	})
	waitFor(t, "the rescan", func() bool { return w.working == 0 })
	on(func() {
		if len(w.changes.unstaged.SelectedItems()) != 0 {
			t.Error("a rescan chose a file")
		}
	})

	staged := newRepo(t)
	writeFile(t, staged, "file", "changed\n")
	runGit(t, staged, "add", "file")
	on(func() {
		if err := w.openRepository(staged); err != nil {
			t.Fatal(err)
		}
	})
	waitFor(t, "the status", func() bool { return w.idleWith(0, 1) && w.working == 0 })
	on(func() {
		if len(w.changes.staged.SelectedItems()) != 0 || w.chooseFirst {
			t.Error("a staged file was chosen")
		}
	})
}
