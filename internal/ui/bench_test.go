package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtgui"
	"github.com/nomad-software/qute/qtwidgets"

	"github.com/nomad-software/delta/internal/diff"
	"github.com/nomad-software/delta/internal/git"
	"github.com/nomad-software/delta/internal/graph"
	"github.com/nomad-software/delta/internal/settings"
)

// The benchmarks measure what the program asks of Qt most: making the window,
// filling the lists and the diff, laying out and painting the history, and
// painting the window and its icons. Each runs its loop on the main thread.
// Run them with `make bench`, and compare two runs with benchstat.

// benchWindow returns a window shown, with no repository open, and its settings
// kept in a temporary folder.
func benchWindow(b *testing.B) *Window {
	b.Helper()
	path := filepath.Join(b.TempDir(), "settings.json")
	var w *Window
	on(func() {
		w = NewWindow(path)
		w.Show()
	})
	b.Cleanup(func() { on(func() { w.Close() }) })
	return w
}

// benchDiff returns the text of a diff of hunks of lines kept, removed and
// added, of about the lines given.
func benchDiff(lines int) string {
	var text strings.Builder
	text.WriteString("diff --git a/file b/file\nindex 1111111..2222222 100644\n--- a/file\n+++ b/file\n")
	for i := 0; i < lines; i += 10 {
		fmt.Fprintf(&text, "@@ -%d,8 +%d,8 @@ func f%d() {\n", i+1, i+1, i)
		for j := range 3 {
			fmt.Fprintf(&text, " \tkept line %d of hunk %d\n", j, i)
		}
		for j := range 3 {
			fmt.Fprintf(&text, "-\tremoved line %d of hunk %d\n", j, i)
		}
		for j := range 3 {
			fmt.Fprintf(&text, "+\tadded line %d of hunk %d\n", j, i)
		}
	}
	return text.String()
}

// benchChanges returns files changed in every way git has.
func benchChanges(n int, prefix string) []git.Change {
	codes := []byte{'M', 'A', 'D', 'R', 'C', 'T', git.Untracked, git.Conflict}
	changes := make([]git.Change, n)
	for i := range changes {
		changes[i] = git.Change{Path: fmt.Sprintf("%s/folder%d/file%d.go", prefix, i%17, i), Code: codes[i%len(codes)]}
	}
	return changes
}

// benchCommits returns a history of commits on a few branches merged back in
// turn, newest first, with hashes that start with the prefix.
func benchCommits(n int, prefix string) []git.Commit {
	hash := func(i int) string { return fmt.Sprintf("%s%038d", prefix, i) }
	commits := make([]git.Commit, n)
	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range commits {
		c := git.Commit{Hash: hash(i), Short: hash(i)[:7], Author: "Some One", Date: date.Add(-time.Duration(i) * time.Hour), Subject: fmt.Sprintf("Change number %d of the history", i)}
		if i+1 < n {
			c.Parents = []string{hash(i + 1)}
		}
		// Every tenth is a merge of a branch that left five commits before.
		if i%10 == 0 && i+6 < n {
			c.Parents = append(c.Parents, hash(i+6))
		}
		if i%25 == 0 {
			c.Refs = "tag: v1." + fmt.Sprint(i)
		}
		commits[i] = c
	}
	return commits
}

func BenchmarkNewWindow(b *testing.B) {
	path := filepath.Join(b.TempDir(), "settings.json")
	on(func() {
		for range b.N {
			w := NewWindow(path)
			w.Delete()
		}
	})
}

func BenchmarkShowDiff(b *testing.B) {
	text := benchDiff(5000)
	parsed := diff.Parse(text)
	on(func() {
		v := newDiffView(settings.DefaultAdded, settings.DefaultRemoved, settings.DefaultHunk)
		defer v.Delete()
		b.ResetTimer()
		for range b.N {
			v.raw = ""
			v.showParsed(text, parsed)
		}
	})
}

func BenchmarkStatusLists(b *testing.B) {
	w := benchWindow(b)
	statuses := []git.Status{
		{Branch: "main", Unstaged: benchChanges(1000, "a"), Staged: benchChanges(200, "b")},
		{Branch: "main", Unstaged: benchChanges(1000, "c"), Staged: benchChanges(200, "d")},
	}
	on(func() {
		b.ResetTimer()
		for i := range b.N {
			w.applyStatus(statuses[i%2])
		}
	})
}

func BenchmarkHistoryPage(b *testing.B) {
	w := benchWindow(b)
	pages := []historyPage{}
	for _, prefix := range []string{"aa", "bb"} {
		commits := benchCommits(500, prefix)
		pages = append(pages, layOutPage(&graph.Layout{}, commits))
	}
	on(func() {
		w.showHistory.SetChecked(true)
		b.ResetTimer()
		for i := range b.N {
			// The page is laid out again, as it is in the background.
			page := pages[i%2]
			w.showCommits(layOutPage(&graph.Layout{}, page.commits))
		}
	})
}

func BenchmarkPaintHistory(b *testing.B) {
	w := benchWindow(b)
	on(func() {
		w.showHistory.SetChecked(true)
		w.showCommits(layOutPage(&graph.Layout{}, benchCommits(500, "aa")))
		b.ResetTimer()
		for range b.N {
			grab(w.history.commits.Viewport())
		}
	})
}

func BenchmarkPaintWindow(b *testing.B) {
	w := benchWindow(b)
	on(func() {
		w.applyStatus(git.Status{Branch: "main", Unstaged: benchChanges(100, "a"), Staged: benchChanges(20, "b")})
		w.changes.diff.show(benchDiff(200))
		b.ResetTimer()
		for range b.N {
			grab(w)
		}
	})
}

func BenchmarkPaintIcons(b *testing.B) {
	codes := []byte{'M', 'A', 'D', 'R', 'C', 'T', git.Untracked, git.Conflict}
	on(func() {
		b.ResetTimer()
		for range b.N {
			for _, code := range codes {
				iconPixmap(changeIcon(code), changeDotSize)
			}
			for _, shape := range []iconShape{openShape, newBranchShape, checkoutShape, finishShape, rescanShape, pullShape, pushShape, historyShape} {
				icon := toolIcon(shape)
				iconPixmap(icon, toolIconSize)
				icon.Delete()
			}
		}
	})
}

// grab paints the widget into a pixmap, and frees it.
func grab(widget qtwidgets.QWidgetLike) {
	widget.AsQWidget().Grab().Delete()
}

// iconPixmap paints the icon at the size, and frees what it painted.
func iconPixmap(icon *qtgui.QIcon, size int) {
	icon.Pixmap(qtcore.QSize{Width: size, Height: size}).Delete()
}
