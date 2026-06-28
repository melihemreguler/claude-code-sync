VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

.PHONY: build install test vet clean

build:
	go build -ldflags "$(LDFLAGS)" -o ccsync .

install:
	go install -ldflags "$(LDFLAGS)" .

vet:
	go vet ./...

test:
	go test ./...

clean:
	rm -f ccsync
