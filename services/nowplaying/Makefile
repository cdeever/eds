# nowplaying - what is playing, on the bus.
#
# Plain recipes, one shell per line: macOS ships GNU make 3.81, which has no
# .ONESHELL, and this module is developed on a Mac. /bin/bash is on macOS and
# Linux alike.
SHELL := /bin/bash

BIN_DIR     ?= $(CURDIR)/bin
BROKER_PORT ?= 21883
TEST_BROKER ?= tcp://127.0.0.1:$(BROKER_PORT)
VERSION     ?= $(shell git describe --always --dirty 2>/dev/null || echo dev)
PREFIX      ?= edsdev

.PHONY: help build build-arm64 test test-integration check fmt vet run broker broker-stop broker-watch clean

help:
	@echo "nowplaying - available targets"
	@echo
	@echo "  build             build nowplayd and npagent into $(BIN_DIR)"
	@echo "  build-arm64       cross-compile npagent for a Raspberry Pi (linux/arm64)"
	@echo "  test              unit tests (no broker needed)"
	@echo "  broker            start a development MQTT broker (mosquitto) on $(TEST_BROKER)"
	@echo "  test-integration  all tests with -race, including those needing the broker"
	@echo "  check             fmt check, vet, and the unit tests"
	@echo "  run               run nowplayd against the development broker, under $(PREFIX)/"
	@echo "  broker-watch      tail every $(PREFIX)/ topic; covers shown by size"
	@echo "  broker-stop       stop the development broker"

build:
	mkdir -p "$(BIN_DIR)"
	go build -ldflags "-X main.version=$(VERSION)" -o "$(BIN_DIR)/nowplayd" ./cmd/nowplayd
	go build -o "$(BIN_DIR)/npagent" ./cmd/npagent

# No cgo anywhere in the module, so the Mac builds the Pi's binary directly.
build-arm64:
	mkdir -p "$(BIN_DIR)"
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o "$(BIN_DIR)/npagent-linux-arm64" ./cmd/npagent

test:
	go test ./...

# Retention, the last will and subscription timing are the broker's behaviour,
# not the client's, so the tests that matter most need a real one.
test-integration: broker
	NP_TEST_BROKER=$(TEST_BROKER) go test -race -count=1 ./...

check: fmt vet test

fmt:
	@if [[ -n "$$(gofmt -l .)" ]]; then echo "not gofmt-clean:" >&2; gofmt -l . >&2; exit 1; fi

vet:
	go vet ./...

# The development broker is mosquitto itself rather than a container: it is a
# brew or dnf install away, and it starts in the time podman takes to notice
# it was asked.
broker:
	@command -v mosquitto >/dev/null || { echo "mosquitto is not installed (brew install mosquitto / dnf install mosquitto)" >&2; exit 2; }
	@if nc -z 127.0.0.1 $(BROKER_PORT) 2>/dev/null; then echo "development broker already on $(TEST_BROKER)"; \
	else mosquitto -c dev/mosquitto.conf -p $(BROKER_PORT) -d && echo "development broker on $(TEST_BROKER)"; fi

broker-stop:
	@pkill -f "mosquitto -c dev/mosquitto.conf" && echo "stopped" || echo "not running"

# A prefix of its own, so nothing here can be mistaken for the tenant's topics.
run: build broker
	NP_MQTT_URL=$(TEST_BROKER) NP_TOPIC_PREFIX=$(PREFIX) "$(BIN_DIR)/nowplayd"

# Covers are image bytes, so they are shown by size and never printed. The
# exclusions keep the pictures out of the terminal; `broker-watch-art` in a
# second terminal shows them arriving. Piping the bytes through a filter
# instead does not survive the first JPEG.
broker-watch:
	mosquitto_sub -h 127.0.0.1 -p $(BROKER_PORT) -t '$(PREFIX)/#' -T '$(PREFIX)/nowplaying/current/art' -T '$(PREFIX)/nowplaying/source/+/art' -v

.PHONY: broker-watch-art
broker-watch-art:
	mosquitto_sub -h 127.0.0.1 -p $(BROKER_PORT) -t '$(PREFIX)/nowplaying/current/art' -t '$(PREFIX)/nowplaying/source/+/art' -F '%t  (%l bytes)'

clean:
	rm -rf "$(BIN_DIR)"
