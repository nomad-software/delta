BINARY := delta

.PHONY: build run install test vet screenshots icons appimage macos clean

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

vet:
	go vet ./...

# Draws the window into docs/.
screenshots:
	DELTA_THEME=dark DELTA_VIEW=branch DELTA_SCREENSHOT=$(CURDIR)/docs/branch.png go test -count=1 -run '^TestScreenshot$$' ./internal/ui/
	DELTA_THEME=dark DELTA_VIEW=history DELTA_SCREENSHOT=$(CURDIR)/docs/history.png go test -count=1 -run '^TestScreenshot$$' ./internal/ui/

# Draws the icons kept for packaging. A test says when this has not been run
# since the code that draws them changed.
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
