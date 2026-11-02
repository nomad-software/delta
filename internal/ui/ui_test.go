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

	"github.com/nomad-software/merge-monkey/internal/git"
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
		if w.tabs.Count() != 2 || w.tabs.TabText(0) != "Branch" || w.tabs.TabText(historyTabIndex) != "History" {
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

func TestCodeFontSetting(t *testing.T) {
	on(func() {
		system := codeFont("", 0)
		defer system.Delete()
		var got settings.Settings
		d := newSettingsDialog(nil, settings.Defaults(), func(s settings.Settings) { got = s })
		defer d.Delete()

		// Left alone, it follows the system.
		if d.codeSize.Value() != system.PointSize() {
			t.Errorf("size shown %d, not %d", d.codeSize.Value(), system.PointSize())
		}
		if s := d.chosen(settings.Defaults()); s.CodeFont != "" || s.CodeFontSize != 0 {
			t.Errorf("unchanged gave %q, %d", s.CodeFont, s.CodeFontSize)
		}

		// Any font can be chosen, not only those of a fixed width.
		if d.codeFamily.FontFilters() != qt.QFontComboBox__AllFonts {
			t.Errorf("fonts are limited by %v", d.codeFamily.FontFilters())
		}

		// A size and another family are kept.
		d.codeSize.SetValue(system.PointSize() + 3)
		other := -1
		for i := range d.codeFamily.Count() {
			if d.codeFamily.ItemText(i) != d.defaultFamily {
				other = i
				break
			}
		}
		family := ""
		if other >= 0 {
			d.codeFamily.SetCurrentIndex(other)
			family = d.codeFamily.CurrentText()
		}
		d.box.Button(qt.QDialogButtonBox__Ok).Click()
		if got.CodeFontSize != system.PointSize()+3 || got.CodeFont != family {
			t.Errorf("chose %q, %d", got.CodeFont, got.CodeFontSize)
		}

		// The button beside them goes back to the system's.
		d.codeDefault.Click()
		if s := d.chosen(settings.Defaults()); s.CodeFont != "" || s.CodeFontSize != 0 {
			t.Errorf("default button gave %q, %d", s.CodeFont, s.CodeFontSize)
		}

		// Restoring the defaults shows the system's again.
		d.codeSize.SetValue(system.PointSize() + 1)
		d.box.Button(qt.QDialogButtonBox__RestoreDefaults).Click()
		if s := d.chosen(settings.Defaults()); s.CodeFont != "" || s.CodeFontSize != 0 {
			t.Errorf("defaults gave %q, %d", s.CodeFont, s.CodeFontSize)
		}
	})
}

func TestCodeFontIsApplied(t *testing.T) {
	w := newTestWindow(t, newRepo(t))
	on(func() {
		s := w.config
		s.CodeFont, s.CodeFontSize = "Some Font", 21
		w.applySettings(s)
		for _, widget := range []*qt.QWidget{w.tab.diff.QWidget, w.history.diff.QWidget, w.tab.message.QWidget} {
			font := widget.Font()
			if font.Family() != "Some Font" || font.PointSize() != 21 {
				t.Errorf("font %q, %d", font.Family(), font.PointSize())
			}
		}
		want := float64(tabWidth * w.tab.diff.FontMetrics().HorizontalAdvance(" "))
		if got := w.tab.diff.TabStopDistance(); got != want {
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
		if font := again.tab.diff.Font(); font.Family() != "Some Font" || font.PointSize() != 21 {
			t.Errorf("reopened with %q, %d", font.Family(), font.PointSize())
		}
	})
}

func TestColourTable(t *testing.T) {
	on(func() {
		current := settings.Defaults()
		current.HunkColour = "#111111"
		var got settings.Settings
		d := newSettingsDialog(nil, current, func(s settings.Settings) { got = s })
		defer d.Delete()
		if d.colours.ColumnCount() != 2 || d.colours.RowCount() != 4 {
			t.Fatalf("table is %d by %d", d.colours.ColumnCount(), d.colours.RowCount())
		}
		var names []string
		for row := range d.colours.RowCount() {
			names = append(names, d.colours.Item(row, colColourName).Text())
		}
		if want := []string{"Diff", "Added lines", "Removed lines", "Hunk headers"}; !slices.Equal(names, want) {
			t.Errorf("names %q", names)
		}
		if !d.colours.Item(0, colColourName).Font().Bold() || d.colours.ColumnSpan(0, colColourName) != 2 {
			t.Error("the section row is not a bold heading across the table")
		}
		if !d.colours.CellWidget(3, colColourSwatch).IsAncestorOf(d.hunk.QWidget) {
			t.Error("the button is not in the table")
		}
		if d.hunk.Text() != "#111111" {
			t.Errorf("shown as %q", d.hunk.Text())
		}
		d.hunk.set("#222222")
		d.box.Button(qt.QDialogButtonBox__Ok).Click()
		if got.HunkColour != "#222222" {
			t.Errorf("chose %q", got.HunkColour)
		}
	})
}

func TestTabKeys(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "file", "changed\n")
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.idleWith(1, 0) })
	on(func() {
		if len(w.tabKeys) != 2 || w.tabKeys[0].Key().ToString() != "Ctrl+1" || w.tabKeys[1].Key().ToString() != "Ctrl+2" {
			t.Fatalf("keys %v", w.tabKeys)
		}
		w.tabKeys[1].Activated()
		if w.tabs.CurrentIndex() != historyTabIndex {
			t.Errorf("on tab %d", w.tabs.CurrentIndex())
		}
		w.tabKeys[0].Activated()
		if w.tabs.CurrentIndex() != 0 {
			t.Errorf("on tab %d", w.tabs.CurrentIndex())
		}
	})
}

func TestSizesAreSaved(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "file", "changed\n")
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

// TestScreenshot draws the window to the path in MERGE_MONKEY_SCREENSHOT, in the
// theme in MERGE_MONKEY_THEME, on the tab MERGE_MONKEY_TAB says.
func TestScreenshot(t *testing.T) {
	path := os.Getenv("MERGE_MONKEY_SCREENSHOT")
	if path == "" {
		t.Skip("no path given")
	}
	dir := screenshotRepo(t)
	writeFile(t, dir, "file", "one\nTWO\nthree\nfour\n")
	writeFile(t, dir, "staged.go", "package main\n")
	runGit(t, dir, "add", "staged.go")
	writeFile(t, dir, "untracked.txt", "new\n")
	w := newTestWindow(t, dir)
	waitFor(t, "the files", func() bool { return w.idleWith(2, 1) })
	on(func() {
		if os.Getenv("MERGE_MONKEY_TAB") == "history" {
			w.tabs.SetCurrentIndex(historyTabIndex)
		}
		applyTheme(os.Getenv("MERGE_MONKEY_THEME"))
		w.tab.unstaged.SetCurrentRow(0)
		w.tab.message.SetPlainText("Make the file shout")
		w.say("")
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
func treeRow(h *historyTab, row int) []string {
	item := h.commits.TopLevelItem(row)
	return []string{item.Text(colSubject), item.Text(colAuthor), item.Text(colDate), item.Text(colCommit)}
}

func TestStartsOnHistoryWithNothingToCommit(t *testing.T) {
	dir := historyRepo(t, 1)
	w := newTestWindow(t, dir)
	waitFor(t, "the history tab", func() bool { return w.tabs.CurrentIndex() == historyTabIndex })
	waitFor(t, "the commits", func() bool { return w.history.commits.TopLevelItemCount() == 2 })

	// It is only on starting: going back and rescanning stays on the branch tab.
	on(func() {
		w.tabs.SetCurrentIndex(0)
		w.rescan.Trigger()
	})
	request := 0
	on(func() { request = w.rescans })
	waitFor(t, "the rescan", func() bool { return w.rescans == request && !w.busy })
	time.Sleep(100 * time.Millisecond)
	on(func() {
		if w.tabs.CurrentIndex() != 0 {
			t.Errorf("on tab %d after a rescan", w.tabs.CurrentIndex())
		}
		w.Close()
	})

	// The branch tab's panes, out of sight, are saved as they were.
	if got, want := settings.Load(w.path), settings.Defaults(); got.Lists[0] <= 0 || got.Sidebar[0] != want.Sidebar[0] {
		t.Errorf("panes saved as %v, %v", got.Sidebar, got.Lists)
	}
}

func TestStaysOnBranchWithChanges(t *testing.T) {
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
				if w.tabs.CurrentIndex() != 0 {
					t.Errorf("on tab %d", w.tabs.CurrentIndex())
				}
				if w.history.commits.TopLevelItemCount() != 0 {
					t.Error("the history was read before it was on show")
				}
			})
		})
	}
}

func TestOpeningAnotherRepositoryStaysOnTheTab(t *testing.T) {
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
		if w.tabs.CurrentIndex() != 0 {
			t.Errorf("on tab %d", w.tabs.CurrentIndex())
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
		w.tabs.SetCurrentIndex(historyTabIndex)
	})
	waitFor(t, "the commits", func() bool { return h.commits.TopLevelItemCount() == 2 })
	// The newest is chosen, and its files and diff shown.
	waitFor(t, "the files", func() bool { return h.files.Count() == 3 })
	on(func() {
		if row := treeRow(h, 0); !strings.Contains(row[0], "Two files") || !strings.Contains(row[0], "main") || row[1] != "Test" || row[2] == "" || row[3] == "" {
			t.Errorf("row %q", row)
		}
		if got := listTexts(h.files); !slices.Equal(got, []string{"Commit message", "A  a", "A  b"}) {
			t.Errorf("files %q", got)
		}
		if !strings.Contains(h.diff.text, "The body") || !strings.Contains(h.diff.text, "+b") {
			t.Errorf("diff %q", h.diff.text)
		}
		// Each file is headed by its name alone.
		if strings.Contains(h.diff.text, "diff --git") || strings.Contains(h.diff.text, "+++") {
			t.Errorf("diff %q", h.diff.text)
		}
		for i, line := range h.diff.fileLines() {
			name := []string{"a", "b"}[i]
			if got := h.diff.Document().FindBlockByNumber(line).Text(); got != name {
				t.Errorf("heading %q, not %q", got, name)
			}
		}
		if h.fileLabel.Text() != "Files (2)" {
			t.Errorf("label %q", h.fileLabel.Text())
		}
		// Choosing a file goes to it in the diff, and the first row back.
		starts := h.diff.fileLines()
		if len(starts) != 2 {
			t.Fatalf("file lines %v", starts)
		}
		h.files.SetCurrentRow(2)
		if got := h.diff.VerticalScrollBar().Value(); got != min(starts[1], h.diff.VerticalScrollBar().Maximum()) {
			t.Errorf("scrolled to %d, file at %d", got, starts[1])
		}
		h.files.SetCurrentRow(0)
		if got := h.diff.VerticalScrollBar().Value(); got != 0 {
			t.Errorf("scrolled to %d", got)
		}
		// Choosing another commit shows that one.
		h.commits.SetCurrentItem(h.commits.TopLevelItem(1))
	})
	waitFor(t, "the first commit", func() bool { return strings.Contains(h.diff.text, "first") && h.files.Count() == 2 })
}

func TestHistoryFollowsChanges(t *testing.T) {
	dir := historyRepo(t, 1)
	w := newTestWindow(t, dir)
	h := w.history
	on(func() { w.tabs.SetCurrentIndex(historyTabIndex) })
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

	// One made on the other tab shows on coming back.
	writeFile(t, dir, "file", "changed\n")
	on(func() {
		w.tabs.SetCurrentIndex(0)
		w.rescan.Trigger()
	})
	waitFor(t, "the change", func() bool { return w.idleWith(1, 0) })
	on(func() { w.stageAll.Trigger() })
	waitFor(t, "staged", func() bool { return w.idleWith(0, 1) })
	on(func() {
		w.tab.message.SetPlainText("From the tab")
		w.commit.Trigger()
	})
	waitFor(t, "committed", func() bool { return w.idleWith(0, 0) })
	on(func() {
		if h.commits.TopLevelItemCount() != 3 {
			t.Error("the history was read while not on show")
		}
		w.tabs.SetCurrentIndex(historyTabIndex)
	})
	waitFor(t, "the commit from the tab", func() bool {
		return h.commits.TopLevelItemCount() == 4 && strings.Contains(treeRow(h, 0)[0], "From the tab")
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
		w.tabs.SetCurrentIndex(historyTabIndex)
	})
	// It comes first and is chosen, on top of the first page of real commits.
	waitFor(t, "the rows", func() bool { return h.commits.TopLevelItemCount() == 3 && !h.loading })
	waitFor(t, "its files", func() bool { return h.files.Count() == 3 })
	on(func() {
		if row := treeRow(h, 0); row[0] != "Uncommitted changes" || row[3] != "" {
			t.Errorf("row %q", row)
		}
		if !h.commits.TopLevelItem(0).Font(colSubject).Italic() {
			t.Error("it is not in italics")
		}
		if got := listTexts(h.files); !slices.Equal(got, []string{"All changes", "M  file", "?  untracked"}) {
			t.Errorf("files %q", got)
		}
		if !strings.Contains(h.diff.text, "+changed") || !strings.Contains(h.diff.text, "+untracked") {
			t.Errorf("diff %q", h.diff.text)
		}
		if h.diffLabel.Text() != "Uncommitted changes" {
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

	// A new change to it shows on a rescan, staying where it is.
	writeFile(t, dir, "later", "later\n")
	on(func() { w.rescan.Trigger() })
	waitFor(t, "the new file", func() bool { return h.files.Count() == 4 && strings.Contains(h.diff.text, "+later") })

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
		w.tabs.SetCurrentIndex(historyTabIndex)
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
		w.tabs.SetCurrentIndex(historyTabIndex)
	})
	waitFor(t, "the commits", func() bool { return h.commits.TopLevelItemCount() == 4 })
	on(func() {
		if len(h.graph.rows) != 4 || h.layout.Width() != 2 {
			t.Fatalf("%d rows, %d lanes", len(h.graph.rows), h.layout.Width())
		}
		if got, want := h.commits.Header().SectionSize(colGraph), graphWidth(2); got != want {
			t.Errorf("column %d wide, not %d", got, want)
		}
		// The merge's node shows, in the colour of its lane, in a row with room.
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
	on(func() { w.tabs.SetCurrentIndex(historyTabIndex) })
	waitFor(t, "the commits", func() bool { return h.files.Count() == 2 })
	on(func() {
		if err := w.openRepository(empty); err != nil {
			t.Fatal(err)
		}
	})
	waitFor(t, "nothing shown", func() bool {
		return w.repo.Root != "" && !h.loading && h.commits.TopLevelItemCount() == 0 && h.files.Count() == 0 && h.diff.text == ""
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
		again.tabs.SetCurrentIndex(historyTabIndex)
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
		if d.WindowTitle() != "About Merge Monkey" {
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
	list := w.tab.unstaged
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
	waitFor(t, "the first file", func() bool { return strings.Contains(w.tab.diff.text, "text of a") })
	for row, name := range []string{"a", "b", "c"} {
		on(func() { sendMouse(list, qt.QEvent__MouseMove, row, qt.NoButton, qt.LeftButton, qt.NoModifier) })
		waitFor(t, "the file dragged over", func() bool { return strings.Contains(w.tab.diff.text, "text of "+name) })
		on(func() {
			if got := selected(); !slices.Equal(got, []int{row}) {
				t.Errorf("over row %d, selected %v", row, got)
			}
			if w.tab.diffLabel.Text() != "Unstaged: "+name {
				t.Errorf("label %q", w.tab.diffLabel.Text())
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
