# LP jacket stand - firmware.
#
# The hardware-independent half (scene validation, effects, gamma) is tested
# with `make test` and needs nothing but a C compiler. Everything else needs
# ESP-IDF on the PATH and, to be certain, a device.
.ONESHELL:
SHELL := /usr/bin/bash
.SHELLFLAGS := -euo pipefail -c

PORT ?= /dev/ttyUSB0

.PHONY: help test clean build flash monitor menuconfig

help:
	@echo "lp-stand - available targets"
	@echo
	@echo "  test        host tests for scene/effects/gamma (no ESP-IDF needed)"
	@echo "  menuconfig  configure WiFi, broker and strip"
	@echo "  build       idf.py build"
	@echo "  flash       idf.py flash        (PORT=$(PORT))"
	@echo "  monitor     idf.py monitor      (PORT=$(PORT))"
	@echo "  clean       remove host test artifacts"
	@echo
	@echo "ESP-IDF targets need . \$$IDF_PATH/export.sh first."

test:
	$(MAKE) -C test/host test

menuconfig:
	idf.py menuconfig

build:
	idf.py build

flash:
	idf.py -p "$(PORT)" flash

monitor:
	idf.py -p "$(PORT)" monitor

clean:
	$(MAKE) -C test/host clean
