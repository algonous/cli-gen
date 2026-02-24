BIN_NAME ?= cli-gen
BIN_PATH := $(BIN)/$(BIN_NAME)
GOCACHE ?= /tmp/go-build-cache

.PHONY: all test build clean

all: test build

test:
	GOCACHE=$(GOCACHE) go test ./...

build:
	mkdir -p $(BIN)
	GOCACHE=$(GOCACHE) go build -o $(BIN_PATH) ./cmd/cli-gen

clean:
	rm -f $(BIN_PATH)
