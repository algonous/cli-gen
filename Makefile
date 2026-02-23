BIN_DIR ?= ./bin
BIN_NAME ?= cli-gen
BIN_PATH := $(BIN_DIR)/$(BIN_NAME)
GOCACHE ?= /tmp/go-build-cache

.PHONY: all test build clean

all: test build

test:
	GOCACHE=$(GOCACHE) go test ./...

build:
	mkdir -p $(BIN_DIR)
	GOCACHE=$(GOCACHE) go build -o $(BIN_PATH) ./cmd/cli-gen

clean:
	rm -f $(BIN_PATH)
