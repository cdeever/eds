# eds-ota - a firmware build to one device, through the broker (ADR-0035).
#
# Plain recipes, one shell per line: macOS ships GNU make 3.81.
SHELL := /bin/bash

HOST        ?= tenant@services.eds.mobile.deevnet.net
BROKER_PORT ?= 21883
TEST_BROKER ?= tcp://127.0.0.1:$(BROKER_PORT)
LINUX_BIN   := bin/eds-ota-linux-amd64

.PHONY: help check fmt vet test test-integration build-linux send clean

help:
	@echo "eds-ota - a firmware build to one device, through the broker"
	@echo
	@echo "  check             gofmt check, go vet, unit tests"
	@echo "  test-integration  the tests, and a build through a dev broker on $(TEST_BROKER)"
	@echo "  send              DEVICE=lightstand/lp-stand-01 IMAGE=path/to/lp-stand.bin, from $(HOST)"
	@echo
	@echo "Usually run as 'make ota' in a firmware directory, which names both."

check: fmt vet test

fmt:
	@if [[ -n "$$(gofmt -l .)" ]]; then echo "not gofmt-clean:" >&2; gofmt -l . >&2; exit 1; fi

vet:
	go vet ./...

test:
	go test ./...

# The dev broker is nowplaying's (native mosquitto), started if it is not there.
test-integration:
	$(MAKE) -C ../../services/nowplaying broker
	OTA_TEST_BROKER=$(TEST_BROKER) go test -race -count=1 ./...

build-linux:
	@mkdir -p bin
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(LINUX_BIN) ./cmd/eds-ota

# The workload reaches the broker; the Mac does not need to. The sender is
# copied there each time and run with the broker settings the services use,
# which only root may read. The image goes over the SSH connection's stdin.
send: build-linux
	@if [[ -z "$(DEVICE)" || ! -s "$(IMAGE)" ]]; then echo "send needs DEVICE=kind/id and IMAGE=a built .bin" >&2; exit 2; fi
	ssh $(HOST) 'mkdir -p ~/eds-ota'
	scp -q $(LINUX_BIN) $(HOST):eds-ota/eds-ota
	ssh $(HOST) 'sudo bash -c "set -a; . /opt/eds/kit.env; . /opt/eds/ota-broker.env; set +a; MQTT_CA_FILE=/opt/eds/deevnet-root-ca.pem exec $$HOME/eds-ota/eds-ota -device $(DEVICE)"' < "$(IMAGE)"

clean:
	rm -rf bin
