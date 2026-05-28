VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BINARY  := grove
PKG     := github.com/m00nk0d3/grove/internal/version
LDFLAGS := -X $(PKG).Version=$(VERSION)

# Cross-compile targets require a POSIX shell (Git Bash / WSL on Windows).
.PHONY: build test lint clean install release snapshot \
        build-linux build-darwin build-windows

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/grove

build-linux:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINARY)-linux-amd64 ./cmd/grove
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINARY)-linux-arm64 ./cmd/grove

build-darwin:
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINARY)-darwin-amd64 ./cmd/grove
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINARY)-darwin-arm64 ./cmd/grove

build-windows:
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINARY).exe ./cmd/grove

test:
	go test ./... -v -count=1

lint:
	golangci-lint run

clean:
	rm -f $(BINARY) $(BINARY)-linux-* $(BINARY)-darwin-* $(BINARY).exe
	go clean -testcache

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/grove

release:
	goreleaser release --clean

snapshot:
	goreleaser release --snapshot --clean
