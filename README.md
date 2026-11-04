# <img src="internal/appicon/appicon.png" alt="" width="40" height="40"> Delta

A graphical user interface for git, in the spirit of `git gui` and `gitk`.
Stage and commit your changes, and browse the history as a graph. It is
written in Go with Qt 6, through the [miqt](https://github.com/mappu/miqt)
bindings, and runs on Linux and macOS.

![The changes in Delta](docs/changes.png)

![The history in Delta](docs/history.png)

## Running

```sh
delta [folder]
```

It opens the repository holding the folder given. Without one, it opens the
repository the current folder is in, or else the one you last opened, or else
none, until you open one from **File > Open Repository…** or **Repository** in
the toolbar. It always starts on the changes, with the first unstaged file
chosen, if there is one, as it is whenever a repository is opened.

## The toolbar

| Button | Does |
| ------ | ---- |
| **Open** | Opens a repository, as **File > Open Repository…** (`Ctrl+O`) does. |
| **New Branch** | Makes a branch, and checks it out if you like. |
| **Checkout** | Switches to another branch. |
| **Delete** | Deletes a branch, after asking. |
| **Rescan** | Reads the state of the repository again. |
| **Stage** | Stages everything listed. When everything is staged it is **Unstage**, which unstages it all. |
| **Pull** | Pulls into the current branch. |
| **Push** | Pushes the current branch, setting its upstream on the first push. Its arrow opens a **Force push** option. |
| **History** | Shows the history in place of the changes until it is pressed again, as **Branch > Show History** (`Ctrl+H`) does. |

Under the toolbar, a progress bar the width of the window shows while anything
takes more than a moment: a command, reading the status, a diff or the history,
or a fetch.

## Repository and Branch

At the toolbar's right, **Repository** lists every repository you have opened,
including one Delta was started in, by the names of their folders in
alphabetical order. Hover over one to see its whole path. Choosing one opens it. The dropdowns grow to fit a long name, up to a limit,
and narrow when the window does.
The list is saved with the settings, with each repository in it only once.

**Branch** lists the open repository's local branches, with the current one
chosen. Choosing another checks it out.

## The changes

* **Unstaged Changes** and **Staged Changes**, in the sidebar, list the changed
  files, each with a circle in a Solarized colour for what happened to it. Hover
  over a file to have it said in words. The history's list of files has the same
  circles.

  | Circle | Change |
  | ------ | ------ |
  | Blue   | Modified |
  | Green  | Added |
  | Red    | Deleted |
  | Violet | Renamed |
  | Cyan   | Copied |
  | Yellow | Type changed |
  | Grey   | Untracked |
  | Orange | In conflict |

* Choose a file to see its **diff**, with added lines in green and removed lines
  in red, and without git's file headers. Drag over the list to see each file in
  turn. Double-click a file to stage or unstage it. Ctrl and shift select several.
* The **message pane** holds the commit message, with **Rescan**, **Stage
  Changes** (or **Unstage Changes**), **Commit** and **Push** beside it.
* **Force push**, beside them and in the toolbar Push's menu, makes the Push
  buttons force the next push, after asking you first. The two are ticked
  together, it is remembered between runs, and a successful push clears it.
  `Ctrl+P` always pushes without forcing, and **Remote > Force Push…** always
  forces.

Forcing uses `--force-with-lease`: git refuses if the remote has commits you have
not fetched, so you can't overwrite someone else's work without seeing it.

**Branch > Discard All Changes…** puts the branch back as if it had just been
checked out: every change not committed, staged or not, is undone, new files and
folders are deleted, and a merge in progress is abandoned. Commits, and files
git ignores, are kept. It asks first, saying how many files it will put back and
delete, as it can't be undone.

## The history

As in `gitk`, the commits of the current branch are listed at the top, newest
first, with a graph at the left drawing how they branch and merge, and any
branches and tags at them in brackets. They are read 500 at a time (a setting),
and more as you scroll to the end. If anything is not committed yet, an
**Uncommitted changes** row comes first, staged, unstaged and untracked together.

Below the commits, choose one to see the files it changed on the left, and its
message and diff on the right. Choose a file to go to its place in the diff. A
merge is shown as what it changed against its first parent.

A diff over 4 MiB, of the changes or in the history, is cut off with a note
saying so.

### Interactive rebase

Right-click a commit and choose **Rebase from here…** to rewrite it and the
commits after it. A dialog lists them oldest first, as git applies them, each
with a dropdown of what to do with it:

| Action | Does |
| ------ | ---- |
| Pick   | Keeps the commit. |
| Fixup  | Folds the commit into the one above, keeping only that message. |
| Reword | Keeps the commit with a new message: the message below the list becomes editable. |
| Edit   | Stops after the commit, so that you can change it. |
| Squash | Folds the commit into the one above, keeping both messages. |
| Drop   | Leaves the commit out, after asking you first. |

Drag commits into the order to apply them in, one or several at a time. Below
the list is the chosen commit's message. The list shows only the start of each
commit's first line, and its tooltip the start of its message, so that long
messages fit; the message below has all of it. The dialog opens at the size it
was last closed at.

Git needs a clean work tree, so commit or stash your changes first. A range
holding a merge can't be rebased this way. When the rebase stops, at a commit to
edit or one that doesn't apply cleanly, the status bar says so and how far it
has got. Resolve and stage in the changes, then choose **Continue Rebase** from
the Branch menu. Anything staged at a commit to edit goes into that commit.
**Skip Commit** leaves out the commit it stopped at, and **Abort Rebase** puts
the branch back as it was.

## Menus

| Menu   | Items | Shortcuts |
| ------ | ----- | --------- |
| File   | Open Repository, Settings, Quit | `Ctrl+O`, `Ctrl+,`, `Ctrl+Q` |
| Branch | New Branch, Checkout, Rename, Delete, Show History, Merge, Abort Merge, Continue Rebase, Skip Commit, Abort Rebase, Discard All Changes | New Branch `Ctrl+N`, Checkout `Ctrl+B`, Show History `Ctrl+H`, Merge `Ctrl+M` |
| Commit | Rescan, Stage Changes, Stage Selected, Unstage Selected, Unstage All, Commit | `F5`, `Ctrl+Shift+S`, `Ctrl+S`, `Ctrl+U`, `Ctrl+Shift+U`, `Ctrl+Return` |
| Remote | Fetch, Pull, Push, Force Push, Add Remote, Rename Remote, Change URL, Remove Remote | Fetch `Ctrl+Shift+F`, Pull `Ctrl+Shift+L`, Push `Ctrl+P`, Force Push `Ctrl+Shift+P` |
| Help   | About | `Ctrl+I` |

**Checkout** lists every local branch by name, with the current one chosen. Tick
**Include remote branches** to fetch and list the remote ones too: checking one
out makes a local branch that tracks it.

Deleting a branch asks first, and asks again if it has commits that have not been
merged.

## Settings

The **User interface** tab of **File > Settings…** sets:

- The theme: follow the system, light or dark.
- The font and size of the code (diffs and the commit message). Any installed
  font can be chosen.
- How many commits of the history are read at a time: 500 to start with, from
  50 to 10,000.
- The colours of added lines, removed lines and hunk headers (the grey
  `@@ -1,6 +1,7 @@`), in a table. They start as Solarized's green, red and grey.

The settings are kept in `delta/settings.json` under the system's configuration
folder (`~/.config` on Linux), along with whether **Force push** is ticked, the
size of the window and its panes, which are saved when it closes, and the size of
the interactive rebase dialog.

## Desktop entry

On Linux, Delta adds itself to the desktop's applications every time it
starts, so that it can be opened from the menu and kept in a dock. It writes
`~/.local/share/applications/delta.desktop` and its icon to
`~/.local/share/icons/hicolor/256x256/apps/delta.png` (or under
`$XDG_DATA_HOME`). The entry starts the copy that ran last, wherever it is, which
is the AppImage if it was run from one.

## Passwords

Delta never prompts for a password itself. Git and ssh run without a
terminal, so remote operations need a credential helper, an ssh agent or
`SSH_ASKPASS`; otherwise they fail with git's error rather than hanging.

## Requirements

Running it needs `git` 2.31 or later. Building it from source also needs:

- Go 1.27.3 or later
- A C++ compiler (GCC or Clang)
- `pkg-config`
- The Qt 6 development files of Qt Widgets

These commands install them:

```sh
# Arch, CachyOS, Manjaro
sudo pacman -S git go gcc pkgconf qt6-base

# Debian, Ubuntu
sudo apt install git golang-go build-essential pkg-config qt6-base-dev

# Fedora
sudo dnf install git golang gcc-c++ pkgconf-pkg-config qt6-qtbase-devel

# openSUSE
sudo zypper install git go gcc-c++ pkgconf-pkg-config qt6-base-devel

# macOS, with Homebrew and the Xcode Command Line Tools
brew install go pkgconf qtbase
```

If your distribution's Go is too old, get a newer one from
[go.dev/dl](https://go.dev/dl/). To check that Qt can be found, run
`pkg-config --modversion Qt6Widgets`, which should print a version starting
with 6.

## Building

The first build compiles all of the Qt bindings, which takes several minutes and
prints compiler warnings from the Qt headers that can be ignored. Later builds
take a few seconds.

```sh
make build        # builds ./delta
make run          # builds and runs it
make install      # installs it in $GOBIN or $(go env GOPATH)/bin, to run from anywhere
make test         # runs the tests
make race         # runs the tests with the race detector
make vet          # runs go vet
make screenshots  # redraws the pictures in docs/
make icons        # redraws the macOS icon in packaging/, after the icon changes
make appimage     # builds the AppImage
make macos        # builds the macOS app, on a Mac
make clean        # removes ./delta and build/
```

To change the icon, replace `internal/appicon/appicon.png` with another 256×256
PNG and run `make icons`.

## Releases

Pushing a tag starting with `v` runs the GitHub workflows, which build both
packages and attach them to a release of that name:

```sh
git tag v1.0.0 && git push origin v1.0.0
```

They can also be run by hand from the repository's Actions tab, which leaves the
package as an artifact of the run to download.

### AppImage for Linux

`Delta-*-x86_64.AppImage` is one file with Qt inside it. Make it
executable and run it. It works under X11 and Wayland, on systems with glibc 2.35
or later, such as Ubuntu 22.04 and Debian 12. `git` is not included.

`make appimage` runs the same script locally, but the result only runs on systems
at least as new as the one that built it. The releases are built on Ubuntu 22.04
for that reason.

### macOS app

`Delta-*-arm64.dmg` is for Macs with Apple Silicon. Open it and drag Delta
into Applications.

The app is not signed with an Apple Developer ID or notarised, so macOS won't
open it the first time. Do one of these once:

- Choose Open Anyway under System Settings > Privacy & Security.
- Run this in Terminal:

  ```sh
  xattr -dr com.apple.quarantine "/Applications/Delta.app"
  ```

## Licence

GNU General Public License version 3. See [LICENSE](LICENSE).
