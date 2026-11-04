BINARY := delta

.PHONY: build run install test race vet screenshots icons appimage macos clean

build:
	go build -ldflags "-s -w" -o $(BINARY) .

run: build
	./$(BINARY)

# Installs the program where Go puts programs ($GOBIN, or else $GOPATH/bin), to run
# from any folder if that is on the path.
install:
	go install -ldflags "-s -w" .

test:
	go test -timeout 5m ./...

# Runs the tests with the race detector, which finds what is touched from the
# background and the main thread at once. Its first run builds the Qt bindings
# again, which takes several minutes.
race:
	go test -race -timeout 20m ./...

vet:
	go vet ./...

# Draws the window into docs/.
screenshots:
	DELTA_THEME=light DELTA_VIEW=changes DELTA_SCREENSHOT=$(CURDIR)/docs/changes.png go test -count=1 -run '^TestScreenshot$$' ./internal/ui/
	DELTA_THEME=light DELTA_VIEW=history DELTA_SCREENSHOT=$(CURDIR)/docs/history.png go test -count=1 -run '^TestScreenshot$$' ./internal/ui/

# Draws the macOS app's icon, packaging/macos/delta.png, from the program's own,
# within Apple's margin. A test says when the icon has changed since it was run.
icons:
	DELTA_ICONS=$(CURDIR)/packaging go test -count=1 -run '^TestWriteIcons$$' ./internal/ui/

# Packs the program and the Qt it uses into one file. Releases are made by the
# GitHub workflow, on an old distribution so that they run on more systems.
appimage:
	packaging/appimage.sh

# Builds the macOS app in a disk image, on a Mac with Homebrew's Qt.
macos:
	packaging/macos.sh

clean:
	rm -f $(BINARY)
	rm -rf build
