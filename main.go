// Merge Monkey is a graphical user interface for git.
package main

import (
	"fmt"
	"os"

	qt "github.com/mappu/miqt/qt6"

	"github.com/nomad-software/merge-monkey/internal/settings"
	"github.com/nomad-software/merge-monkey/internal/ui"
)

func main() {
	path, err := settings.Path()
	if err != nil {
		fmt.Fprintln(os.Stderr, "merge-monkey: the settings are kept in the current folder:", err)
		path = "settings.json"
	}

	qt.NewQApplication(os.Args)
	qt.QCoreApplication_SetApplicationName("merge-monkey")
	qt.QGuiApplication_SetApplicationDisplayName("Merge Monkey")

	// What Qt has not used of the arguments can name a repository.
	var repository string
	if args := qt.QCoreApplication_Arguments(); len(args) > 1 {
		repository = args[1]
	}

	window := ui.NewWindow(path)
	window.Start(repository)

	qt.QApplication_Exec()
}
