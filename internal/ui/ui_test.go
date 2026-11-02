package ui

import (
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

	"github.com/nomad-software/merge-monkey/internal/settings"
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
	t.Cleanup(func() {
		// It is closed but not deleted, as the answer to a command still running
		// comes back to it, as it can in the program between closing and leaving.
		mainthread.Wait(func() { tw.Close() })
	})
	return tw
}

// on runs the function on the main thread.
func on(fn func()) { mainthread.Wait(fn) }

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

func (w *Window) idleWith(unstaged, staged int) bool {
	return !w.busy && w.tab.unstaged.Count() == unstaged && w.tab.staged.Count() == staged
}

func TestLayout(t *testing.T) {
	w := newTestWindow(t, newRepo(t))
	on(func() {
		if w.tabs.Count() != 1 || w.tabs.TabText(0) != "Branch" {
			t.Errorf("tabs: %d, first %q", w.tabs.Count(), w.tabs.TabText(0))
		}
		var menus []string
		for _, a := range w.MenuBar().Actions() {
			menus = append(menus, strings.ReplaceAll(a.Text(), "&", ""))
		}
		if want := []string{"File", "Branch", "Commit", "Remote", "Help"}; !slices.Equal(menus, want) {
			t.Errorf("menus %v", menus)
		}
		for _, b := range []struct {
			button *qt.QPushButton
			text   string
		}{{w.tab.rescan, "Rescan"}, {w.tab.stage, "Stage Changes"}, {w.tab.commit, "Commit"}, {w.tab.push, "Push"}} {
			if b.button.Text() != b.text {
				t.Errorf("button %q", b.button.Text())
			}
		}
		if w.tab.lists.Widget(0).IsAncestorOf(w.tab.staged.QWidget) || !w.tab.lists.Widget(0).IsAncestorOf(w.tab.unstaged.QWidget) {
			t.Error("the unstaged list is not above the staged one")
		}
	})
}

func TestPanesLook(t *testing.T) {
	w := newTestWindow(t, newRepo(t))
	on(func() {
		for _, label := range []*qt.QLabel{w.tab.unstagedLabel, w.tab.stagedLabel, w.tab.diffLabel} {
			if !label.Font().Bold() {
				t.Errorf("%q is not bold", label.Text())
			}
		}
		// Only dots are drawn on a handle, at its centre.
		handle := w.tab.sidebar.Handle(1)
		if w.tab.sidebar.HandleWidth() != paneHandleWidth || handle.Width() != paneHandleWidth {
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
		if got := listTexts(w.tab.unstaged); !slices.Equal(got, []string{"M  file", "?  new"}) {
			t.Errorf("unstaged %q", got)
		}
		if w.commit.IsEnabled() {
			t.Error("commit enabled with nothing staged")
		}
		// Choosing a file shows its diff.
		w.tab.unstaged.SetCurrentRow(0)
	})
	waitFor(t, "the diff", func() bool { return strings.Contains(w.tab.diff.text, "+TWO") })
	on(func() {
		if w.tab.diffLabel.Text() != "Unstaged: file" {
			t.Errorf("label %q", w.tab.diffLabel.Text())
		}
		// The button stages all that is listed.
		w.tab.stage.Click()
	})
	waitFor(t, "all staged", func() bool { return w.idleWith(0, 2) })

	// Choosing a staged file shows the staged diff. Its selection survived.
	on(func() {
		w.tab.staged.SetCurrentRow(1)
	})
	waitFor(t, "the staged diff", func() bool { return w.tab.diffLabel.Text() == "Staged: new" })

	// Selected files are unstaged, and staged again, alone.
	on(func() { w.unstageSelected.Trigger() })
	waitFor(t, "one unstaged", func() bool { return w.idleWith(1, 1) })
	on(func() {
		w.tab.unstaged.SetCurrentRow(0)
		w.stageSelected.Trigger()
	})
	waitFor(t, "both staged", func() bool { return w.idleWith(0, 2) })

	on(func() {
		w.tab.message.SetPlainText("  ")
		if w.tab.commit.IsEnabled() {
			t.Error("commit enabled with a blank message")
		}
		w.tab.message.SetPlainText("A message")
		if !w.tab.commit.IsEnabled() {
			t.Error("commit disabled")
		}
		w.tab.commit.Click()
	})
	waitFor(t, "the commit", func() bool { return w.idleWith(0, 0) })
	on(func() {
		if w.tab.message.ToPlainText() != "" {
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
		if w.tab.stage.Text() != stageText || !w.tab.stage.IsEnabled() {
			t.Errorf("button %q, enabled %v", w.tab.stage.Text(), w.tab.stage.IsEnabled())
		}
		w.tab.stage.Click()
	})
	waitFor(t, "all staged", func() bool { return w.idleWith(0, 2) })
	on(func() {
		if w.tab.stage.Text() != unstageText || !w.tab.stage.IsEnabled() {
			t.Errorf("button %q, enabled %v", w.tab.stage.Text(), w.tab.stage.IsEnabled())
		}
		w.tab.stage.Click()
	})
	waitFor(t, "all unstaged", func() bool { return w.idleWith(2, 0) })

	// With some of each it stages the rest.
	on(func() {
		w.tab.unstaged.SetCurrentRow(0)
		w.stageSelected.Trigger()
	})
	waitFor(t, "one of each", func() bool { return w.idleWith(1, 1) })
	on(func() {
		if w.tab.stage.Text() != stageText {
			t.Errorf("button %q", w.tab.stage.Text())
		}
		w.tab.stage.Click()
	})
	waitFor(t, "all staged again", func() bool { return w.idleWith(0, 2) })

	// With nothing changed there is nothing for it to do.
	on(func() {
		w.tab.message.SetPlainText("Message")
		w.commit.Trigger()
	})
	waitFor(t, "the commit", func() bool { return w.idleWith(0, 0) })
	on(func() {
		if w.tab.stage.Text() != stageText || w.tab.stage.IsEnabled() {
			t.Errorf("button %q, enabled %v", w.tab.stage.Text(), w.tab.stage.IsEnabled())
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
		w.tab.message.SetPlainText("Keep me")
		w.commit.Trigger()
	})
	waitFor(t, "the failure", func() bool { return len(w.failures) == 1 && !w.busy })
	on(func() {
		if !strings.Contains(w.failures[0], "hook says no") {
			t.Errorf("failure %q", w.failures[0])
		}
		if w.tab.message.ToPlainText() != "Keep me" {
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
	on(func() { w.tab.push.Click() })
	waitFor(t, "the upstream", func() bool { return w.status.Upstream == "origin/main" })

	writeFile(t, dir, "file", "rewritten\n")
	runGit(t, dir, "commit", "-a", "--amend", "-m", "rewritten")
	on(func() { w.rescan.Trigger() })
	waitFor(t, "diverged", func() bool { return w.status.Behind == 1 })

	// Without force it is refused.
	on(func() { w.tab.push.Click() })
	waitFor(t, "the refusal", func() bool { return len(w.failures) == 1 && !w.busy })

	// Saying no does nothing.
	on(func() {
		w.tab.force.SetChecked(true)
		if w.tab.push.Text() != "Force Push" {
			t.Errorf("button %q", w.tab.push.Text())
		}
		w.answer = false
		w.tab.push.Click()
		if w.busy || len(w.asked) != 1 {
			t.Errorf("busy %v, asked %v", w.busy, w.asked)
		}
		w.answer = true
		w.tab.push.Click()
	})
	waitFor(t, "the force push", func() bool { return !w.busy && w.status.Behind == 0 && w.status.Ahead == 0 })
	on(func() {
		if w.tab.force.IsChecked() || w.tab.push.Text() != "Push" {
			t.Error("force is still on after pushing")
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
		if len(w.asked) != 3 {
			t.Errorf("asked %q", w.asked)
		}
	})
	waitFor(t, "the deletion", func() bool {
		local, _, _ := w.repo.Branches()
		return !w.busy && !slices.Contains(local, "topic")
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
	waitFor(t, "the message", func() bool { return strings.Contains(w.tab.message.ToPlainText(), "Merge branch 'topic'") })
	on(func() {
		if !w.abortMerge.IsEnabled() {
			t.Error("abort is disabled")
		}
		if got := listTexts(w.tab.unstaged); !slices.Equal(got, []string{"U  file"}) {
			t.Errorf("unstaged %q", got)
		}
		w.tab.unstaged.SetCurrentRow(0)
	})
	waitFor(t, "the conflict's diff", func() bool { return strings.Contains(w.tab.diff.text, "<<<<<<<") })
	on(func() { w.abortMerge.Trigger() })
	waitFor(t, "the abort", func() bool { return !w.status.Merging && !w.busy })
	on(func() {
		if w.tab.message.ToPlainText() != "" {
			t.Error("the message was left behind")
		}
	})
}

// background returns the background of a line of the diff, as a colour name.
func background(v *diffView, line int) string {
	return v.Document().FindBlockByNumber(line).BlockFormat().Background().Color().Name()
}

func TestDiffColours(t *testing.T) {
	v := (*diffView)(nil)
	on(func() {
		applyTheme(settings.ThemeLight)
		v = newDiffView("#00ff00", "#ff0000")
		v.show("diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1 +1 @@\n-old\n+new\n same\n")
		// 30% of the colour in the light theme's white.
		if got := background(v, 5); got != "#b2ffb2" {
			t.Errorf("added line %s", got)
		}
		if got := background(v, 4); got != "#ffb2b2" {
			t.Errorf("removed line %s", got)
		}
		context, removed := background(v, 6), background(v, 4)
		if context == removed {
			t.Errorf("context line %s", context)
		}

		// A change of colour shows at once, with the text as it was.
		v.setColours("#0000ff", "#ff0000")
		if got := background(v, 5); got != "#b2b2ff" {
			t.Errorf("recoloured line %s", got)
		}
		if !strings.Contains(v.ToPlainText(), "+new") {
			t.Error("the text was lost")
		}

		// So does one of theme: 30% of blue in the dark theme's #1f1f1f.
		applyTheme(settings.ThemeDark)
	})
	waitFor(t, "the dark tint", func() bool { return background(v, 5) == "#151562" })
	on(func() {
		applyTheme("")
		v.Delete()
	})
}

func TestLargeDiffDrawsQuickly(t *testing.T) {
	var b strings.Builder
	b.WriteString("diff --git a/f b/f\n@@ -1 +1 @@\n")
	for i := range 100000 {
		fmt.Fprintf(&b, "+line %d of a long new file\n", i)
	}
	on(func() {
		v := newDiffView("#00ff00", "#ff0000")
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

func TestSettingsDialog(t *testing.T) {
	on(func() {
		current := settings.Defaults()
		current.Theme = settings.ThemeDark
		current.AddedColour = "#123456"
		current.Repository = "/kept"
		var got settings.Settings
		d := newSettingsDialog(nil, current, func(s settings.Settings) { got = s })
		defer d.Delete()
		if d.tabs.Count() != 1 || d.tabs.TabText(0) != "User interface" || !d.tabs.Widget(0).IsAncestorOf(d.theme.QWidget) {
			t.Errorf("tabs: %d, first %q", d.tabs.Count(), d.tabs.TabText(0))
		}
		if d.theme.CurrentText() != "Dark" || d.theme.Count() != 3 || d.added.colour != "#123456" {
			t.Errorf("shown as %q, %s", d.theme.CurrentText(), d.added.colour)
		}
		d.theme.SetCurrentIndex(1)
		d.removed.set("#abcdef")
		d.box.Button(qt.QDialogButtonBox__Ok).Click()
		if got.Theme != settings.ThemeLight || got.RemovedColour != "#abcdef" || got.Repository != "/kept" {
			t.Errorf("chosen %+v", got)
		}
		d.box.Button(qt.QDialogButtonBox__RestoreDefaults).Click()
		if d.theme.CurrentIndex() != 0 || d.added.colour != settings.DefaultAdded {
			t.Errorf("defaults %q %s", d.theme.CurrentText(), d.added.colour)
		}
	})
}

func TestSizesAreSaved(t *testing.T) {
	dir := newRepo(t)
	w := newTestWindow(t, dir)
	on(func() {
		w.Resize(1000, 700)
		w.tab.sidebar.SetSizes([]int{222, 760})
		w.tab.lists.SetSizes([]int{111, 400})
		w.tab.editor.SetSizes([]int{333, 250})
	})
	// The event loop has its turn to lay them out.
	waitFor(t, "the size", func() bool { return w.Width() == 1000 })
	var want settings.Settings
	on(func() {
		want.Sidebar, want.Lists, want.Editor = panes(w.tab.sidebar), panes(w.tab.lists), panes(w.tab.editor)
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
	if got.Repository != w.repo.Root {
		t.Errorf("repository %q", got.Repository)
	}

	// A new window opens with them.
	again := newTestWindowAt(t, dir, w.path)
	waitFor(t, "the layout", func() bool { return again.tab.sidebar.Sizes()[0] != 0 })
	on(func() {
		if again.Width() != 1000 || panes(again.tab.sidebar)[0] != 222 {
			t.Errorf("reopened at %d wide, sidebar %v", again.Width(), panes(again.tab.sidebar))
		}
	})
}

func TestNoRepository(t *testing.T) {
	outside := t.TempDir()
	t.Chdir(outside)
	w := newTestWindow(t, "")
	on(func() {
		if w.repo != nil {
			t.Fatalf("opened %q", w.repo.Root)
		}
		if w.rescan.IsEnabled() || w.push.IsEnabled() || w.tab.commit.IsEnabled() {
			t.Error("actions are enabled with no repository")
		}
		if err := w.openRepository(outside); err == nil {
			t.Error("a folder that is no repository was opened")
		}
	})
}

// TestScreenshot draws the window to the path in MERGE_MONKEY_SCREENSHOT, in the
// theme in MERGE_MONKEY_THEME.
func TestScreenshot(t *testing.T) {
	path := os.Getenv("MERGE_MONKEY_SCREENSHOT")
	if path == "" {
		t.Skip("no path given")
	}
	dir := newRepo(t)
	writeFile(t, dir, "file", "one\nTWO\nthree\nfour\n")
	writeFile(t, dir, "staged.go", "package main\n")
	runGit(t, dir, "add", "staged.go")
	writeFile(t, dir, "untracked.txt", "new\n")
	w := newTestWindow(t, dir)
	waitFor(t, "the files", func() bool { return w.idleWith(2, 1) })
	on(func() {
		applyTheme(os.Getenv("MERGE_MONKEY_THEME"))
		w.tab.unstaged.SetCurrentRow(0)
		w.tab.message.SetPlainText("Make the file shout")
	})
	waitFor(t, "the diff", func() bool { return strings.Contains(w.tab.diff.text, "+TWO") })
	time.Sleep(200 * time.Millisecond)
	on(func() {
		if !w.Grab().Save(path) {
			t.Error("not saved")
		}
		applyTheme("")
	})
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
		if d.WindowTitle() != "About Merge Monkey" {
			t.Errorf("title %q", d.WindowTitle())
		}
	})
}

func TestFormDialogValidates(t *testing.T) {
	on(func() {
		d := newFormDialog(nil, "Title")
		defer d.Delete()
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
	})
}
