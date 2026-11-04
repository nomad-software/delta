// Delta is a graphical user interface for git.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"

	"github.com/nomad-software/delta/internal/appicon"
	"github.com/nomad-software/delta/internal/desktop"
	"github.com/nomad-software/delta/internal/instance"
	"github.com/nomad-software/delta/internal/settings"
	"github.com/nomad-software/delta/internal/ui"
)

func main() {
	// The desktop's leave to take the focus is read before Qt, which may take
	// it out of the environment, starts.
	token := os.Getenv("XDG_ACTIVATION_TOKEN")

	path, err := settings.Path()
	if err != nil {
		fmt.Fprintln(os.Stderr, "delta: the settings are kept in the current folder:", err)
		path = "settings.json"
	}

	qt.NewQApplication(os.Args)
	qt.QCoreApplication_SetApplicationName(desktop.Name)
	qt.QGuiApplication_SetApplicationDisplayName("Delta")
	qt.QGuiApplication_SetDesktopFileName(desktop.Name)

	// What Qt has not used of the arguments can name a repository.
	var repository string
	if args := qt.QCoreApplication_Arguments(); len(args) > 1 {
		repository = args[1]
	}

	// One copy runs at a time: one started while another runs has that one's
	// window shown, with the repository it would have opened, and leaves. It
	// does so once Qt has taken its own arguments, and before anything else.
	claim, err := claimInstance(token, repository)
	switch {
	case errors.Is(err, instance.ErrRunning):
		return
	case errors.Is(err, instance.ErrNoAnswer):
		fmt.Fprintln(os.Stderr, "delta:", err)
		os.Exit(1)
	case err != nil:
		// The program runs, as it would without the check.
		fmt.Fprintln(os.Stderr, "delta: another copy could be started beside this one:", err)
	default:
		defer claim.Close()
	}

	if runtime.GOOS == "linux" {
		if err := desktop.Install(appicon.PNG); err != nil {
			fmt.Fprintln(os.Stderr, "delta: the desktop entry could not be written:", err)
		}
	}

	window := ui.NewWindow(path)
	window.Start(repository)
	if claim != nil {
		claim.Serve(func(r instance.Request) {
			mainthread.Start(func() { window.Activate(r) })
		})
	}

	qt.QApplication_Exec()
}

// claimInstance makes this the copy that runs, or else hands the one that runs
// the token and the folder to open: the repository named, made absolute, or
// else the folder this one was started in.
func claimInstance(token, repository string) (*instance.Instance, error) {
	dir, err := instance.Dir()
	if err != nil {
		return nil, err
	}
	request := instance.Request{Token: token, Named: repository != ""}
	if request.Named {
		request.Folder, err = filepath.Abs(repository)
	} else {
		request.Folder, err = os.Getwd()
	}
	if err != nil {
		request.Folder = ""
	}
	return instance.Claim(dir, request)
}
