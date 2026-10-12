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

.PHONY: help image fetch card push spotify drivers turntable status pair logs

comma := ,

help:
	@echo "relay - the now-playing agent on $(PI)"
	@echo
	@echo "  image   cross-compile the agent and build the image on the Builder"
	@echo "  fetch   copy the built image here, and check its checksum"
	@echo "  card    write this card's settings to its boot partition (BOOT=$(BOOT))"
	@echo "  push    update the agent, its unit and its tools on the running Pi"
	@echo "  status  eds-agent-status on the Pi"
	@echo "  pair    pair the agent with iTunes; prints a code to type in"
	@echo "  spotify authorize the agent for Spotify in a browser here, and hand it to the Pi"
	@echo "  drivers which players the Pi follows: DRIVERS=itunes turns Spotify off,"
	@echo "          DRIVERS=itunes,spotify turns it back on; turntable is the third"
	@echo "  turntable  which input the turntable driver listens to: DEVICE=plughw:UA4FX"
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

# No reflash: the same files the image is built from, onto the Pi as it runs.
push:
	$(MAKE) -C $(NP) build-arm64
	@BUILDER="$(BUILDER)" PI="$(PI)" REPO="$(REPO)" ./push.sh

# Interactive: a browser opens for Agree to be clicked. AGAIN=1 authorizes
# afresh when the Pi already holds one.
spotify:
	$(MAKE) -C $(NP) build
	@BUILDER="$(BUILDER)" PI="$(PI)" NPAGENT="$(NP)/bin/npagent" AGAIN="$(AGAIN)" ./spotify.sh

# The off switch for a driver, and the on switch. It changes one line of the
# Pi's settings and restarts the agent. Nothing else is touched: a driver
# turned off keeps its pairing or its authorization, and has it when turned
# back on. The settings file holds the broker password and is never printed.
#
# The line is deleted and then added at the end, as two steps. As one sed
# script ("d", then "$$a" on the last line) it lost the line whenever that
# line was already the last: the second time this was run, the setting was
# gone and the agent started the image's default drivers (CHG-0013).
drivers:
	@case ",$(DRIVERS)," in ,,|*,,*) bad=1;; *) bad=0;; esac; \
		for d in $(subst $(comma), ,$(DRIVERS)); do case "$$d" in itunes|spotify|turntable) ;; *) bad=1;; esac; done; \
		if [ $$bad = 1 ]; then echo "DRIVERS must name itunes, spotify, turntable, with commas" >&2; exit 2; fi
	ssh $(BUILDER) "ssh a_autoprov@$(PI) 'sudo sed -i -e \"/^NP_DRIVERS=/d\" /etc/eds/npagent.env && echo NP_DRIVERS=$(DRIVERS) | sudo tee -a /etc/eds/npagent.env >/dev/null && sudo systemctl restart npagent && sleep 3 && systemctl is-active npagent && sudo grep ^NP_DRIVERS= /etc/eds/npagent.env'"

# Which input the turntable driver listens to, as ALSA names it: plughw:<card>.
# `arecord -L` on the Pi lists them. Like `drivers`, one line of the Pi's
# settings and a restart; the driver itself is turned on with `drivers`.
turntable:
	@case "$(DEVICE)" in *[!A-Za-z0-9:,_=-]*|"") echo "DEVICE must be an ALSA name such as plughw:UA4FX" >&2; exit 2;; esac
	ssh $(BUILDER) "ssh a_autoprov@$(PI) 'sudo sed -i -e \"/^NP_TURNTABLE_DEVICE=/d\" /etc/eds/npagent.env && echo NP_TURNTABLE_DEVICE=$(DEVICE) | sudo tee -a /etc/eds/npagent.env >/dev/null && sudo systemctl restart npagent && sleep 3 && systemctl is-active npagent && sudo grep ^NP_TURNTABLE_DEVICE= /etc/eds/npagent.env'"

# The Pi is reached through the Builder, as the site's automation reaches it:
# it admits SSH on its wired side only, to the automation user.
status:
	ssh -t $(BUILDER) 'ssh -o StrictHostKeyChecking=accept-new a_autoprov@$(PI) eds-agent-status'

pair:
	ssh -t $(BUILDER) 'ssh -t a_autoprov@$(PI) eds-agent-pair'

logs:
	ssh -t $(BUILDER) 'ssh a_autoprov@$(PI) sudo journalctl -u npagent -f -o cat'
