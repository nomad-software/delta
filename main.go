// Delta is a graphical user interface for git.
package main

import (
	_ "embed"
	"fmt"
	"os"
	"runtime"

	qt "github.com/mappu/miqt/qt6"

	"github.com/nomad-software/delta/internal/desktop"
	"github.com/nomad-software/delta/internal/settings"
	"github.com/nomad-software/delta/internal/ui"
)

// iconPNG is the program's icon, for the desktop entry.
//
//go:embed packaging/delta.png
var iconPNG []byte

func main() {
	path, err := settings.Path()
	if err != nil {
		fmt.Fprintln(os.Stderr, "delta: the settings are kept in the current folder:", err)
		path = "settings.json"
	}

	qt.NewQApplication(os.Args)
	qt.QCoreApplication_SetApplicationName(desktop.Name)
	qt.QGuiApplication_SetApplicationDisplayName("Delta")
	qt.QGuiApplication_SetDesktopFileName(desktop.Name)

	if runtime.GOOS == "linux" {
		if err := desktop.Install(iconPNG); err != nil {
			fmt.Fprintln(os.Stderr, "delta: the desktop entry could not be written:", err)
		}
	}

	// What Qt has not used of the arguments can name a repository.
	var repository string
	if args := qt.QCoreApplication_Arguments(); len(args) > 1 {
		repository = args[1]
	}

	window := ui.NewWindow(path)
	window.Start(repository)

	qt.QApplication_Exec()
}
