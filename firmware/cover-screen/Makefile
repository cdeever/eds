# Cover screen - firmware.
#
# The hardware-independent half (what a touch means, what is drawn over the
# cover) is tested with `make test` and needs nothing but a C compiler.
# Everything else needs ESP-IDF and, to be certain, the board.
#
# Every recipe is one line: this is run on macOS, whose make is 3.81 and has
# no .ONESHELL.
PORT ?= /dev/cu.usbserial-2120

# The ESP-IDF Installation Manager exposes idf.py as a shell function rather
# than adding it to PATH, so a recipe's subshell cannot see it. Resolve the
# script out of IDF_PATH when it is there.
IDF_PY := $(if $(wildcard $(IDF_PATH)/tools/idf.py),python3 $(IDF_PATH)/tools/idf.py,idf.py)

.PHONY: help test pictures clean build flash monitor menuconfig provision backup ota

help:
	@echo "cover-screen - available targets"
	@echo
	@echo "  test        host tests for touches and drawing (no ESP-IDF needed)"
	@echo "  pictures    draw the screens to test/host/pictures, to look at"
	@echo "  menuconfig  configure the panel, touch and network"
	@echo "  build       idf.py build"
	@echo "  backup      save what is on the board now to backup/ (PORT=$(PORT))"
	@echo "  flash       idf.py flash        (PORT=$(PORT))"
	@echo "  monitor     idf.py monitor      (PORT=$(PORT))"
	@echo "  ota         send build/cover-screen.bin to the screen it was built for, through the broker"
	@echo "  provision   write what the EdS tenant issued this screen into flash (PORT=$(PORT))"
	@echo "  clean       remove host test artifacts"
	@echo
	@echo "ESP-IDF targets need . ~/.espressif/tools/activate_idf_v6.1.sh first."

test:
	$(MAKE) -C test/host test

pictures:
	$(MAKE) -C test/host pictures

menuconfig:
	$(IDF_PY) menuconfig

build:
	$(IDF_PY) build

flash:
	$(IDF_PY) -p "$(PORT)" flash

monitor:
	$(IDF_PY) -p "$(PORT)" monitor

# The build to the screen in place, through the broker (ADR-0035): no cable.
# It goes to the screen this build is for, read from sdkconfig, because the id
# is compiled in: a build for another would log in here and listen on the
# other's topics, where no update could reach it.
ota:
	@id=$$(sed -n 's/^CONFIG_CS_SCREEN_ID="\(.*\)"$$/\1/p' sdkconfig); \
	if [ -z "$$id" ] || [ ! -s build/cover-screen.bin ]; then echo "no build here: make build first" >&2; exit 2; fi; \
	$(MAKE) -C ../../tools/ota send DEVICE=screen/$$id IMAGE="$(CURDIR)/build/cover-screen.bin"

# The whole flash, before the first flash replaces what the board came with.
# Writing it back at 0x0 restores the board exactly.
backup:
	mkdir -p backup && python3 -m esptool -p "$(PORT)" read-flash 0 ALL backup/factory-$$(date +%Y%m%d-%H%M%S).bin

# Everything the screen is flashed with was issued by the EdS tenant and is in
# its Terraform outputs. The WiFi key, the broker account and the CA go into
# the creds partition; nothing is typed, and nothing reaches the build.
provision:
	python3 tools/provision_tenant.py -p "$(PORT)" $(if $(SCREEN),--screen "$(SCREEN)",)

clean:
	$(MAKE) -C test/host clean
