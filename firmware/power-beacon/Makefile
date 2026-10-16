# Power beacon - firmware.
#
# The event lines are tested with `make test` and need nothing but a C
# compiler. Everything else needs ESP-IDF and, to be certain, the board.
#
# Every recipe is one line: this is run on macOS, whose make is 3.81 and has
# no .ONESHELL.
#
# The port is the one the cover screen takes when it is the board plugged in,
# so `flash` and `provision` ask the chip what it is first: the beacon is an
# ESP32-S2 and the screen is not.
PORT ?= /dev/cu.usbserial-2120
CHIP ?= esp32s2

# The ESP-IDF Installation Manager exposes idf.py as a shell function rather
# than adding it to PATH, so a recipe's subshell cannot see it. Resolve the
# script out of IDF_PATH when it is there.
IDF_PY := $(if $(wildcard $(IDF_PATH)/tools/idf.py),python3 $(IDF_PATH)/tools/idf.py,idf.py)

.PHONY: help test clean build chip flash monitor menuconfig provision ota

help:
	@echo "power-beacon - available targets"
	@echo
	@echo "  test        host tests for the event lines (no ESP-IDF needed)"
	@echo "  menuconfig  configure the beacon's name and network"
	@echo "  build       idf.py build"
	@echo "  chip        say what is on PORT, and stop if it is not a $(CHIP)"
	@echo "  flash       idf.py flash        (PORT=$(PORT))"
	@echo "  monitor     idf.py monitor      (PORT=$(PORT))"
	@echo "  ota         send build/power-beacon.bin to the beacon it was built for, through the broker"
	@echo "  provision   write what the EdS tenant issued this beacon into flash (PORT=$(PORT))"
	@echo "  clean       remove host test artifacts"
	@echo
	@echo "ESP-IDF targets need . ~/.espressif/tools/activate_idf_v6.1.sh first."

test:
	$(MAKE) -C test/host test

menuconfig:
	$(IDF_PY) menuconfig

build:
	$(IDF_PY) build

# esptool, told the chip, refuses any other.
chip:
	python3 -m esptool --chip "$(CHIP)" -p "$(PORT)" chip-id

flash: chip
	$(IDF_PY) -p "$(PORT)" flash

monitor:
	$(IDF_PY) -p "$(PORT)" monitor

# The build to the beacon in place, through the broker (ADR-0035): no cable.
# It goes to the beacon this build is for, read from sdkconfig, because the id
# is compiled in: a build for another would log in here and listen on the
# other's topics, where no update could reach it.
ota:
	@id=$$(sed -n 's/^CONFIG_PB_BEACON_ID="\(.*\)"$$/\1/p' sdkconfig); \
	if [ -z "$$id" ] || [ ! -s build/power-beacon.bin ]; then echo "no build here: make build first" >&2; exit 2; fi; \
	$(MAKE) -C ../../tools/ota send DEVICE=beacon/$$id IMAGE="$(CURDIR)/build/power-beacon.bin"

# Everything the beacon is flashed with was issued by the EdS tenant and is in
# its Terraform outputs. The WiFi key, the broker account and the CA go into
# the creds partition; nothing is typed, and nothing reaches the build.
provision: chip
	python3 tools/provision_tenant.py -p "$(PORT)" $(if $(BEACON),--beacon "$(BEACON)",)

clean:
	$(MAKE) -C test/host clean
