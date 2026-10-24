# Deploy palette, lightd, nowplayd, the console, scribe and voiced to the eds
# services workload.
#
# The Deploy Your App pattern: build the images, copy them over SSH, and run
# them under systemd with kit.env. No registry and no operator. A replaced
# workload comes back empty, and `make deploy` puts everything back.
#
# Plain recipes, one shell per line: macOS ships GNU make 3.81.
SHELL := /bin/bash

HOST  ?= tenant@services.eds.mobile.deevnet.net
REPO  := $(CURDIR)/..
INFRA := $(REPO)/infra/deevnet-tenant-eds
SVC   := $(REPO)/services

# The container tool that builds the images. Where there is none - a Mac with
# no podman - `make WHERE=builder …` builds and loads from the Builder instead,
# which has one and can reach the workload.
CONTAINER ?= podman
WHERE     ?= local
BUILDER   ?= cdeever@builder
BUILD_DIR ?= /srv/eds-deploy-build
VERSION   ?= $(shell git -C $(REPO) describe --always --dirty 2>/dev/null || echo dev)

SERVICES := eds-palette eds-lightd eds-nowplayd eds-console eds-scribe eds-voiced
UNITS    := $(addsuffix .service,$(SERVICES))

.PHONY: help images load kit-env broker-env tls-files discogs-env ship units library deploy restart prune status logs clean

help:
	@echo "deploy - palette, lightd, nowplayd, the console, scribe and voiced onto $(HOST)"
	@echo
	@echo "  deploy   everything: images, settings, units, start"
	@echo "  images   build the six images for linux/amd64"
	@echo "  ship     build, copy the images that changed (rate-capped), then restart"
	@echo "  load     copy the built images to the workload"
	@echo "  kit-env  write kit.env and each service's broker login from the tenant's Terraform outputs"
	@echo "  tls-files  write the console's key and certificate from the tenant's Terraform outputs"
	@echo "  units    copy the settings, deevnet-root-ca.pem and the units; start"
	@echo "  library  index XML=\"…/iTunes Library.xml\" and copy the index to voiced"
	@echo "  restart  restart the services"
	@echo "  status   systemd status of each"
	@echo "  logs     follow the services' logs"
	@echo
	@echo "No container tool here? Add WHERE=builder: images are built on, and loaded"
	@echo "from, $(BUILDER). Settings and units still go from this machine."

ifeq ($(WHERE),builder)

# Only what a build needs goes to the Builder: the services' sources and this
# Makefile. No settings, no secrets.
images:
	ssh $(BUILDER) 'mkdir -p $(BUILD_DIR)/deploy'
	rsync -a --delete --exclude .venv/ --exclude bin/ --exclude __pycache__/ --exclude '._*' --exclude .DS_Store --exclude .pytest_cache/ $(SVC)/ $(BUILDER):$(BUILD_DIR)/services/
	rsync -a Makefile load.sh build-there.sh $(BUILDER):$(BUILD_DIR)/deploy/
	ssh $(BUILDER) 'make -C $(BUILD_DIR)/deploy images VERSION=$(VERSION)'

load:
	rsync -a load.sh build-there.sh $(BUILDER):$(BUILD_DIR)/deploy/
	ssh $(BUILDER) 'make -C $(BUILD_DIR)/deploy load HOST=$(HOST)'

else

# Nothing is left behind. With layers kept, every build left its build stage,
# about a gigabyte, untagged, and the Builder's /home filled (CHG-0025): so
# no layers, at about fifteen seconds an image for the cache it would have
# been. What a build replaces - the image that was :latest - is untagged too,
# and carries the label, so it alone is pruned: the Builder builds Deevnet
# IoTaaS's images as well, and nothing of theirs matches.
BUILD := $(CONTAINER) build --platform linux/amd64 --layers=false --label eds.deploy=1

# What an image was built from, as a hash: its directory's files, as the
# images target sees them. load.sh sends an image only when the workload's
# carries a different one (CHG-0037).
source = $(shell cd $(SVC)/$(1) && find . -type f -not -path './.venv/*' -not -path './bin/*' -not -path '*/__pycache__/*' -not -path './.pytest_cache/*' | LC_ALL=C sort | xargs sha256sum | sha256sum | cut -c1-16)
# The same for named files in a directory: scribe's base is its
# Containerfile.base and the dependencies in pyproject.toml, and nothing else.
files = $(shell cd $(SVC)/$(1) && sha256sum $(2) | sha256sum | cut -c1-16)

# An image already built from the same sources is not built again: a build
# fetches its base image, packages and, for scribe, its model, all through
# the core router (INC-0002), to make an image the same as the last.
# $(1) the image, $(2) its directory, then the build's own arguments.
# $(1) the image, $(2) its sources' hash, then the build's own arguments.
built = $(CONTAINER) image inspect --format '{{index .Labels "eds.source"}}' localhost/eds-$(1):latest 2>/dev/null | grep -qx '$(2)'
build = if $(call built,$(1),$(2)); then echo "$(1): sources unchanged ($(2)), not built"; \
	else $(BUILD) --label eds.source=$(2) -t localhost/eds-$(1):latest $(3); fi

images:
	@$(call build,palette,$(call source,palette),$(SVC)/palette)
	@$(call build,lightd,$(call source,lightd),$(SVC)/lightd)
	@$(call build,nowplayd,$(call source,nowplaying),--build-arg VERSION=$(VERSION) -f $(SVC)/nowplaying/Containerfile $(SVC)/nowplaying)
	@$(call build,console,$(call source,console),--build-arg VERSION=$(VERSION) -f $(SVC)/console/Containerfile $(SVC)/console)
	@$(call build,scribe-base,$(call files,scribe,Containerfile.base pyproject.toml),-f $(SVC)/scribe/Containerfile.base $(SVC)/scribe)
	@$(call build,voiced,$(call source,nowplaying),--build-arg VERSION=$(VERSION) -f $(SVC)/nowplaying/Containerfile.voiced $(SVC)/nowplaying)
	$(CONTAINER) image prune -f --filter label=eds.deploy=1 >/dev/null

# Only what changed, a file at a time, compressed, at a capped rate: load.sh. One image
# per archive: a multi-image `podman save` without --multi-image-archive
# loads back as ONE image under every tag.
#
# scribe is two: its base is sent as the others are, and its own code is
# built on the workload, on that base, from its sources (build-there.sh).
load:
	CONTAINER=$(CONTAINER) ./load.sh $(HOST) palette lightd nowplayd console scribe-base voiced
	./build-there.sh $(HOST) scribe $(SVC)/scribe $(call source,scribe)

endif

# Secret: kit.env holds lightd's broker password and the tenant's log tokens,
# and nowplayd-broker.env and console-broker.env hold those services' own.
# ota-broker.env is eds-ota's, which no service runs: `make ota` in a firmware
# directory does, on the workload (ADR-0035).
# All are read from the tenant's state, which lives in the state store, and
# none is committed.
kit-env: broker-env
	@umask 077; set -a; . $(INFRA)/.backend.env; set +a; \
		terraform -chdir=$(INFRA) output -raw kit_env > kit.env
	@echo "wrote kit.env (secret, gitignored)"

# kit.env carries one login, lightd's. A second service on the same workload
# names its own beside it.
broker-env:
	@umask 077; set -a; . $(INFRA)/.backend.env; set +a; \
		terraform -chdir=$(INFRA) output -json nowplayd_broker | python3 -c 'import json,sys; a=json.load(sys.stdin); print("NP_MQTT_USERNAME=%s\nNP_MQTT_PASSWORD=%s" % (a["username"], a["password"]))' > nowplayd-broker.env
	@echo "wrote nowplayd-broker.env (secret, gitignored)"
	@umask 077; set -a; . $(INFRA)/.backend.env; set +a; \
		terraform -chdir=$(INFRA) output -json console_broker | python3 -c 'import json,sys; a=json.load(sys.stdin); print("CONSOLE_MQTT_USERNAME=%s\nCONSOLE_MQTT_PASSWORD=%s" % (a["username"], a["password"]))' > console-broker.env
	@echo "wrote console-broker.env (secret, gitignored)"
	@umask 077; set -a; . $(INFRA)/.backend.env; set +a; \
		terraform -chdir=$(INFRA) output -json ota_broker | python3 -c 'import json,sys; a=json.load(sys.stdin); print("OTA_MQTT_USERNAME=%s\nOTA_MQTT_PASSWORD=%s" % (a["username"], a["password"]))' > ota-broker.env
	@echo "wrote ota-broker.env (secret, gitignored)"
	@umask 077; set -a; . $(INFRA)/.backend.env; set +a; \
		terraform -chdir=$(INFRA) output -json voiced_broker | python3 -c 'import json,sys; a=json.load(sys.stdin); print("NP_MQTT_USERNAME=%s\nNP_MQTT_PASSWORD=%s" % (a["username"], a["password"]))' > voiced-broker.env
	@echo "wrote voiced-broker.env (secret, gitignored)"

# Secret: the console's key. It was made in the tenant's state and has been
# nowhere else; the certificate beside it is public. tls/ is gitignored.
tls-files:
	@mkdir -p tls && chmod 700 tls
	@umask 077; set -a; . $(INFRA)/.backend.env; set +a; \
		terraform -chdir=$(INFRA) output -json console_tls | python3 -c 'import json,sys; t=json.load(sys.stdin); open("tls/console.key","w").write(t["key"]); open("tls/console.crt","w").write(t["fullchain"])'
	@openssl x509 -in tls/console.crt -noout -subject -enddate | sed 's/^/  /'
	@echo "wrote tls/console.key and tls/console.crt (the key is secret; both gitignored)"

ship: images
	$(MAKE) load
	ssh $(HOST) sudo systemctl restart $(SERVICES)
	$(MAKE) prune

# What a deploy replaced, untagged now, once the services have let go of it:
# the workload has room for a few of these, not one every deploy. Before the
# restart, the running containers still hold the old ones, and podman will
# not prune an image in use. Every untagged image, not only those labelled
# eds.deploy: images loaded or relabelled there lose the label, and fifty
# were left after one evening. The workload holds EdS's images and nothing
# else.
prune:
	ssh $(HOST) 'sudo podman image prune -f >/dev/null'

# discogs.env is kept by hand: NP_DISCOGS_USER and NP_DISCOGS_TOKEN, a personal
# access token from discogs.com/settings/developers. Secret, gitignored, and
# not removed by `clean`: nothing here can make it again.
discogs-env:
	@if [[ ! -s discogs.env ]]; then \
		echo "No discogs.env here. nowplayd.env names a source to look up in a Discogs" >&2; \
		echo "collection (ADR-0022), and discogs.env says whose:" >&2; \
		echo "  NP_DISCOGS_USER=<the Discogs username>" >&2; \
		echo "  NP_DISCOGS_TOKEN=<a personal access token>" >&2; \
		exit 2; fi
	@chmod 600 discogs.env

units: kit-env tls-files discogs-env
	ssh $(HOST) 'sudo install -d -m 0755 /opt/eds && mkdir -p ~/eds-deploy'
	scp kit.env lightd.env nowplayd.env nowplayd-broker.env discogs.env console.env console-broker.env ota-broker.env voiced-broker.env $(INFRA)/deevnet-root-ca.pem tls/console.key tls/console.crt $(UNITS) $(HOST):eds-deploy/
	ssh $(HOST) 'cd ~/eds-deploy && \
		sudo install -m 0600 kit.env lightd.env nowplayd.env nowplayd-broker.env discogs.env console.env console-broker.env ota-broker.env voiced-broker.env /opt/eds/ && \
		sudo install -m 0644 deevnet-root-ca.pem /opt/eds/ && \
		sudo install -d -m 0700 -o 65532 -g 65532 /opt/eds/tls && \
		sudo install -d -m 0700 -o 65532 -g 65532 /var/lib/eds-voiced && \
		sudo install -m 0400 -o 65532 -g 65532 console.key console.crt /opt/eds/tls/ && \
		sudo install -m 0644 $(UNITS) /etc/systemd/system/ && \
		cd && rm -rf ~/eds-deploy && \
		sudo firewall-cmd --quiet --permanent --add-service=http --add-service=https && sudo firewall-cmd --quiet --reload && \
		sudo systemctl daemon-reload && sudo systemctl enable $(SERVICES) && \
		sudo systemctl restart $(SERVICES)'

# The owner's library, as voiced matches requests against it (ADR-0046):
# indexed here, from the XML iTunes writes, and only the index sent - a few
# hundred KB of artists, albums and years, no paths or play counts. voiced
# reads it again at its next request. Neither file is committed. It takes
# the directory's SELinux label, which podman's :Z gave voiced's container:
# installed, it would be var_lib_t, and unreadable there (2026-10-24).
library:
	@if [[ ! -s "$(XML)" ]]; then echo 'usage: make library XML="/path/to/iTunes Library.xml"' >&2; exit 2; fi
	@umask 077; cd $(SVC)/nowplaying && go run ./cmd/library index "$(XML)" > $(CURDIR)/library.json
	@ls -l library.json | awk '{print "  " $$5 " bytes"}'
	scp -l 40000 library.json $(HOST):library.json
	ssh $(HOST) 'sudo install -m 0400 -o 65532 -g 65532 library.json /var/lib/eds-voiced/ && sudo chcon --reference=/var/lib/eds-voiced /var/lib/eds-voiced/library.json && rm -f library.json'
	@rm -f library.json

deploy: images
	$(MAKE) load
	$(MAKE) units

restart:
	ssh $(HOST) sudo systemctl restart $(SERVICES)

status:
	ssh $(HOST) systemctl --no-pager status $(SERVICES)

logs:
	ssh $(HOST) sudo journalctl -f $(addprefix -u ,$(SERVICES))

clean:
	rm -f kit.env nowplayd-broker.env console-broker.env ota-broker.env
	rm -rf tls
