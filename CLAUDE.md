# Merge Monkey

A git GUI in Go with Qt 6 through the miqt bindings, modelled on `git gui`. `plan.txt` is the original brief. `../prism` is a reference for the look of the settings, about box and themes: use it as a reference, don't copy from it.

## Rules

* After every change run `make build` and `make install`, and `make vet` and `make test` when code changed. `staticcheck ./...` and `deadcode -test ./...` must be clean.
* Keep this file and README.md up to date with every change.
* Don't commit; the user does that.
* miqt comes from `github.com/mappu/miqt`, at the latest commit of its master branch (`go get github.com/mappu/miqt@master`), not its last tag: "latest" means master. No `replace` directives or local dependencies.
* Delete only what `qt.New…` makes. A value miqt returns by value is freed by its finalizer.
* Qt is only touched on the main thread. Anything that runs git in the background comes back through `mainthread.Start`, and tests use `mainthread.Wait`.

## Packaging

`packaging/` and `.github/workflows/` are adapted from `../prism`: an AppImage built in an Ubuntu 22.04 container with Qt's own 6.9.3, and a macOS app for Apple Silicon with Homebrew's qtbase, both on `v` tags or by hand. The icon is drawn in code (`drawIcon`); `make icons` writes the copies in `packaging/`, and `TestPackagedIconsAreCurrent` says when that is needed. `make screenshots` redraws `docs/`. `main.go` embeds `packaging/merge-monkey.png` for the desktop entry, and `TestPackagedEntryAgrees` keeps `packaging/merge-monkey.desktop` in step with what is written at start.

## Layout

* `internal/git`: runs the git command line. No Qt. Tests use real temporary repositories.
* `internal/diff`: turns git's diff output into what is shown, and what each line is. No Qt.
* `internal/graph`: lays out commits in lanes for the graph. No Qt.
* `internal/desktop`: writes the Linux desktop entry and icon, at every start, only when they differ. No Qt.
* `internal/settings`: the JSON settings, with defaults for unusable values. No Qt.
* `internal/ui`: `window.go` (the window, its menus, reading status and settings), `operations.go` (every git command the menus and buttons run, and the questions before them), `branchtab.go` (the first tab's widgets), `historytab.go` (the second tab, its loading included), `graphcolumn.go` (paints the graph), `diffview.go`, `settingsdialog.go`, `about.go`, `theme.go`, `dialogs.go`, `icon.go`. Tests run with the offscreen platform.

## Behaviour to keep

* Git runs with `--literal-pathspecs`, `GIT_TERMINAL_PROMPT=0`, `GIT_EDITOR=true` and in a new session (`Setsid`), so it can't hang waiting on a terminal. `Setsid` is why this builds for Linux and macOS but not Windows.
* `Window.run` runs one changing command at a time in the background, with the actions disabled, then rescans. Rescans and diffs run in the background too, and an answer that isn't to the latest request is dropped.
* Status comes from `git status --porcelain=v2 --branch -z`. Unstaging always names paths, as a `git reset` without any would abandon a merge in progress.
* Diffs are read up to `git.MaxDiff`. All patches are asked for with `git.patchOptions`, which `diff.Parse` relies on. It drops each file's header (`diff`, `index`, `---`, `+++`, new and deleted file modes, similarity), keeping what is news (renames, mode changes, binary). On the History tab a bold file name replaces it (`diffView.titles`), which file jumping uses (`Parsed.Files`). It also makes sure the text has exactly one block per line, which `diffView.paint` relies on. A hunk's `@@` range is in the Hunk headers colour (`HunkColour`, grey by default) with no background, and any text after it is the normal colour.
* Diffs colour text, not backgrounds (the user's choice): added and removed lines take their colours as they are, through char formats (`diffView.paint`). Nothing in a diff has a background.
* The settings dialog is tabbed, with one tab so far, User interface (an Appearance section, then a Colours table of Name and Colour, as `../prism` has it, with bold section rows and a filled button for each colour). Code font is one row under Appearance, as `../apemail` has it: any font of the system (no monospaced filter: the user wants any), the size inline, and a Default button. The code's font and size (`CodeFont`, `CodeFontSize`) apply to both diffs and the commit message; empty or 0 follows the system's fixed width font.
* Themes: light and dark are Fusion with our own palette; following the system restores the desktop's style and an empty palette. `applyTheme` is not called at startup when following the system.
* Force push is `--force-with-lease`, always asks first, and the Force push checkbox clears after a successful push. The Push action honours the checkbox from menus and button alike.
* The stage button stages everything listed, and is Unstage Changes when everything is staged (`allStaged`). The Stage Changes menu item only stages; Stage Selected and double-click stage only those chosen. Only one list has a selection at a time.
* Pane headings are bold (`heading`), and splitter handles are made through `OnCreateHandle` to paint only a row of grip dots, as `../reghead` does (`newPaneSplitter`).
* The status bar has a size grip and all its text at the left, at 50% opacity: the branch, then the last message, in labels of their own (`Window.say`). Don't use `ShowMessage`, which hides widgets beside it, or `AddPermanentWidget`, which goes on the right.
* The History tab is like gitk: commits of HEAD above, in `--topo-order`, as the graph needs every commit before its parents, (read `historyPageSize` at a time, more on scrolling to the end), the chosen commit's files beside its whole diff below. Choosing a file scrolls to its name in the diff, relying on `git diff-tree --name-status` and `-p` listing files in the same order. Merges are diffed against their first parent. It reads only while on show: a rescan marks it stale (`markHistoryStale`), and it reloads on being shown. Its panes get their sizes when first shown (`historyTab.sized`), and are saved only if they have been.
* While there is anything uncommitted, the first row of the history is a stand-in commit (`git.UncommittedCommit`), whose parent is HEAD so that the graph joins up. It is in `historyTab.list` but read commits are counted without it (`committed`). `CommitDetail` shows it as everything since HEAD plus untracked files. It changes without its row changing, so it is read again on every rescan, keeping its scroll position.
* The graph is a column of the commit list, painted by an item delegate (`graphColumn`) from `graph.Layout`'s rows: only rows on show are painted, and it scrolls with the list. The layout is fed a page at a time. Each row's bottom edges must meet the next row's top edges, which `TestRowsJoinUp` checks on random histories. Changing lanes is an S curve leaving and arriving upright, so that lines run on smoothly between rows. The column is as wide as the lanes need, up to `graphMost`.
* In the file lists, ctrl and shift select several files. Dragging keeps one file selected, moving with the pointer, so each is shown in the diff in turn (`newFileList`).
* Ctrl+1 and Ctrl+2 go to the tabs (`makeTabKeys`). On starting, the first status read picks the tab: History if there is nothing to commit or merge, else Branch (`chooseTab`). It is not done on a rescan or on opening another repository.
* Window size, maximized state and the three splitters are saved on close. Pane sizes are applied after the window is shown.
* A window must not be deleted while a background command may still answer to it, so tests close windows without deleting them.
