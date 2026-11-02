# Merge Monkey

A graphical user interface for git, in the spirit of `git gui`, written in Go
with Qt 6 through the [miqt](https://github.com/mappu/miqt) bindings.

## Building

You need Go, a C++ compiler, the Qt 6 development libraries and `git`. It is
developed on Linux. The first build compiles the Qt bindings, which takes some
minutes.

```sh
make build   # builds ./merge-monkey
make run
make test
```

## Running

```sh
merge-monkey [folder]
```

It opens the repository holding the folder given. Without one, it opens the
repository the current folder is in, or else the one last open. You can open
another from **File > Open Repository…**.

## The Branch tab

* **Unstaged Changes** and **Staged Changes**, in the sidebar, list the changed
  files, each with a letter for what happened to it: `M` modified, `A` added,
  `D` deleted, `R` renamed, `C` copied, `T` type changed, `?` untracked, `U` in
  conflict.
* Choose a file to see its **diff**, with added lines in green and removed lines
  in red. Double-click a file to stage or unstage it.
* The **message pane** holds the commit message, with these beside it:
  * **Rescan** reads the state of the repository again.
  * **Stage Changes** stages everything in the unstaged list, untracked files
    included. When everything is staged it becomes **Unstage Changes**, which
    unstages everything.
  * **Commit** commits what is staged.
  * **Push** pushes the current branch, setting its upstream on the first push.
  * **Force push** makes the next push a forced one, after asking you first.

Forcing uses `--force-with-lease`: git refuses if the remote has commits you have
not fetched, so you can't overwrite someone else's work without seeing it.

## Menus

| Menu   | Items | Shortcuts |
| ------ | ----- | --------- |
| File   | Open Repository, Settings, Quit | `Ctrl+O`, `Ctrl+,`, `Ctrl+Q` |
| Branch | New Branch, Checkout, Rename, Delete, Merge, Abort Merge | `Ctrl+N`, `Ctrl+B`, Merge `Ctrl+M` |
| Commit | Rescan, Stage Changes, Stage Selected, Unstage Selected, Unstage All, Commit, Push, Force Push | `F5`, `Ctrl+Shift+T`, `Ctrl+T`, `Ctrl+U`, `Ctrl+Shift+U`, `Ctrl+Return`, `Ctrl+P`, `Ctrl+Shift+P` |
| Remote | Fetch, Pull, Push, Force Push, Add Remote, Rename Remote, Change URL, Remove Remote | Fetch `Ctrl+Shift+F`, Pull `Ctrl+Shift+L` |
| Help   | About | `Ctrl+I` |

## Settings

The **User interface** tab of **File > Settings…** sets the theme (follow the system, light or dark) and the
colours of added and removed lines. Lines are tinted with those colours, mixed
into the background so that the text stays readable in any theme.

The settings are kept in `merge-monkey/settings.json` under the system's
configuration folder (`~/.config` on Linux), along with the size of the window
and its panes, which are saved when it closes.

## Passwords

Merge Monkey never prompts for a password itself. Git and ssh run without a
terminal, so remote operations need a credential helper, an ssh agent or
`SSH_ASKPASS`; otherwise they fail with git's error rather than hanging.

## Licence

GNU General Public License version 3. See [LICENSE](LICENSE).
