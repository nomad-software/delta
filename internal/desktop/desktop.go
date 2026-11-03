// Package desktop adds the program to a Linux desktop's applications, so that it
// can be started from the menu and kept in a dock. It knows nothing of the UI.
package desktop

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/nomad-software/delta/internal/atomicfile"
)

// Name names the desktop entry and the icon, without their extensions. Under
// Wayland, a window is matched to its entry by this name, which main.go gives Qt.
const Name = "delta"

// Install writes the desktop entry and the icon, given as PNG data, among the
// user's own. It is called at every start, so that the entry always starts the
// copy that ran last, wherever that is.
func Install(icon []byte) error {
	command, err := launcher()
	if err != nil {
		return err
	}
	data, err := dataFolder()
	if err != nil {
		return err
	}
	// Under X11, a window is matched by the name of the file that is running,
	// which in an AppImage is the program inside it.
	class := Name
	if self, err := os.Executable(); err == nil {
		class = filepath.Base(self)
	}
	return install(data, command, class, icon)
}

// install writes the entry and icon into the data folder.
func install(data, command, class string, icon []byte) error {
	image := filepath.Join(data, "icons", "hicolor", "256x256", "apps", Name+".png")
	entryPath := filepath.Join(data, "applications", Name+".desktop")
	return errors.Join(
		writeIfChanged(image, icon),
		writeIfChanged(entryPath, []byte(entry(command, class, image))),
	)
}

// launcher returns the file that starts the program. In an AppImage, whose
// runtime names it in $APPIMAGE, that is the image: the program inside is in a
// folder that only exists while it runs.
func launcher() (string, error) {
	if image := os.Getenv("APPIMAGE"); image != "" {
		return image, nil
	}
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved, nil
	}
	return path, nil
}

// dataFolder returns $XDG_DATA_HOME, or ~/.local/share if it is not set to an
// absolute path, as the XDG specification says.
func dataFolder() (string, error) {
	if data := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(data) {
		return data, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share"), nil
}

// entry returns the text of the desktop entry. The icon is given by its path
// rather than a theme's icon name, which a desktop with a stale cache of its
// icon folders may not find.
func entry(command, class, icon string) string {
	return "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=Delta\n" +
		"Comment=A graphical user interface for git\n" +
		"Exec=" + escapeValue(quoteArgument(command)) + "\n" +
		"TryExec=" + escapeValue(command) + "\n" +
		"Icon=" + escapeValue(icon) + "\n" +
		"Categories=Development;RevisionControl;\n" +
		"Terminal=false\n" +
		"StartupWMClass=" + escapeValue(class) + "\n"
}

// quoteArgument makes a path one argument of an Exec key, as the desktop entry
// specification says: % is doubled, as it starts a field code, and a path with
// anything reserved in it goes in double quotes, inside which ", `, $ and \ are
// escaped.
func quoteArgument(path string) string {
	path = strings.ReplaceAll(path, "%", "%%")
	if !strings.ContainsAny(path, " \t\n\"'\\><~|&;$*?#()`") {
		return path
	}
	return `"` + strings.NewReplacer(`"`, `\"`, "`", "\\`", `$`, `\$`, `\`, `\\`).Replace(path) + `"`
}

// escapeValue escapes what a value in a desktop entry can't hold as it is.
func escapeValue(value string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\t", `\t`, "\r", `\r`).Replace(value)
}

// writeIfChanged writes the data unless the file already holds it.
func writeIfChanged(path string, data []byte) error {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return atomicfile.Write(path, data, 0o644)
}
