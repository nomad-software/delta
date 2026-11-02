# Merge Monkey

A git GUI in Go with Qt 6 through the miqt bindings, modelled on `git gui`. `plan.txt` is the original brief. `../prism` is a reference for the look of the settings, about box and themes: use it as a reference, don't copy from it.

## Rules

* After every change run `make build`, and `make vet` and `make test` when code changed. `staticcheck ./...` and `deadcode -test ./...` must be clean.
* Keep this file and README.md up to date with every change.
* Don't commit; the user does that.
* miqt comes from `github.com/mappu/miqt`, at its newest tag. No `replace` directives or local dependencies.
* Delete only what `qt.New…` makes. A value miqt returns by value is freed by its finalizer.
* Qt is only touched on the main thread. Anything that runs git in the background comes back through `mainthread.Start`, and tests use `mainthread.Wait`.

## Layout

* `internal/git`: runs the git command line. No Qt. Tests use real temporary repositories.
* `internal/diff`: classifies the lines of a diff. No Qt.
* `internal/settings`: the JSON settings, with defaults for unusable values. No Qt.
* `internal/ui`: `window.go` (menus, actions, every operation), `branchtab.go` (the tab's widgets), `diffview.go`, `settingsdialog.go`, `about.go`, `theme.go`, `dialogs.go`, `icon.go`. Tests run with the offscreen platform.

## Behaviour to keep

* Git runs with `--literal-pathspecs`, `GIT_TERMINAL_PROMPT=0`, `GIT_EDITOR=true` and in a new session (`Setsid`), so it can't hang waiting on a terminal. `Setsid` is why this does not build for Windows.
* `Window.run` runs one changing command at a time in the background, with the actions disabled, then rescans. Rescans and diffs run in the background too, and an answer that isn't to the latest request is dropped.
* Status comes from `git status --porcelain=v2 --branch -z`. Unstaging always names paths, as a `git reset` without any would abandon a merge in progress.
* Diffs are read up to `git.MaxDiff`. `diff.Clean` makes sure the text has exactly one block per line, which `diffView.paint` relies on to line up `diff.Classify`'s result.
* Diff tints are block backgrounds, the only thing that reaches the full width; a QSyntaxHighlighter can't. A setting's colour is mixed into the palette's base, so one colour works in every theme. Changing theme or colour repaints.
* The settings dialog is tabbed, with one tab so far, User interface (Appearance and Diff colours sections).
* Themes: light and dark are Fusion with our own palette; following the system restores the desktop's style and an empty palette. `applyTheme` is not called at startup when following the system.
* Force push is `--force-with-lease`, always asks first, and the Force push checkbox clears after a successful push. The Push action honours the checkbox from menus and button alike.
* The stage button stages everything listed, and is Unstage Changes when everything is staged (`allStaged`). The Stage Changes menu item only stages; Stage Selected and double-click stage only those chosen. Only one list has a selection at a time.
* Pane headings are bold (`heading`), and splitter handles are made through `OnCreateHandle` to paint only a row of grip dots, as `../reghead` does (`newPaneSplitter`).
* The status bar has a size grip and all its text at the left: the branch, then the last message, in labels of their own (`Window.say`). Don't use `ShowMessage`, which hides widgets beside it, or `AddPermanentWidget`, which goes on the right.
* Window size, maximized state and the three splitters are saved on close. Pane sizes are applied after the window is shown.
* A window must not be deleted while a background command may still answer to it, so tests close windows without deleting them.
