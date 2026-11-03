# Delta

A git GUI in Go with Qt 6 through the miqt bindings, modelled on `git gui` and `gitk`. `plan.txt` is the original brief. `../apemail` is the reference for the toolbar, icons, about box and colours, `../reghead` for the pane splitters and the size of the change circles, and `../prism` for the settings dialog and themes: use them as references, don't copy from them.

## Rules

* After every change run `make build` and `make install`, and `make vet` and `make test` when code changed. `staticcheck ./...` and `deadcode -test ./...` must be clean, and `gofmt -l .` empty. Run `make race` too when what goes to the background changes: only the race detector finds a value touched from both sides.
* Keep this file and README.md up to date with every change.
* Every bug found gets a unit test that fails without the fix, so that it can't come back. Check that it does fail without it.
* Don't commit or stage; the user does that. Move and delete files without `git mv` or `git rm`.
* miqt comes from `github.com/mappu/miqt`, at the latest commit of its master branch (`go get github.com/mappu/miqt@master`), not its last tag: "latest" means master. No `replace` directives or local dependencies.
* Delete only what `qt.New…` makes. A value miqt returns by value (a size, a font, a palette) is freed by its finalizer: deleting it frees it twice, which crashes only when the collector runs. Keep it alive with `runtime.KeepAlive` while what it gave is used.
* Qt is only touched on the main thread. Anything that runs git in the background comes back through `mainthread.Start`, and tests use `mainthread.Wait`.
* Test it yourself, offscreen. Never open windows on the user's desktop, inject input, or ask the user to try something; say plainly what could not be checked (a real drag, Wayland's focus) instead.

## Packaging

`packaging/` and `.github/workflows/` are adapted from `../prism`: an AppImage built in an Ubuntu 22.04 container with Qt's own 6.9.3, and a macOS app for Apple Silicon with Homebrew's qtbase, both on `v` tags or by hand.

The icon (three green slanted bars making a triangle) is `internal/appicon/appicon.png`, a 256×256 PNG embedded by the `appicon` package, as apemail's is: the window's, the about box's, and the desktop entry's (installed as a 256 pixel icon). To change it, replace the file with one of the same size (`TestPNG` checks) and run `make icons`, which draws the macOS app's, `packaging/macos/delta.png`, from it within Apple's margin (`drawMacIcon`); `TestPackagedIconsAreCurrent` fails until that is done. The AppImage copies the icon as it is (`appimage.sh`). `make screenshots` redraws `docs/changes.png` and `docs/history.png`. `TestPackagedEntryAgrees` keeps `packaging/delta.desktop` in step with the entry written at start.

## Layout

* `internal/git`: runs the git command line (`git.go`; `rebase.go` for interactive rebases). No Qt. Tests use real temporary repositories.
* `internal/diff`: turns git's diff output into what is shown, and what each line is. No Qt.
* `internal/graph`: lays out commits in lanes for the graph. No Qt.
* `internal/settings`: the JSON settings, with defaults for unusable values. No Qt.
* `internal/appicon`: the program's icon, embedded. No Qt.
* `internal/desktop`: writes the Linux desktop entry and icon, at every start, only when they differ. No Qt.
* `internal/atomicfile`: replaces a file whole, through a temporary file, for the settings and the desktop entry. No Qt.
* `internal/ui`, tested with the offscreen platform:
  * `window.go`: the window, its menus and toolbar, the status bar, reading the status, and the settings.
  * `operations.go`: every git command the menus and buttons run, and the questions before them.
  * `changesview.go`: the changes, as `git gui` has them. `historyview.go`: the history, as `gitk` has it, its loading and its commits' context menu. `graphcolumn.go` paints its graph. `diffview.go` shows a diff in either.
  * `rebasedialog.go`: the plan of an interactive rebase.
  * `dialogs.go`: the small form dialogs and questions, and the Checkout dialog. `settingsdialog.go`, `about.go`.
  * `iconengine.go`: icons drawn each time they are shown (`paintedIcon`). `toolicons.go`: the toolbar's icons. `changeicons.go`: the circles of changed files. `spinner.go`: the toolbar's sign of work. `icon.go`: the program's icon.
  * `theme.go`, and `widgets.go` for what several share: pane splitters and their handles, headings, the code's font, and the rows of changed files.

## Behaviour to keep

### Git

* Git runs with `--literal-pathspecs`, `GIT_TERMINAL_PROMPT=0`, `GIT_EDITOR=true`, `GIT_MERGE_AUTOEDIT=no` and in a new session (`Setsid`), so it can't hang waiting on a terminal, and with `GIT_OPTIONAL_LOCKS=0`, so that its background reads don't get in the way of other git commands. `Setsid` is why this builds for Linux and macOS but not Windows.
* `Window.background` runs one changing command at a time off the main thread, with the actions disabled, then rescans. `Window.run` is that, telling of any failure; deleting a branch uses `background` itself, as a refusal from git becomes a question. Only quick reads that a dialog needs first (branches, remotes, a branch name's validity, a rebase's commits) run on the main thread. Rescans, diffs and the history run in the background too, and an answer that isn't to the latest request is dropped.
* Every piece of background work calls `beginWork` before it goes and `endWork` first thing when it comes back, its answer used or dropped, so that they always pair (`TestWorkPairs`); Checkout's fetch does so from its goroutine through `mainthread`.
* Status comes from `git status --porcelain=v2 --branch -z`, with a merge and a rebase in progress (`Status.Merging`, `Status.Rebasing`, read from `rebase-merge` or `rebase-apply`). Unstaging always names paths, as a `git reset` without any would abandon a merge in progress. A status line's old path goes only on the side that renamed or copied the file (`sideOf`), as staging names it to git.
* Diffs are read up to `git.MaxDiff`, and nothing follows one that was cut off. All patches are asked for with `git.patchOptions`, which `diff.Parse` relies on. It drops each file's header (`diff`, `index`, `---`, `+++`, new and deleted file modes, similarity), keeping what is news (renames, mode changes, binary). In the history a bold file name replaces it (`diffView.titles`), which file jumping uses (`Parsed.Files`). It also makes sure the text has exactly one block per line, which `diffView.paint` relies on.
* Force push is `--force-with-lease` and always asks first. Discard All Changes is `git reset --hard HEAD` then `git clean -f -d` (`git.DiscardChanges`; `read-tree --empty` on a branch with no commits), keeping ignored files and commits and abandoning a merge.

### The window

* There are no tabs. The window shows the changes, or the history in their place while History is on (`views`, a stacked widget; `setHistoryShown`, `historyShown`). It always starts on the changes, whatever the repository holds (the user's choice). History is a checkable action, the toolbar's History and the Branch menu's Show History on Ctrl+H.
* Under the menus is a toolbar the size of apemail's (`makeToolBar`: 32 pixel icons, text under them, not movable): New Branch, Checkout, Delete, a separator, Rescan, Stage, a separator, Pull, Push, a separator, History. Each button is at least `toolButtonWidth` wide, so that short names are not thin, and has no tooltip: Qt gives a button its action's name as one whatever is set, so an event filter takes the tooltip event. Its buttons are the menus' own actions, with their icon text set to drop the ellipsis, so that they are enabled alike, but for Stage and Push, below.
* The toolbar's icons are drawn in code in apemail's style (`toolicons.go`: outlines on a grid of 24, 1.5 wide, round ends and joins, in the palette's text colour, disabled text where disabled), through `paintedIcon`, so that they follow the theme and Qt's SVG module is not needed.
* At the toolbar's right, beyond a space that takes up the rest, is a spinner (`spinner.go`): an arc turning, the size of a button's icon and in the icons' style, with Working under it, not a button. It shows once background work has gone on for `spinnerDelay`, so that quick work does not flicker, and goes when the count is back to 0.
* The status bar has a size grip and all its text at the left, at 50% opacity: the branch, then the last message, in labels of their own (`Window.say`). Don't use `ShowMessage`, which hides widgets beside it, or `AddPermanentWidget`, which goes on the right. A rebase in progress shows how far it has got there.
* Pane headings are bold (`heading`), and splitter handles are made through `OnCreateHandle` to paint only a row of grip dots, as `../reghead` does (`newPaneSplitter`).
* On starting, the repository opened is the one named on the command line, or else the one the current folder is in, or else none, waiting for File > Open Repository. The last one open is not remembered (the user's choice), and an older settings file's `repository` is ignored.
* Window size, maximized state and the three splitters are saved on close, and the rebase dialog's size whenever it closes. Pane sizes are applied after the window is shown. A window must not be deleted while a background command may still answer to it, so tests close windows without deleting them.

### The changes

* A changed file in a list is its path with a circle for what happened to it (`changeIcon`), in place of git's letter: `changeDotSize` (12, as reghead's dots) wide, an outline in its colour with a wash of it (`changeFill`) inside, as apemail's folders are. The colours are Solarized's (`changeColours`: blue modified, green added, red deleted, violet renamed, cyan copied, yellow type changed, orange in conflict, and its base0 grey untracked), the same in every theme (the user's choice), and kept on a selected row. The tooltip says it in words (`explain`). The first row of the history's files, the message, has a blank of the same size.
* In the file lists, ctrl and shift select several files. Dragging keeps one file selected, moving with the pointer, so each is shown in the diff in turn (`newFileList`). Only one list has a selection at a time.
* Both Stage buttons, the toolbar's and the one beside the message, are `stageButton`: it stages everything listed, and is Unstage, with a minus icon, when everything is staged (`allStaged`, kept as `unstaging`, not read back from a label). The Stage Changes menu item only stages; Stage Selected (Ctrl+S) and double-click stage only those chosen.
* Both Push buttons are `pushButton`, which forces while Force push is ticked; the Remote menu's Push, on Ctrl+P, never forces (the user's choice), and its Force Push… always does. The toolbar's Push is a split button (`MenuButtonPopup`) whose own menu, not the Remote menu, has a checkable Force push option (`forceToggle`), ticked along with the checkbox either way. While ticked, both Push buttons say Force Push. A successful push clears them all. Whether it is ticked is saved on closing (`ForcePush`) and ticked again on starting, once the window is joined up so that the option and buttons follow.
* Discard All Changes, in the Branch menu, always asks first, No being the default, counting files put back and new files (untracked or newly staged) deleted. It is enabled only with something to discard, and not during a rebase, which has Abort Rebase.

### The history

* It is like gitk: the commits of HEAD above, in `--topo-order`, as the graph needs every commit before its parents, and the chosen commit's files beside its whole diff below. They are read `pageSize` at a time, from the settings' `HistoryPageSize` (500 by default, 50 to 10000, taking effect from the next read), and more on scrolling to the end. Choosing a file scrolls to its name in the diff, relying on `git diff-tree --name-status` and `-p` listing files in the same order. Merges are diffed against their first parent.
* It reads only while on show: a rescan marks it stale (`markHistoryStale`), and it reloads on being shown. Its panes get their sizes when first shown (`historyView.sized`), and are saved only if they have been.
* While there is anything uncommitted, the first row is a stand-in commit (`git.UncommittedCommit`), whose parent is HEAD so that the graph joins up. It is in `historyView.list`, but read commits are counted without it (`committed`). `CommitDetail` shows it as everything since HEAD plus untracked files. It changes without its row changing, so it is read again on every rescan, keeping its scroll position.
* The graph is a column of the commit list, painted by an item delegate (`graphColumn`) from `graph.Layout`'s rows: only rows on show are painted, and it scrolls with the list. The layout is fed a page at a time. Each row's bottom edges must meet the next row's top edges, which `TestRowsJoinUp` checks on random histories. Changing lanes is an S curve leaving and arriving upright, so that lines run on smoothly between rows. The column is as wide as the lanes need, up to `graphMost`. A first parent already waited for in a lane to the right doesn't step over to it: its own lane waits too, and they meet at the parent, in the leftmost, so the graph doesn't wander right (`TestDoesNotWanderRight`).

### Interactive rebase

* It starts from a commit's context menu in the history, Rebase from here… (`commitMenu`), never the uncommitted row, and only on a clean work tree with no merge or rebase going. It covers that commit up to HEAD (`git.RebaseCommits`, which refuses a range holding a merge).
* The dialog (`rebaseDialog`) edits the plan: a row per commit, oldest first, each with its own combo of actions, in the order Pick, Fixup, Reword, Edit, Squash, Drop (`git.Actions`). A combo is an item widget; the wheel changes it only once it has focus, so that it scrolls the list; focusing or choosing in it makes its row the only one chosen (`chooseOnly`), so that the message below is that commit's. Below the list is only the chosen commit's message, full width, with no diff. The dialog has no keyboard shortcuts (the user's choice).
* The message is enabled only for a commit reworded, so that it is visibly greyed otherwise. Reword puts the cursor in it once the popup has closed (`editMessage`), and for a second after sends the focus back to it if it lands on the list or a combo (`reclaimFocus`), as on Wayland closing the popup puts the focus on the list later still.
* Rows are only dragged into order (`moved`). Qt only draws where a drag would go: the drop is handled in `dropAt` and ignored, so that Qt moves no rows, as it would delete the combos of rows it moved. Rows stay in place while the steps shown on them change. Qt delivers no drag events sent by hand, so tests call `dropAt`.
* Long messages are cut in the list (`subjectWidth`) and tooltips (`excerpt`), and shown whole below. The dialog's size is saved (`RebaseWidth`, `RebaseHeight`, 0 until it is first closed, when it opens at four fifths of the window). It is marked `finished` before it is deleted, so that a timer firing late does nothing.
* Git is given the plan through `GIT_SEQUENCE_EDITOR`, a `cp` of a temporary file. Reword is a pick then an `exec` that amends with the new message, a line at a time through `printf`, each quoted for the shell (`git.todo`), as git would otherwise want an editor. Squash keeps both messages as git joins them, as there is no editor.
* A stop is told of by `Window.rebaseStep` and handled from the Branch menu: Continue Rebase amends what is staged into a commit stopped at to edit, then continues; Skip Commit; Abort Rebase.

### Dialogs

* Form dialogs' fields grow to the dialog's width (`AllNonFixedFieldsGrow`), as some styles keep them narrow, and the first field of text has the focus when one is shown (`formDialog.firstText`), as it would otherwise go to OK, made first, unless `formDialog.focus` names another. A dialog answered is marked `finished`, so that what comes back from the background later touches nothing.
* Checkout (`newCheckoutDialog`) offers every local branch, in order of name ignoring case (`byName`), with the current one chosen and its list focused; OK is disabled while the current one is chosen, as there is nothing to do. Its Include remote branches checkbox, unticked to start with, adds the remote ones after them, in order too. Ticking it the first time fetches (`git fetch --all`, through `checkoutFetch`) in the background, with the list and OK disabled and Fetching… said; a failed fetch says why and lists those already known. What the fetch notes for the window, that it fetched and the spinner's count, it notes on the main thread (`TestCheckoutFetch`, under `make race`). The branch chosen is kept while it is still listed, or else the current one is chosen again. A cancelled dialog that fetched rescans. A remote branch is checked out as a new local branch tracking it.

### Look and settings

* Themes: light and dark are Fusion with our own palette; following the system restores the desktop's style and an empty palette. `applyTheme` is not called at startup when following the system.
* Diffs colour text, not backgrounds (the user's choice): added and removed lines take their colours as they are, through char formats (`diffView.paint`), and a hunk's `@@` range is in the Hunk headers colour, any text after it the normal colour. Nothing in a diff has a background. The default colours are Solarized's green, red and base0 grey (`DefaultAdded`, `DefaultRemoved`, `DefaultHunk`), as the change circles are.
* The settings dialog is tabbed, with one tab so far, User interface: Appearance, then History in the same form, so that the fields line up, with how many commits are read at a time, then a Colours table of Name and Colour, as `../prism` has it, with bold section rows and a filled button for each colour. Code font is one row under Appearance, as `../apemail` has it: any font of the system (no monospaced filter: the user wants any), the size inline, and a Default button. The code's font and size (`CodeFont`, `CodeFontSize`) apply to both diffs and the commit message; empty or 0 follows the system's fixed width font.
* The about box's title is as apemail's (`newAboutTitle`): the name in bold at `titleScale`, with the version to its right on the same baseline at `versionScale` and `versionOpacity`, as the release it is or follows (`release`: v1.2.4-0.… is shown as v1.2.3), or dev. The box is fitted to the height its contents need at its width (`fitHeight`), as apemail's is, as Qt's own height leaves a gap above the licences.
