# The backend Pi: the whole of EdS on one Raspberry Pi, with nothing of
# Deevnet IoTaaS behind it (ADR-0033).
#
# The image is EdS's own (images/backend), built on the take-home image
# Deevnet IoTaaS publishes. This directory is the rest: getting the image
# built, writing a flashed card's settings, and looking after the Pi.
#
# The Pi is its owner's, not the site's. It is reached directly, as the user
# made on its first boot, and not through the Builder: the take-home image
# has no automation user. The Builder is only where the image is built.
#
# Plain recipes, one shell per line: macOS ships GNU make 3.81.
SHELL := /bin/bash

BUILDER     ?= cdeever@builder
BUILD_DIR   ?= /srv/eds-backend-build
PI_HOSTNAME ?= eds-backend
PI          ?= $(PI_HOSTNAME).local
PI_USER     ?= eds
IMAGE       := eds-pi-backend.img.xz

# The tenant's name and index on Deevnet IoTaaS. Kept, so that every topic,
# every account's username and the device log partition are what they are
# there, and no firmware is rebuilt.
TENANT := eds
INDEX  := 2

REPO    := $(CURDIR)/../..
NP      := $(REPO)/services/nowplaying
LIGHTD  := $(REPO)/services/lightd
CONSOLE := $(REPO)/services/console
PALETTE := $(REPO)/services/palette
STAGE   := $(CURDIR)/stage
VERSION := $(shell git -C $(REPO) describe --always --dirty 2>/dev/null || echo dev)
SSH     := $(PI_USER)@$(PI)

# Where the flashed card's boot partition is mounted. macOS names it bootfs.
BOOT ?= /Volumes/bootfs
# The owner's Discogs settings, kept by hand and never committed.
DISCOGS ?= $(REPO)/deploy/discogs.env

ARM64 := CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath

.PHONY: help check stage image fetch card push status logs pair spotify drivers turntable devices

comma := ,

help:
	@echo "backend - the whole of EdS on one Pi, $(SSH)"
	@echo
	@echo "  check   are the Pi's broker accounts the tenant's?"
	@echo "  image   cross-compile everything and build the image on the Builder"
	@echo "  fetch   copy the built image here, and check its checksum"
	@echo "  card    write this card's settings to its boot partition (BOOT=$(BOOT))"
	@echo "  status  eds-backend-status on the Pi"
	@echo "  pair    pair the agent with iTunes; prints a code to type in"
	@echo "  spotify authorize the agent for Spotify in a browser here, and hand it to the Pi"
	@echo "  drivers which players the Pi follows: DRIVERS=itunes,spotify,turntable"
	@echo "  turntable  which input the turntable driver listens to: DEVICE=plughw:UA4FX"
	@echo "  devices fetch what the stand, the screen and the beacon are provisioned with"
	@echo "  push    update the services, the agent, their units and tools on the running Pi"
	@echo "  logs    follow the services and the agent on the Pi"
	@echo
	@echo "In order, for a new card: image, fetch, flash it, card, boot it, status, pair."
	@echo "PI_HOSTNAME=$(PI_HOSTNAME) and PI_USER=$(PI_USER) name the card and your login on it."

check:
	@python3 $(REPO)/images/backend/check-accounts.py

# Everything the image installs that images/ does not hold: the agent and the
# three Go services for the Pi, palette as a wheel, and the services' settings
# that are not secret, which are the workload's own.
stage: check
	rm -rf $(STAGE)
	mkdir -p $(STAGE)/palette $(STAGE)/settings
	$(MAKE) -C $(NP) build-arm64
	cp $(NP)/bin/npagent-linux-arm64 $(STAGE)/npagent
	cd $(NP) && $(ARM64) -ldflags "-s -w -X main.version=$(VERSION)" -o $(STAGE)/nowplayd ./cmd/nowplayd
	cd $(LIGHTD) && $(ARM64) -ldflags "-s -w" -o $(STAGE)/lightd ./cmd/lightd
	cd $(CONSOLE) && $(ARM64) -ldflags "-s -w -X main.version=$(VERSION)" -o $(STAGE)/consoled ./cmd/consoled
	python3 -m pip wheel --quiet --disable-pip-version-check --no-deps --wheel-dir $(STAGE)/palette $(PALETTE)
	cp $(REPO)/deploy/lightd.env $(REPO)/deploy/nowplayd.env $(REPO)/deploy/console.env $(STAGE)/settings/

# The image definition goes to the Builder as it is here, uncommitted changes
# and all, and is recorded in the image by commit. It has a build directory of
# its own, so nothing the agent image is built from is touched. out/ is the
# Builder's, and is left alone.
image: stage
	ssh $(BUILDER) 'mkdir -p $(BUILD_DIR)/inputs'
	rsync -a --delete --exclude out/ --exclude inputs/ --exclude '._*' --exclude .DS_Store $(REPO)/images/ $(BUILDER):$(BUILD_DIR)/
	rsync -a --delete --exclude '._*' --exclude .DS_Store $(STAGE)/ $(BUILDER):$(BUILD_DIR)/inputs/
	ssh $(BUILDER) 'cd $(BUILD_DIR) && SOURCE=$(VERSION) ./build.sh backend'

fetch:
	scp $(BUILDER):$(BUILD_DIR)/out/$(IMAGE) $(BUILDER):$(BUILD_DIR)/out/$(IMAGE).sha256 .
	shasum -a 256 -c $(IMAGE).sha256

# Interactive: it asks for the Wi-Fi key and a password for your login.
# Nothing it writes is kept here.
card:
	@BOOT="$(BOOT)" PI_HOSTNAME="$(PI_HOSTNAME)" PI_USER="$(PI_USER)" TENANT="$(TENANT)" INDEX="$(INDEX)" DISCOGS="$(DISCOGS)" ./card.sh

status:
	ssh -t $(SSH) eds-backend-status

pair:
	ssh -t $(SSH) eds-agent-pair

logs:
	ssh -t $(SSH) sudo journalctl -u eds-nowplayd -u eds-lightd -u eds-console -u eds-palette -u npagent -f -o short

# Interactive: a browser opens for Agree to be clicked. AGAIN=1 authorizes
# afresh when the Pi already holds one.
spotify:
	$(MAKE) -C $(NP) build
	@SSH="$(SSH)" NPAGENT="$(NP)/bin/npagent" AGAIN="$(AGAIN)" ./spotify.sh

# As deploy/relay's: one line of the agent's settings, deleted and then added
# at the end as two steps, and a restart. The settings file holds the broker
# password and is never printed.
drivers:
	@case ",$(DRIVERS)," in ,,|*,,*) bad=1;; *) bad=0;; esac; \
		for d in $(subst $(comma), ,$(DRIVERS)); do case "$$d" in itunes|spotify|turntable) ;; *) bad=1;; esac; done; \
		if [ $$bad = 1 ]; then echo "DRIVERS must name itunes, spotify, turntable, with commas" >&2; exit 2; fi
	ssh $(SSH) 'sudo sed -i -e "/^NP_DRIVERS=/d" /etc/eds/npagent.env && echo NP_DRIVERS=$(DRIVERS) | sudo tee -a /etc/eds/npagent.env >/dev/null && sudo systemctl restart npagent && sleep 3 && systemctl is-active npagent && sudo grep ^NP_DRIVERS= /etc/eds/npagent.env'

turntable:
	@case "$(DEVICE)" in *[!A-Za-z0-9:,_=-]*|"") echo "DEVICE must be an ALSA name such as plughw:UA4FX" >&2; exit 2;; esac
	ssh $(SSH) 'sudo sed -i -e "/^NP_TURNTABLE_DEVICE=/d" /etc/eds/npagent.env && echo NP_TURNTABLE_DEVICE=$(DEVICE) | sudo tee -a /etc/eds/npagent.env >/dev/null && sudo systemctl restart npagent && sleep 3 && systemctl is-active npagent && sudo grep ^NP_TURNTABLE_DEVICE= /etc/eds/npagent.env'

# Interactive: it asks which Wi-Fi network the devices are to join, and its
# key. What it writes holds passwords, and is gitignored.
devices:
	@SSH="$(SSH)" REPO="$(REPO)" BROKER_HOST="$(BROKER_HOST)" ./devices.sh

# No reflash: the same files the image is built from, onto the Pi as it runs.
push: stage
	@SSH="$(SSH)" REPO="$(REPO)" STAGE="$(STAGE)" VERSION="$(VERSION)" ./push.sh
