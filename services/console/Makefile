# console - one page that shows what EdS is doing.
#
# Plain recipes, one shell per line: macOS ships GNU make 3.81, which has no
# .ONESHELL, and this module is developed on a Mac.
SHELL := /bin/bash

BIN_DIR     ?= $(CURDIR)/bin
BROKER_PORT ?= 21883
PREFIX      ?= edsdev
ADDR        ?= 127.0.0.1:8089
VERSION     ?= $(shell git describe --always --dirty 2>/dev/null || echo dev)

.PHONY: help build test test-js check fmt vet run feed clean

help:
	@echo "console - available targets"
	@echo
	@echo "  build    build consoled into $(BIN_DIR)"
	@echo "  test     Go tests, and the effects' tests if node is installed"
	@echo "  check    fmt check, vet, and the tests"
	@echo "  run      run against the development broker, on http://$(ADDR)"
	@echo "  feed     put a sample of the bus on the development broker"
	@echo "           (WHAT=itunes|spotify EFFECT=breathe|sweep|solid COVER=file.jpg)"
	@echo
	@echo "The development broker is nowplaying's: make -C ../nowplaying broker"

build:
	mkdir -p "$(BIN_DIR)"
	go build -ldflags "-X main.version=$(VERSION)" -o "$(BIN_DIR)/consoled" ./cmd/consoled

test: test-js
	go test ./...

# The effects are a port of the firmware's, and these are the firmware's own
# checks asked of the port. Skipped, and said so, where there is no node:
# nothing else here needs one.
test-js:
	@if command -v node >/dev/null; then node --test internal/web/; \
	else echo "node is not installed: the effects' and the clock's tests were NOT run"; fi

check: fmt vet test

fmt:
	@if [[ -n "$$(gofmt -l .)" ]]; then echo "not gofmt-clean:" >&2; gofmt -l . >&2; exit 1; fi

vet:
	go vet ./...

# A prefix of its own, so nothing here can be mistaken for the tenant's topics.
run: build
	CONSOLE_MQTT_URL=tcp://127.0.0.1:$(BROKER_PORT) CONSOLE_TOPIC_PREFIX=$(PREFIX) CONSOLE_ADDR=$(ADDR) "$(BIN_DIR)/consoled"

feed:
	BROKER_PORT=$(BROKER_PORT) PREFIX=$(PREFIX) EFFECT=$(or $(EFFECT),breathe) dev/feed.sh $(or $(WHAT),itunes) $(COVER)

clean:
	rm -rf "$(BIN_DIR)"
