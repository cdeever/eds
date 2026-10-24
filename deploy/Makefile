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

.PHONY: help images load kit-env broker-env tls-files discogs-env ship units deploy restart status logs clean

help:
	@echo "deploy - palette, lightd, nowplayd, the console, scribe and voiced onto $(HOST)"
	@echo
	@echo "  deploy   everything: images, settings, units, start"
	@echo "  images   build the six images for linux/amd64"
	@echo "  ship     build and copy the images, then restart (a new version)"
	@echo "  load     copy the built images to the workload"
	@echo "  kit-env  write kit.env and each service's broker login from the tenant's Terraform outputs"
	@echo "  tls-files  write the console's key and certificate from the tenant's Terraform outputs"
	@echo "  units    copy the settings, deevnet-root-ca.pem and the units; start"
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
	rsync -a Makefile $(BUILDER):$(BUILD_DIR)/deploy/Makefile
	ssh $(BUILDER) 'make -C $(BUILD_DIR)/deploy images VERSION=$(VERSION)'

load:
	ssh $(BUILDER) 'make -C $(BUILD_DIR)/deploy load HOST=$(HOST)'

else

# Nothing is left behind. With layers kept, every build left its build stage,
# about a gigabyte, untagged, and the Builder's /home filled (CHG-0025): so
# no layers, at about fifteen seconds an image for the cache it would have
# been. What a build replaces - the image that was :latest - is untagged too,
# and carries the label, so it alone is pruned: the Builder builds Deevnet
# IoTaaS's images as well, and nothing of theirs matches.
BUILD := $(CONTAINER) build --platform linux/amd64 --layers=false --label eds.deploy=1

images:
	$(BUILD) -t localhost/eds-palette:latest $(SVC)/palette
	$(BUILD) -t localhost/eds-lightd:latest $(SVC)/lightd
	$(BUILD) --build-arg VERSION=$(VERSION) -t localhost/eds-nowplayd:latest -f $(SVC)/nowplaying/Containerfile $(SVC)/nowplaying
	$(BUILD) --build-arg VERSION=$(VERSION) -t localhost/eds-console:latest -f $(SVC)/console/Containerfile $(SVC)/console
	$(BUILD) -t localhost/eds-scribe:latest $(SVC)/scribe
	$(BUILD) --build-arg VERSION=$(VERSION) -t localhost/eds-voiced:latest -f $(SVC)/nowplaying/Containerfile.voiced $(SVC)/nowplaying
	$(CONTAINER) image prune -f --filter label=eds.deploy=1 >/dev/null

# One image per archive: a multi-image `podman save` without
# --multi-image-archive loads back as ONE image under every tag.
load:
	$(CONTAINER) save localhost/eds-palette:latest | ssh $(HOST) sudo podman load
	$(CONTAINER) save localhost/eds-lightd:latest | ssh $(HOST) sudo podman load
	$(CONTAINER) save localhost/eds-nowplayd:latest | ssh $(HOST) sudo podman load
	$(CONTAINER) save localhost/eds-console:latest | ssh $(HOST) sudo podman load
	$(CONTAINER) save localhost/eds-scribe:latest | ssh $(HOST) sudo podman load
	$(CONTAINER) save localhost/eds-voiced:latest | ssh $(HOST) sudo podman load

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
