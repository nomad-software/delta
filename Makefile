BINARY := delta

.PHONY: build run install test race vet bench screenshots appimage clean

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

# Measures what the program asks of Qt most. Compare two runs with benchstat
# (golang.org/x/perf/cmd/benchstat).
bench:
	go test -run '^$$' -bench . -benchmem -count 6 -timeout 30m ./internal/ui/

# Draws the window into docs/.
screenshots:
	DELTA_THEME=light DELTA_VIEW=changes DELTA_SCREENSHOT=$(CURDIR)/docs/changes.png go test -count=1 -run '^TestScreenshot$$' ./internal/ui/
	DELTA_THEME=light DELTA_VIEW=history DELTA_SCREENSHOT=$(CURDIR)/docs/history.png go test -count=1 -run '^TestScreenshot$$' ./internal/ui/

# Packs the program and the Qt it uses into one file, made in /tmp/delta-build
# and left in the project's folder. Releases are made by the GitHub workflow,
# on an old distribution so that they run on more systems.
appimage:
	packaging/appimage.sh

clean:
	rm -f $(BINARY) Delta-*.AppImage
	rm -rf /tmp/delta-build
