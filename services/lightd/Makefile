# lightd - cover image in, MQTT scene out.
.ONESHELL:
SHELL := /usr/bin/bash
.SHELLFLAGS := -euo pipefail -c

BIN        ?= $(CURDIR)/bin/lightd
BROKER_PORT ?= 21883
TEST_BROKER ?= tcp://127.0.0.1:$(BROKER_PORT)

.PHONY: help build test test-integration check fmt vet run broker broker-stop broker-watch clean

help:
	@echo "lightd - available targets"
	@echo
	@echo "  build             build $(BIN)"
	@echo "  test              unit tests (no broker needed)"
	@echo "  broker            start a development MQTT broker in podman"
	@echo "  test-integration  all tests, including those needing the broker"
	@echo "  check             fmt check, vet, and the full test suite"
	@echo "  run               run against a local palette service and broker"
	@echo "  broker-watch      tail every eds/# topic on the dev broker"
	@echo "  broker-stop       remove the development broker"

build:
	mkdir -p "$(dir $(BIN))"
	go build -o "$(BIN)" ./cmd/lightd

test:
	go test ./...

# The retained-scene behaviour is a broker behaviour, not a client one, so the
# tests that matter most need a real one.
test-integration: broker
	LIGHTD_TEST_BROKER=$(TEST_BROKER) go test -race ./...

check: fmt vet test

fmt:
	@if [[ -n "$$(gofmt -l .)" ]]; then
		echo "not gofmt-clean:" >&2
		gofmt -l . >&2
		exit 1
	fi

vet:
	go vet ./...

broker:
	@if [[ -z "$$(podman ps -q --filter name=eds-mosquitto)" ]]; then
		podman rm -f eds-mosquitto >/dev/null 2>&1 || true
		podman run -d --name eds-mosquitto -p $(BROKER_PORT):1883 \
			-v "$(CURDIR)/dev/mosquitto.conf:/mosquitto/config/mosquitto.conf:Z" \
			docker.io/library/eclipse-mosquitto:2 >/dev/null
		echo "development broker on $(TEST_BROKER)"
	else
		echo "development broker already running on $(TEST_BROKER)"
	fi

broker-watch:
	podman exec eds-mosquitto mosquitto_sub -h localhost -t 'eds/#' -v

broker-stop:
	podman rm -f eds-mosquitto >/dev/null 2>&1 || true

run: build broker
	LIGHTD_MQTT_URL=$(TEST_BROKER) "$(BIN)"

clean:
	rm -rf "$(dir $(BIN))"
