# The relay: an agent Pi with a second leg, on the Deevnet site by Ethernet
# and the home LAN by Wi-Fi (ADR-0010).
#
# The image is EdS's own (images/agent), built on the substrate's base. This
# directory is the rest: getting the image built, and writing a flashed card's
# settings - which are secret, and are in no image.
#
# Plain recipes, one shell per line: macOS ships GNU make 3.81.
SHELL := /bin/bash

BUILDER   ?= cdeever@builder
BUILD_DIR ?= /srv/eds-image-build
FACTORY   ?= /srv/dvnt/deevnet-image-factory
PI        ?= dv02rpi002p01.mobile.deevnet.net
IMAGE     := eds-pi-agent.img.xz

REPO  := $(CURDIR)/../..
INFRA := $(REPO)/infra/deevnet-tenant-eds
NP    := $(REPO)/services/nowplaying

# Where the flashed card's boot partition is mounted. macOS names it bootfs.
BOOT ?= /Volumes/bootfs

.PHONY: help image fetch card status pair logs

help:
	@echo "relay - the now-playing agent on $(PI)"
	@echo
	@echo "  image   cross-compile the agent and build the image on the Builder"
	@echo "  fetch   copy the built image here, and check its checksum"
	@echo "  card    write this card's settings to its boot partition (BOOT=$(BOOT))"
	@echo "  status  eds-agent-status on the Pi"
	@echo "  pair    pair the agent with iTunes; prints a code to type in"
	@echo "  logs    follow the agent on the Pi"
	@echo
	@echo "In order, for a new card: image, fetch, flash it, card, boot it, status, pair."

# The image definition goes to the Builder as it is here, uncommitted changes
# and all, and is recorded in the image by commit. --delete keeps a renamed
# file from lingering there; out/ is the Builder's, and is left alone.
image:
	$(MAKE) -C $(NP) build-arm64
	ssh $(BUILDER) 'mkdir -p $(BUILD_DIR)/inputs'
	rsync -a --delete --exclude out/ --exclude inputs/ --exclude '._*' --exclude .DS_Store $(REPO)/images/ $(BUILDER):$(BUILD_DIR)/
	scp -q $(NP)/bin/npagent-linux-arm64 $(BUILDER):$(BUILD_DIR)/inputs/npagent
	ssh $(BUILDER) 'cd $(BUILD_DIR) && FACTORY=$(FACTORY) SOURCE=$(shell git -C $(REPO) describe --always --dirty) ./build.sh agent'

fetch:
	scp $(BUILDER):$(BUILD_DIR)/out/$(IMAGE) $(BUILDER):$(BUILD_DIR)/out/$(IMAGE).sha256 .
	shasum -a 256 -c $(IMAGE).sha256

# Interactive: it asks for the home Wi-Fi key. Nothing it writes is kept here.
card:
	@INFRA="$(INFRA)" BOOT="$(BOOT)" PI_HOSTNAME="$(firstword $(subst ., ,$(PI)))" ./card.sh

# The Pi is reached through the Builder, as the site's automation reaches it:
# it admits SSH on its wired side only, to the automation user.
status:
	ssh -t $(BUILDER) 'ssh -o StrictHostKeyChecking=accept-new a_autoprov@$(PI) eds-agent-status'

pair:
	ssh -t $(BUILDER) 'ssh -t a_autoprov@$(PI) eds-agent-pair'

logs:
	ssh -t $(BUILDER) 'ssh a_autoprov@$(PI) sudo journalctl -u npagent -f -o cat'
