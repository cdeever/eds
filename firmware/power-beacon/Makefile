# Power beacon - firmware.
#
# The event lines are tested with `make test` and need nothing but a C
# compiler. Everything else needs ESP-IDF and, to be certain, the board.
#
# Every recipe is one line: this is run on macOS, whose make is 3.81 and has
# no .ONESHELL.
#
# The beacon is a plain ESP32, as the stand and the cover screen are, and any
# of them can be the board on the port. So `flash` and `provision` ask the
# chip for its MAC first, and stop unless it is this beacon's. A MAC is not a
# secret; it is the one thing that tells three ESP32s apart.
PORT ?= /dev/cu.usbserial-0001
CHIP ?= esp32
MAC  ?= 70:4b:ca:27:06:14

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
	@echo "  chip        say what is on PORT, and stop unless it is a $(CHIP) with MAC $(MAC)"
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

# esptool, told the chip, refuses any other; the MAC is then checked by hand.
chip:
	@out=$$(python3 -m esptool --chip "$(CHIP)" -p "$(PORT)" read-mac 2>&1) || { echo "$$out" >&2; exit 2; }; mac=$$(echo "$$out" | sed -n 's/^MAC: *\([0-9a-f:]*\).*/\1/p' | head -1); if [ "$$mac" != "$(MAC)" ]; then echo "the $(CHIP) on $(PORT) is $${mac:-unknown}, not this beacon ($(MAC)); give MAC= to flash another" >&2; exit 2; fi; echo "$(PORT): $(CHIP), $$mac, this beacon"

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
