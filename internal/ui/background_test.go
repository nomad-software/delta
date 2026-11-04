package ui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nomad-software/delta/internal/diff"
	"github.com/nomad-software/delta/internal/git"
	"github.com/nomad-software/delta/internal/graph"
)

// What a dialog needs is read in the background while the main thread goes on,
// once at a time, and given to it on the main thread.
func TestReadFirst(t *testing.T) {
	w := newTestWindow(t, newRepo(t))
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })
	release := make(chan struct{})
	var reads atomic.Int32
	var given int
	read := func(r *git.Repo) (string, error) {
		reads.Add(1)
		<-release
		return r.Root, nil
	}
	var got string
	on(func() {
		readFirst(w.Window, "Read", read, func(root string) { given++; got = root })
		// Asked again while it is read, nothing more is.
		readFirst(w.Window, "Read", read, func(string) { given++ })
	})
	// The main thread is free while it is read.
	on(func() {
		if !w.reading || given != 0 {
			t.Errorf("reading %v, given %d", w.reading, given)
		}
	})
	close(release)
	waitFor(t, "the read", func() bool { return !w.reading && w.working == 0 })
	on(func() {
		if reads.Load() != 1 || given != 1 || got != w.repo.Root {
			t.Errorf("%d reads, given %d, %q", reads.Load(), given, got)
		}
	})
}

// What is read for a dialog is dropped if another repository is opened or a
// command started meanwhile, and a failure is told of.
func TestReadFirstDropped(t *testing.T) {
	one, two := namedRepo(t, "one"), namedRepo(t, "two")
	w := newTestWindow(t, one)
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })
	given := 0
	then := func(string) { given++ }
	wait := func(release chan struct{}) func(*git.Repo) (string, error) {
		return func(*git.Repo) (string, error) {
			<-release
			return "", nil
		}
	}

	opened := make(chan struct{})
	on(func() {
		readFirst(w.Window, "Read", wait(opened), then)
		if err := w.openRepository(two); err != nil {
			t.Error(err)
		}
	})
	close(opened)
	waitFor(t, "the read", func() bool { return !w.reading })
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })

	started, command := make(chan struct{}), make(chan struct{})
	on(func() {
		readFirst(w.Window, "Read", wait(started), then)
		w.background("Slow", func() (string, error) {
			<-command
			return "", nil
		}, func(string, error) {})
	})
	close(started)
	waitFor(t, "the read", func() bool { return !w.reading })
	close(command)
	waitFor(t, "the command", func() bool { return !w.busy })

	on(func() {
		readFirst(w.Window, "Read", func(*git.Repo) (string, error) { return "", errors.New("refused") }, then)
	})
	waitFor(t, "the read", func() bool { return !w.reading })
	on(func() {
		if given != 0 {
			t.Errorf("given %d times", given)
		}
		if len(w.failures) != 1 || w.failures[0] != "refused" {
			t.Errorf("failures %q", w.failures)
		}
	})
}

// A rebase is refused, from what is read in the background, as git refuses it,
// before any dialog.
func TestRebaseRefusedInTheBackground(t *testing.T) {
	dir := newRepo(t)
	commitFiles(t, dir, "a")
	runGit(t, dir, "switch", "-c", "topic", "HEAD~1")
	commitFiles(t, dir, "b")
	runGit(t, dir, "switch", "main")
	runGit(t, dir, "merge", "--no-ff", "-m", "Merge", "topic")
	w := newTestWindow(t, dir)
	waitFor(t, "the status", func() bool { return w.idleWith(0, 0) })
	on(func() {
		commits, _ := w.repo.Log(10, 0)
		w.rebaseFrom(commits[len(commits)-1])
	})
	waitFor(t, "the commits", func() bool { return !w.reading })
	on(func() {
		if len(w.failures) != 1 || !strings.HasPrefix(w.failures[0], "Commit ") || !strings.HasSuffix(w.failures[0], ".") {
			t.Errorf("failures %q", w.failures)
		}
	})
}

// Pages of the history laid out in the background, each in a copy of the
// layout, make the graph one layout of all of them would.
func TestHistoryPagesJoinUp(t *testing.T) {
	dir := historyRepo(t, 3)
	runGit(t, dir, "switch", "-c", "topic", "HEAD~2")
	for i := range 3 {
		writeFile(t, dir, fmt.Sprintf("topic%d", i), "topic\n")
		runGit(t, dir, "add", ".")
		runGit(t, dir, "commit", "-m", fmt.Sprintf("Topic %d", i))
	}
	runGit(t, dir, "switch", "main")
	runGit(t, dir, "merge", "--no-ff", "-m", "Merge topic", "topic")
	writeFile(t, dir, "file", "uncommitted\n")
	w := newTestWindow(t, dir)
	h := w.history
	on(func() {
		h.pageSize = 2
		w.showHistory.SetChecked(true)
	})
	waitFor(t, "the first page", func() bool { return len(h.list) == 3 && !h.loading })
	for more := true; more; {
		on(func() {
			w.loadMoreHistory()
			more = h.more
		})
		waitFor(t, "the next page", func() bool { return !h.loading })
	}
	on(func() {
		whole := layOutPage(&graph.Layout{}, h.list)
		// Eight commits and the uncommitted changes.
		if len(h.graph.rows) != len(whole.rows) || len(h.list) != 9 {
			t.Errorf("%d rows of %d commits", len(h.graph.rows), len(h.list))
			return
		}
		for i, row := range h.graph.rows {
			want := whole.rows[i]
			if row.Lane != want.Lane || row.Colour != want.Colour || !slices.Equal(row.Top, want.Top) || !slices.Equal(row.Bottom, want.Bottom) {
				t.Errorf("row %d is %+v, not %+v", i, row, want)
			}
			if got := treeRow(h, i); got[0] != whole.texts[i][colSubject] || got[3] != whole.texts[i][colCommit] {
				t.Errorf("row %d says %q", i, got)
			}
		}
		if h.layout.Width() != whole.layout.Width() {
			t.Errorf("%d lanes, not %d", h.layout.Width(), whole.layout.Width())
		}
	})
}

// A next page of the history that comes back after the history was read again
// is dropped, leaving the layout as the new read has it.
func TestHistoryPageDroppedAfterReload(t *testing.T) {
	dir := historyRepo(t, 4)
	w := newTestWindow(t, dir)
	h := w.history
	on(func() {
		h.pageSize = 2
		w.showHistory.SetChecked(true)
	})
	waitFor(t, "the first page", func() bool { return len(h.list) == 2 && !h.loading })
	var layout *graph.Layout
	on(func() {
		w.loadMoreHistory()
		w.loadHistory()
	})
	waitFor(t, "the reads", func() bool { return !h.loading && w.working == 0 })
	on(func() {
		layout = h.layout
		if len(h.list) != 2 || len(h.graph.rows) != 2 || h.commits.TopLevelItemCount() != 2 {
			t.Errorf("%d commits, %d rows, %d items", len(h.list), len(h.graph.rows), h.commits.TopLevelItemCount())
		}
		if whole := layOutPage(&graph.Layout{}, h.list); layout.Width() != whole.layout.Width() {
			t.Error("the layout is not the new read's")
		}
	})
}

// Diffs parsed in the background are shown as those parsed by the view itself.
func TestDiffsParsedInTheBackground(t *testing.T) {
	dir := historyRepo(t, 2)
	writeFile(t, dir, "file", "changed\n")
	w := newTestWindow(t, dir)
	waitFor(t, "the diff", func() bool { return strings.Contains(w.changes.diff.raw, "+changed") })
	on(func() {
		v := w.changes.diff
		if want := diff.Parse(v.raw); !slices.Equal(v.parsed.Kinds, want.Kinds) || v.parsed.Text != want.Text || v.ToPlainText() != want.Text {
			t.Errorf("parsed %q", v.parsed.Text)
		}
		w.showHistory.SetChecked(true)
	})
	h := w.history
	waitFor(t, "the file's diff", func() bool { return h.shownPath != "" && h.diff.raw != "" })
	on(func() {
		want := diff.Parse(h.diff.raw)
		if !slices.Equal(h.diff.parsed.Kinds, want.Kinds) || h.diff.parsed.Text != want.Text || h.diff.ToPlainText() != want.Text {
			t.Errorf("parsed %q", h.diff.parsed.Text)
		}
	})
}

// Of files chosen one after another before their diffs are read, the last
// chosen is shown, whatever order the diffs come back in.
func TestLastFileChosenIsShown(t *testing.T) {
	dir := newRepo(t)
	// The files chosen before the last are large, so that their diffs come
	// back after its.
	for _, name := range []string{"a", "b", "c"} {
		text := "text of " + name + "\n"
		if name != "b" {
			text += strings.Repeat(name+" is large\n", 200000)
		}
		writeFile(t, dir, name, text)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "Three files")
	w := newTestWindow(t, dir)
	h := w.history
	on(func() { w.showHistory.SetChecked(true) })
	waitFor(t, "the files", func() bool { return h.files.Count() == 3 && h.diff.raw != "" })
	on(func() {
		for _, row := range []int{2, 0, 2, 1} {
			h.files.SetCurrentRow(row)
		}
	})
	waitFor(t, "the diffs", func() bool { return w.working == 0 })
	on(func() {
		if !strings.HasSuffix(h.diffLabel.Text(), ": b") || !strings.Contains(h.diff.parsed.Text, "+text of b") || h.shownPath != "b" {
			t.Errorf("label %q, diff %q", h.diffLabel.Text(), h.diff.parsed.Text)
		}
	})
}
