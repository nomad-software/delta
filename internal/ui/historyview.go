package ui

import (
	"cmp"
	"fmt"
	"slices"
	"sync"
	"time"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"

	"github.com/nomad-software/delta/internal/diff"
	"github.com/nomad-software/delta/internal/git"
	"github.com/nomad-software/delta/internal/graph"
	"github.com/nomad-software/delta/internal/settings"
)

// Where the changes and the history are among the window's views.
const (
	changesIndex = iota
	historyIndex
)

// The columns of the list of commits.
const (
	colGraph = iota
	colSubject
	colAuthor
	colDate
	colCommit
)

// historyView is the history, shown in place of the changes while History is
// on, as in gitk: the commits above, and below them the files of the commit
// chosen beside its diff.
type historyView struct {
	page *qt.QWidget
	// split divides the commits from what is below them, and detail the files
	// from the diff.
	split, detail *qt.QSplitter

	commits *qt.QTreeWidget
	// graph draws the tree of commits, and layout works out where.
	graph     *graphColumn
	layout    *graph.Layout
	files     *qt.QListWidget
	diff      *diffView
	diffLabel *qt.QLabel
	fileLabel *qt.QLabel

	// list is what the commits show, which starts with the uncommitted changes
	// if there are any.
	list []git.Commit
	// shownHash and shownChanges are the hash and files of what is shown below.
	shownHash    string
	shownChanges []git.Change

	// pageSize is how many commits are read at a time, as the settings say.
	pageSize int
	// more says that there may be commits beyond those read, and loading that
	// some are being read.
	more, loading bool
	// stale says that the repository may have changed since it was read, and
	// sized that it has been on show, so that its panes have sizes.
	stale, sized bool
	// updating says that the program is changing a list, so that its selection
	// changing means nothing.
	updating bool
	// logs and details count the requests for each, so that the answer to any
	// but the latest is dropped.
	logs, details int
}

func newHistoryView(config settings.Settings) *historyView {
	h := &historyView{page: qt.NewQWidget2(), pageSize: config.HistoryPageSize, stale: true}

	h.commits = qt.NewQTreeWidget2()
	h.commits.SetHeaderLabels([]string{"Graph", "Subject", "Author", "Date", "Commit"})
	h.commits.SetRootIsDecorated(false)
	h.commits.SetUniformRowHeights(true)
	h.commits.SetAllColumnsShowFocus(true)
	h.commits.SetSelectionMode(qt.QAbstractItemView__SingleSelection)
	header := h.commits.Header()
	header.SetStretchLastSection(false)
	h.graph = newGraphColumn(h.commits.QObject)
	h.commits.SetItemDelegateForColumn(colGraph, h.graph.QAbstractItemDelegate)
	header.SetSectionResizeMode2(colGraph, qt.QHeaderView__Fixed)
	h.resetGraph()
	header.SetSectionResizeMode2(colSubject, qt.QHeaderView__Stretch)
	header.ResizeSection(colAuthor, 170)
	header.ResizeSection(colDate, 140)
	header.ResizeSection(colCommit, 90)

	h.fileLabel = heading("Files")
	h.files = qt.NewQListWidget2()
	h.files.SetUniformItemSizes(true)
	setChangeIconSize(h.files)

	h.diffLabel = heading("Diff")
	h.diffLabel.SetTextFormat(qt.PlainText)
	h.diff = newDiffView(config.AddedColour, config.RemovedColour, config.HunkColour)
	h.diff.titles = true

	h.detail = newPaneSplitter(qt.Horizontal)
	h.detail.AddWidget(labelled(h.fileLabel, h.files.QWidget))
	h.detail.AddWidget(labelled(h.diffLabel, h.diff.QWidget))
	h.detail.SetStretchFactor(1, 1)

	h.split = newPaneSplitter(qt.Vertical)
	h.split.AddWidget(labelled(heading("Commits"), h.commits.QWidget))
	h.split.AddWidget(h.detail.QWidget)
	h.split.SetStretchFactor(1, 1)
	for _, s := range []*qt.QSplitter{h.split, h.detail} {
		s.SetChildrenCollapsible(false)
	}

	layout := qt.NewQVBoxLayout(h.page)
	layout.SetContentsMargins(pageMargin, pageMargin, pageMargin, pageMargin)
	layout.AddWidget(h.split.QWidget)
	return h
}

// connectHistory joins the widgets of the history to what they do.
func (w *Window) connectHistory() {
	h := w.history
	h.commits.OnItemSelectionChanged(func() {
		if !h.updating {
			w.showCommit()
		}
	})
	h.commits.SetContextMenuPolicy(qt.CustomContextMenu)
	h.commits.OnCustomContextMenuRequested(func(pos *qt.QPoint) {
		row := h.commits.IndexOfTopLevelItem(h.commits.ItemAt(pos))
		if row < 0 || row >= len(h.list) {
			return
		}
		menu := w.commitMenu(h.list[row])
		defer menu.DeleteLater()
		menu.ExecWithPos(h.commits.Viewport().MapToGlobalWithQPoint(pos))
	})
	// More are read on reaching the end of those read.
	bar := h.commits.VerticalScrollBar()
	bar.OnValueChanged(func(value int) {
		if value == bar.Maximum() {
			w.loadMoreHistory()
		}
	})
	jump := func(row int) {
		if !h.updating {
			h.jumpToFile(row)
		}
	}
	// Moving by keyboard changes the row, and clicking the same one again goes
	// back to its place in the diff.
	h.files.OnCurrentRowChanged(jump)
	h.files.OnItemClicked(func(item *qt.QListWidgetItem) { jump(h.files.Row(item)) })
}

// setHistoryShown shows the history in place of the changes, or the changes
// again, as the History button says.
func (w *Window) setHistoryShown(on bool) {
	h := w.history
	if !on {
		w.views.SetCurrentIndex(changesIndex)
		return
	}
	w.views.SetCurrentIndex(historyIndex)
	// Sizes are only taken as they stand once the panes are on show.
	if !h.sized {
		h.sized = true
		setPanes(h.split, w.config.History)
		setPanes(h.detail, w.config.HistoryDetail)
	}
	if h.stale {
		w.loadHistory()
	}
}

// historyShown says whether the history is on show.
func (w *Window) historyShown() bool {
	return w.views.CurrentIndex() == historyIndex
}

// commitMenu returns the menu of a commit in the history, which rebases from it.
// The uncommitted changes are not a commit to rebase from.
func (w *Window) commitMenu(c git.Commit) *qt.QMenu {
	menu := qt.NewQMenu(w.QWidget)
	rebase := menu.AddActionWithText("&Rebase from here…")
	rebase.SetEnabled(w.repo != nil && !w.busy && c.Hash != git.Uncommitted)
	rebase.OnTriggered(func() { w.rebaseFrom(c) })
	return menu
}

// jumpToFile scrolls the diff to the file on a row of the files, of which the
// first is the commit's message, at the top. The diffs are in the order of the
// files. A file past where a large diff was cut off has nowhere to go.
func (h *historyView) jumpToFile(row int) {
	if row < 0 {
		return
	}
	if row == 0 {
		h.diff.scrollToLine(0)
		return
	}
	if starts := h.diff.parsed.Files; row-1 < len(starts) {
		h.diff.scrollToLine(starts[row-1])
	}
}

// markHistoryStale notes that the repository may have changed, and reads it
// again if the history is on show.
func (w *Window) markHistoryStale() {
	w.history.stale = true
	if w.historyShown() {
		w.loadHistory()
	}
}

// clearHistory forgets what is shown, as when another repository is opened.
func (w *Window) clearHistory() {
	h := w.history
	h.logs++
	h.details++
	h.more, h.loading = false, false
	h.updating = true
	h.commits.Clear()
	h.files.Clear()
	h.updating = false
	h.list, h.shownHash, h.shownChanges = nil, "", nil
	h.resetGraph()
	h.diff.show("")
	h.fileLabel.SetText("Files")
	h.diffLabel.SetText("Diff")
}

// loadHistory reads the commits again, as many as are shown now, in the
// background.
func (w *Window) loadHistory() {
	h, r := w.history, w.repo
	if r == nil {
		return
	}
	h.stale = false
	h.loading = true
	h.logs++
	request := h.logs
	limit := max(h.pageSize, h.committed())
	w.beginWork()
	go func() {
		page, more, err := readHistory(r, limit)
		mainthread.Start(func() {
			w.endWork()
			if request != h.logs || r != w.repo {
				return
			}
			h.loading = false
			if err != nil {
				w.historyFailed(err)
				return
			}
			h.more = more
			w.showCommits(page)
		})
	}()
}

// loadMoreHistory reads the next commits, in the background.
func (w *Window) loadMoreHistory() {
	h, r := w.history, w.repo
	if r == nil || !h.more || h.loading {
		return
	}
	h.loading = true
	h.logs++
	request := h.logs
	skip, limit := h.committed(), h.pageSize
	// The next page is laid out in the background in a copy of the layout, which
	// takes the layout's place once it is back. Nothing else changes the layout
	// meanwhile, as anything that would also counts as a request, dropping this.
	layout := h.layout.Clone()
	w.beginWork()
	go func() {
		commits, err := r.Log(limit, skip)
		var page historyPage
		if err == nil {
			page = layOutPage(layout, commits)
		}
		mainthread.Start(func() {
			w.endWork()
			if request != h.logs || r != w.repo {
				return
			}
			h.loading = false
			if err != nil {
				w.historyFailed(err)
				return
			}
			h.more = len(commits) == limit
			h.list = append(h.list, commits...)
			h.layout = page.layout
			h.graph.rows = append(h.graph.rows, page.rows...)
			h.sizeGraph()
			h.updating = true
			h.commits.AddTopLevelItems(commitItems(page))
			h.updating = false
		})
	}()
}

// historyFailed says why the history could not be read.
func (w *Window) historyFailed(err error) {
	w.say("Reading the history failed: " + firstLine(err.Error()))
}

// committed returns how many real commits have been read.
func (h *historyView) committed() int {
	if len(h.list) > 0 && h.list[0].Hash == git.Uncommitted {
		return len(h.list) - 1
	}
	return len(h.list)
}

// readHistory reads up to limit commits from HEAD, with the uncommitted changes
// first if there are any, and lays them out, all in the background: the log and
// the status at once. more says that there may be more commits than were read.
func readHistory(r *git.Repo, limit int) (page historyPage, more bool, err error) {
	var commits []git.Commit
	var status git.Status
	var logErr, statusErr error
	var wg sync.WaitGroup
	wg.Go(func() { commits, logErr = r.Log(limit, 0) })
	wg.Go(func() { status, statusErr = r.Status() })
	wg.Wait()
	if err := cmp.Or(logErr, statusErr); err != nil {
		return historyPage{}, false, err
	}
	more = len(commits) == limit
	if len(status.Unstaged)+len(status.Staged) > 0 {
		parent := ""
		if len(commits) > 0 {
			parent = commits[0].Hash
		}
		commits = slices.Insert(commits, 0, git.UncommittedCommit(parent))
	}
	return layOutPage(&graph.Layout{}, commits), more, nil
}

// historyPage is commits with what shows them, worked out in the background:
// the rows of the graph, laid out in layout, which goes on from them, and the
// texts of each row's columns.
type historyPage struct {
	commits []git.Commit
	layout  *graph.Layout
	rows    []graph.Row
	texts   [][]string
}

// layOutPage works out how the commits are shown, going on from the layout,
// which it changes and which no one else may be using. It touches nothing of Qt,
// so that it runs in the background.
func layOutPage(layout *graph.Layout, commits []git.Commit) historyPage {
	p := historyPage{commits: commits, layout: layout, rows: make([]graph.Row, len(commits)), texts: make([][]string, len(commits))}
	for i, c := range commits {
		p.rows[i] = layout.Add(c.Hash, c.Parents)
		p.texts[i] = []string{"", commitTitle(c), c.Author, formatDate(c.Date), c.Short}
		if c.Hash == git.Uncommitted {
			p.texts[i] = []string{"", c.Subject, "", "", ""}
		}
	}
	return p
}

// resetGraph starts the graph again with no commits.
func (h *historyView) resetGraph() {
	h.layout = &graph.Layout{}
	h.graph.rows = nil
	h.sizeGraph()
}

// sizeGraph gives the graph's column room for the lanes its rows need.
func (h *historyView) sizeGraph() {
	h.commits.Header().ResizeSection(colGraph, graphWidth(h.layout.Width()))
}

// sameCommit says whether two commits are shown alike. Only the refs of a commit
// can change without its hash changing.
func sameCommit(a, b git.Commit) bool {
	return a.Hash == b.Hash && a.Refs == b.Refs
}

// showCommits shows the commits of the page, leaving the list alone if nothing
// has changed, and keeps the same commit chosen, or else chooses the newest.
func (w *Window) showCommits(page historyPage) {
	h, commits := w.history, page.commits
	if slices.EqualFunc(h.list, commits, sameCommit) && (len(commits) == 0 || h.chosen() != nil) {
		// What is uncommitted changes without its row changing.
		if c := h.chosen(); c != nil && c.Hash == git.Uncommitted {
			w.showCommit()
		}
		return
	}
	var kept string
	if c := h.chosen(); c != nil {
		kept = c.Hash
	}
	scroll := h.commits.VerticalScrollBar().Value()
	h.updating = true
	h.commits.Clear()
	h.list = commits
	h.layout, h.graph.rows = page.layout, page.rows
	h.sizeGraph()
	h.commits.AddTopLevelItems(commitItems(page))
	row := max(slices.IndexFunc(commits, func(c git.Commit) bool { return c.Hash == kept }), 0)
	if len(commits) > 0 {
		h.commits.SetCurrentItem(h.commits.TopLevelItem(row))
	}
	h.commits.VerticalScrollBar().SetValue(scroll)
	h.updating = false
	w.showCommit()
}

// commitItems returns the rows of the commits of the page, from the texts
// worked out for them.
func commitItems(page historyPage) []*qt.QTreeWidgetItem {
	items := make([]*qt.QTreeWidgetItem, len(page.commits))
	for i, c := range page.commits {
		items[i] = qt.NewQTreeWidgetItem2(page.texts[i])
		if c.Hash == git.Uncommitted {
			italic(items[i])
		}
	}
	return items
}

// italic puts the row of the uncommitted changes in italics, as they are not a
// commit.
func italic(item *qt.QTreeWidgetItem) {
	font := qt.NewQFont5(item.Font(colSubject))
	defer font.Delete()
	font.SetItalic(true)
	item.SetFont(colSubject, font)
}

// commitTitle returns the subject of a commit after the refs at it, if any.
func commitTitle(c git.Commit) string {
	if c.Refs != "" {
		return "[" + c.Refs + "] " + c.Subject
	}
	return c.Subject
}

// formatDate returns a time in the local time zone, for a list.
func formatDate(t time.Time) string {
	return t.Local().Format("2006-01-02 15:04")
}

// chosen returns the commit chosen, or nil.
func (h *historyView) chosen() *git.Commit {
	selected := h.commits.SelectedItems()
	if len(selected) == 0 {
		return nil
	}
	row := h.commits.IndexOfTopLevelItem(selected[0])
	if row < 0 || row >= len(h.list) {
		return nil
	}
	return &h.list[row]
}

// showCommit shows the files and diff of the commit chosen, in the background.
func (w *Window) showCommit() {
	h, r := w.history, w.repo
	h.details++
	request := h.details
	c := h.chosen()
	if c == nil || r == nil {
		h.setFiles(nil, "")
		h.diff.show("")
		h.diffLabel.SetText("Diff")
		h.shownHash = ""
		return
	}
	commit, titles := *c, h.diff.titles
	w.beginWork()
	go func() {
		changes, text, err := r.CommitDetail(commit)
		// It is parsed here, so that the main thread only fills the view.
		var parsed diff.Parsed
		if err == nil {
			parsed = diff.Parse(text, titles)
		}
		mainthread.Start(func() {
			w.endWork()
			if request != h.details || r != w.repo {
				return
			}
			first, title := "Commit message", "Commit "+commit.Short
			if commit.Hash == git.Uncommitted {
				first, title = "All changes", commit.Subject
			}
			h.diffLabel.SetText(title)
			if err != nil {
				h.setFiles(nil, "")
				h.shownHash = ""
				h.diff.show(err.Error())
				return
			}
			// The same again, after a rescan, stays as it was, and scrolled.
			same := commit.Hash == h.shownHash
			if !same || !slices.Equal(changes, h.shownChanges) {
				h.setFiles(changes, first)
			}
			h.shownHash, h.shownChanges = commit.Hash, changes
			h.diff.showParsed(text, parsed)
			if !same {
				h.diff.scrollToLine(0)
			}
		})
	}()
}

// setFiles shows the files of a commit, under a row with the text first, which
// goes to the top of the diff. Nil changes clear the list, where an empty list is
// a commit that changed nothing.
func (h *historyView) setFiles(changes []git.Change, first string) {
	h.updating = true
	defer func() { h.updating = false }()
	h.files.Clear()
	h.fileLabel.SetText("Files")
	if changes == nil {
		return
	}
	h.fileLabel.SetText(fmt.Sprintf("Files (%d)", len(changes)))
	// It has a blank in place of a circle, to line up with the files.
	top := qt.NewQListWidgetItem2(first)
	top.SetIcon(changeIcon(0))
	h.files.AddItemWithItem(top)
	// What happened to each file is said in words when the pointer is over it.
	for _, c := range changes {
		item := newChangeItem(c)
		item.SetToolTip(explain(c))
		h.files.AddItemWithItem(item)
	}
}
