# Ears (reSpeaker Lite) - firmware.
#
# The hardware-independent half (what is done to the sound, the event lines)
# is tested with `make test` and needs nothing but a C compiler. Everything
# else needs ESP-IDF and the board.
#
# Every recipe is one line: this is run on macOS, whose make is 3.81.
PORT ?= /dev/cu.usbmodem2101

# The XIAO's MAC: flash and provision check it first, so a cable plugged
# into another of EdS's boards is not written over.
MAC ?= 68:ee:8f:50:45:b0

IDF_PY := $(if $(wildcard $(IDF_PATH)/tools/idf.py),python3 $(IDF_PATH)/tools/idf.py,idf.py)

.PHONY: help test clean build flash monitor menuconfig provision backup check-board

help:
	@echo "ears - available targets"
	@echo
	@echo "  test        host tests for the sound and the events (no ESP-IDF needed)"
	@echo "  menuconfig  configure the pins, the channel and the network"
	@echo "  build       idf.py build"
	@echo "  backup      save what is on the board now to backup/ (PORT=$(PORT))"
	@echo "  flash       idf.py flash, after checking the board (PORT=$(PORT))"
	@echo "  monitor     idf.py monitor  (PORT=$(PORT))"
	@echo "  provision   write what the EdS tenant issued this device into flash"
	@echo "  clean       remove host test artifacts"
	@echo
	@echo "ESP-IDF targets need . ~/.espressif/tools/activate_idf_v6.1.sh first."

test:
	$(MAKE) -C test/host test

menuconfig:
	$(IDF_PY) menuconfig

build:
	$(IDF_PY) build

# The board on the port is the one this is for, or nothing is written.
check-board:
	@mac=$$(python3 -m esptool -p "$(PORT)" read-mac 2>/dev/null | sed -n 's/^MAC: *//p' | head -1); \
	if [ "$$mac" != "$(MAC)" ]; then echo "the board on $(PORT) is $${mac:-not answering}, not $(MAC)" >&2; exit 2; fi

flash: check-board
	$(IDF_PY) -p "$(PORT)" flash

monitor:
	$(IDF_PY) -p "$(PORT)" monitor

# The whole flash, before the first flash replaces what the board came with.
backup:
	mkdir -p backup && python3 -m esptool -p "$(PORT)" read-flash 0 ALL backup/factory-$$(date +%Y%m%d-%H%M%S).bin

provision: check-board
	python3 tools/provision_tenant.py -p "$(PORT)"

clean:
	$(MAKE) -C test/host clean
