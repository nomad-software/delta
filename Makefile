BINARY := merge-monkey

.PHONY: build run test vet clean

build:
	go build -ldflags "-s -w" -o $(BINARY) .

run: build
	./$(BINARY)

test:
	go test -timeout 5m ./...

vet:
	go vet ./...

clean:
	rm -f $(BINARY)
