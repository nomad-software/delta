package ui

import (
	"fmt"
	"slices"
	"time"

	"github.com/nomad-software/qute"
	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtwidgets"

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
// on: the commits above, and below them the files of the commit
// chosen beside its diff.
type historyView struct {
	page *qtwidgets.QWidget
	// split divides the commits from what is below them, and detail the files
	// from the diff.
	split, detail *qtwidgets.QSplitter

	commits *qtwidgets.QTreeWidget
	// graph draws the tree of commits, and layout works out where.
	graph     *graphColumn
	layout    *graph.Layout
	files     *qtwidgets.QListWidget
	diff      *diffView
	diffLabel *qtwidgets.QLabel
	fileLabel *qtwidgets.QLabel

	// list is what the commits show, which starts with the uncommitted changes
	// if there are any.
	list []git.Commit
	// shown is the commit whose files are listed below, with no hash if none
	// is, and shownChanges its files. shownPath is the file whose diff is
	// shown, kept chosen when the same commit's files are read again.
	shown        git.Commit
	shownChanges []git.Change
	shownPath    string

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
	// logs, details and fileDiffs count the requests for each, so that the answer to any
	// but the latest is dropped.
	logs, details, fileDiffs int
}

func newHistoryView(config settings.Settings) *historyView {
	h := &historyView{page: qtwidgets.NewQWidget(nil), pageSize: config.HistoryPageSize, stale: true}

	h.commits = qtwidgets.NewQTreeWidget(nil)
	h.commits.SetHeaderLabels([]string{"Graph", "Subject", "Author", "Date", "Commit"})
	h.commits.SetRootIsDecorated(false)
	h.commits.SetUniformRowHeights(true)
	h.commits.SetAllColumnsShowFocus(true)
	h.commits.SetSelectionMode(qtwidgets.QAbstractItemViewSingleSelection)
	header := h.commits.Header()
	header.SetStretchLastSection(false)
	h.graph = newGraphColumn(h.commits)
	h.commits.SetItemDelegateForColumn(colGraph, h.graph)
	header.SetSectionResizeMode(colGraph, qtwidgets.QHeaderViewFixed)
	h.resetGraph()
	header.SetSectionResizeMode(colSubject, qtwidgets.QHeaderViewStretch)
	header.ResizeSection(colAuthor, 170)
	header.ResizeSection(colDate, 140)
	header.ResizeSection(colCommit, 90)

	h.fileLabel = heading("Files")
	h.files = qtwidgets.NewQListWidget(nil)
	h.files.SetUniformItemSizes(true)
	setChangeIconSize(h.files)

	h.diffLabel = heading("Diff")
	h.diffLabel.SetTextFormat(qtcore.PlainText)
	h.diff = newDiffView(config.AddedColour, config.RemovedColour, config.HunkColour)

	h.detail = newPaneSplitter(qtcore.Horizontal)
	h.detail.AddWidget(labelled(h.fileLabel, h.files))
	h.detail.AddWidget(labelled(h.diffLabel, h.diff))
	h.detail.SetStretchFactor(1, 1)

	h.split = newPaneSplitter(qtcore.Vertical)
	h.split.AddWidget(labelled(heading("Commits"), h.commits))
	h.split.AddWidget(h.detail)
	h.split.SetStretchFactor(1, 1)
	for _, s := range []*qtwidgets.QSplitter{h.split, h.detail} {
		s.SetChildrenCollapsible(false)
	}

	layout := qtwidgets.NewQVBoxLayout(h.page)
	layout.SetContentsMargins(margins(pageMargin))
	layout.AddWidget(h.split)
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
	h.commits.SetContextMenuPolicy(qtcore.CustomContextMenu)
	h.commits.OnCustomContextMenuRequested(func(pos qtcore.QPoint) {
		row := h.commits.IndexOfTopLevelItem(h.commits.ItemAt(pos))
		if row < 0 || row >= len(h.list) {
			return
		}
		menu := w.commitMenu(h.list[row])
		defer menu.DeleteLater()
		menu.ExecAt(h.commits.Viewport().MapToGlobal(pos))
	})
	// More are read on reaching the end of those read, or while it is in
	// sight, as when they all fit.
	bar := h.commits.VerticalScrollBar()
	bar.OnValueChanged(func(int) { w.fillHistory() })
	bar.OnRangeChanged(func(int, int) { w.fillHistory() })
	// The diff is of the file chosen, as in the changes.
	h.files.OnCurrentRowChanged(func(int) {
		if !h.updating {
			w.showFile()
		}
	})
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
func (w *Window) commitMenu(c git.Commit) *qtwidgets.QMenu {
	menu := qtwidgets.NewQMenu(w)
	rebase := menu.AddActionText("&Rebase from here…")
	rebase.SetEnabled(w.repo != nil && !w.busy && c.Hash != git.Uncommitted)
	rebase.OnTriggered(func(bool) { w.rebaseFrom(c) })
	return menu
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
	h.fileDiffs++
	h.more, h.loading = false, false
	h.updating = true
	h.commits.Clear()
	h.files.Clear()
	h.updating = false
	h.list, h.shown, h.shownChanges, h.shownPath = nil, git.Commit{}, nil, ""
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
		qute.Post(func() {
			w.endWork()
			if !w.historyRead(request, r, err) {
				return
			}
			h.more = more
			w.showCommits(page)
			// Commits the same as before leave the scroll bar as it was,
			// which may be at an end that a page dropped for this was
			// to follow.
			w.fillHistoryNow()
		})
	}()
}

// fillHistoryNow is fillHistory once rows just added are laid out, which Qt
// otherwise does later, so that the scroll bar is not taken as at the end of
// rows that will no longer fit, which would read one page after another.
func (w *Window) fillHistoryNow() {
	if w.historyShown() {
		w.history.commits.DoItemsLayout()
		w.fillHistory()
	}
}

// fillHistory reads the next commits if the end of those read is in sight, and
// there are more, while the history is on show.
func (w *Window) fillHistory() {
	bar := w.history.commits.VerticalScrollBar()
	if w.historyShown() && bar.Value() == bar.Maximum() {
		w.loadMoreHistory()
	}
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
		qute.Post(func() {
			w.endWork()
			if !w.historyRead(request, r, err) {
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
			// Rows that all fit leave the scroll bar as it was.
			w.fillHistoryNow()
		})
	}()
}

// historyRead takes in the answer to a read of the history, and says whether
// what it read is to be shown: not if it is not the answer to the latest read
// of the repository open, which is dropped, nor if it failed, which is told of.
func (w *Window) historyRead(request int, r *git.Repo, err error) bool {
	h := w.history
	if request != h.logs || r != w.repo {
		return false
	}
	h.loading = false
	if err != nil {
		w.historyFailed(err)
		return false
	}
	return true
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
// first if there are any (git.Repo.History), and lays them out, all in the
// background. more says that there may be more commits than were read.
func readHistory(r *git.Repo, limit int) (page historyPage, more bool, err error) {
	commits, more, err := r.History(limit)
	if err != nil {
		return historyPage{}, false, err
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
func commitItems(page historyPage) []*qtwidgets.QTreeWidgetItem {
	items := make([]*qtwidgets.QTreeWidgetItem, len(page.commits))
	for i, c := range page.commits {
		items[i] = qtwidgets.NewQTreeWidgetItemWithStrings(page.texts[i])
		if c.Hash == git.Uncommitted {
			italic(items[i])
		}
	}
	return items
}

// italic puts the row of the uncommitted changes in italics, as they are not a
// commit.
func italic(item *qtwidgets.QTreeWidgetItem) {
	font := item.Font(colSubject)
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

// showCommit lists the files of the commit chosen, read in the background, and
// chooses one, which shows its diff: the one shown before, if the same commit's
// files are read again, as on a rescan, or else the first.
func (w *Window) showCommit() {
	h, r := w.history, w.repo
	h.details++
	request := h.details
	c := h.chosen()
	if c == nil || r == nil {
		w.showNoCommit()
		return
	}
	commit := *c
	w.beginWork()
	go func() {
		changes, err := r.CommitFiles(commit)
		qute.Post(func() {
			w.endWork()
			if request != h.details || r != w.repo {
				return
			}
			if err != nil {
				w.showNoCommit()
				h.diffLabel.SetText(commitName(commit))
				h.diff.show(err.Error())
				return
			}
			same := commit.Hash == h.shown.Hash
			if !same || !slices.Equal(changes, h.shownChanges) {
				h.setFiles(changes)
			}
			kept := ""
			if same {
				kept = h.shownPath
			}
			h.shown, h.shownChanges = commit, changes
			row := max(slices.IndexFunc(changes, func(c git.Change) bool { return c.Path == kept }), 0)
			h.updating = true
			h.files.SetCurrentRow(row)
			h.updating = false
			// It is read again even if the same file stays chosen, as what is
			// uncommitted changes.
			w.showFile()
		})
	}()
}

// showNoCommit lists no files, and shows no diff, as when no commit is chosen.
func (w *Window) showNoCommit() {
	h := w.history
	h.setFiles(nil)
	h.shown, h.shownChanges = git.Commit{}, nil
	w.showFile()
}

// commitName names a commit above the diff of one of its files.
func commitName(c git.Commit) string {
	if c.Hash == git.Uncommitted {
		return "Uncommitted"
	}
	return "Commit " + c.Short
}

// showFile shows the diff of the file chosen among the commit's, read and
// parsed in the background, or nothing if none is.
func (w *Window) showFile() {
	h, r := w.history, w.repo
	h.fileDiffs++
	request := h.fileDiffs
	row := h.files.CurrentRow()
	if r == nil || h.shown.Hash == "" || row < 0 || row >= len(h.shownChanges) {
		h.shownPath = ""
		h.diffLabel.SetText("Diff")
		h.diff.show("")
		return
	}
	commit, change := h.shown, h.shownChanges[row]
	h.shownPath = change.Path
	w.loadDiff(func() (string, error) { return r.CommitFileDiff(commit, change) },
		func() bool { return request == h.fileDiffs && r == w.repo },
		h.diffLabel, commitName(commit)+": "+change.Path, h.diff)
}

// setFiles lists the files of a commit. Nil changes clear the list, where an
// empty list is a commit that changed nothing.
func (h *historyView) setFiles(changes []git.Change) {
	h.updating = true
	defer func() { h.updating = false }()
	h.files.Clear()
	h.fileLabel.SetText("Files")
	if changes == nil {
		return
	}
	h.fileLabel.SetText(fmt.Sprintf("Files (%d)", len(changes)))
	// What happened to each file is said in words when the pointer is over it.
	for _, c := range changes {
		item := newChangeItem(c)
		item.SetToolTip(explain(c))
		h.files.AddItem(item)
	}
}
