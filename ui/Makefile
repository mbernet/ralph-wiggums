# ralph-ui: the terminal UI of ralph.sh. ralph.sh finds it at ui/bin/ralph-ui.
#
#   make -C ui          build it
#   make -C ui test     its tests (they run ralph.sh dry runs)
#   make -C ui install  copy it to $(PREFIX)/bin

PREFIX ?= /usr/local

bin/ralph-ui: $(shell find . -name '*.go') go.mod go.sum
	go build -trimpath -o $@ ./cmd/ralph-ui

.PHONY: test install clean
# -count=1: the view tests run ralph.sh, which Go's test cache does not track.
test:
	go vet ./...
	go test -count=1 ./...

install: bin/ralph-ui
	install -m 0755 bin/ralph-ui $(PREFIX)/bin/ralph-ui

clean:
	rm -rf bin
