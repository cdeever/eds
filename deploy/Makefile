# Deploy palette and lightd to the eds services workload.
#
# The Deploy Your App pattern: build the images here, copy them over SSH, and
# run them under systemd with kit.env. No registry and no operator. A replaced
# workload comes back empty, and `make deploy` puts everything back.
#
# Plain recipes, one shell per line: macOS ships GNU make 3.81.
SHELL := /bin/bash

HOST  ?= tenant@services.eds.mobile.deevnet.net
INFRA := $(CURDIR)/../infra/deevnet-tenant-eds
SVC   := $(CURDIR)/../services

.PHONY: help images load kit-env ship units deploy restart status logs clean

help:
	@echo "deploy - palette and lightd onto $(HOST)"
	@echo
	@echo "  deploy   everything: images, kit.env, units, start"
	@echo "  images   build both images for linux/amd64"
	@echo "  ship     build and copy the images, then restart (a new version)"
	@echo "  load     copy the built images to the workload"
	@echo "  kit-env  write kit.env from the tenant's Terraform outputs"
	@echo "  units    copy kit.env, lightd.env, site-ca.pem and the units; start"
	@echo "  restart  restart both services"
	@echo "  status   systemd status of both"
	@echo "  logs     follow both services' logs"

images:
	podman build --platform linux/amd64 -t localhost/eds-palette:latest $(SVC)/palette
	podman build --platform linux/amd64 -t localhost/eds-lightd:latest $(SVC)/lightd

# Secret: it holds lightd's broker password and the tenant's log tokens.
# Read from the tenant's state, which lives in the state store.
kit-env:
	@umask 077; set -a; . $(INFRA)/.backend.env; set +a; \
		terraform -chdir=$(INFRA) output -raw kit_env > kit.env
	@echo "wrote kit.env (secret, gitignored)"

ship: images
	$(MAKE) load
	ssh $(HOST) sudo systemctl restart eds-palette eds-lightd

units: kit-env
	ssh $(HOST) 'sudo install -d -m 0755 /opt/eds && mkdir -p ~/eds-deploy'
	scp kit.env lightd.env $(INFRA)/site-ca.pem eds-palette.service eds-lightd.service $(HOST):eds-deploy/
	ssh $(HOST) 'cd ~/eds-deploy && \
		sudo install -m 0600 kit.env lightd.env /opt/eds/ && \
		sudo install -m 0644 site-ca.pem /opt/eds/ && \
		sudo install -m 0644 eds-palette.service eds-lightd.service /etc/systemd/system/ && \
		cd && rm -rf ~/eds-deploy && \
		sudo systemctl daemon-reload && sudo systemctl enable eds-palette eds-lightd && \
		sudo systemctl restart eds-palette eds-lightd'

deploy: images
	$(MAKE) load
	$(MAKE) units

# One image per archive: a two-image `podman save` without
# --multi-image-archive loads back as ONE image under both tags.
load:
	podman save localhost/eds-palette:latest | ssh $(HOST) sudo podman load
	podman save localhost/eds-lightd:latest | ssh $(HOST) sudo podman load

restart:
	ssh $(HOST) sudo systemctl restart eds-palette eds-lightd

status:
	ssh $(HOST) systemctl --no-pager status eds-palette eds-lightd

logs:
	ssh $(HOST) sudo journalctl -f -u eds-palette -u eds-lightd

clean:
	rm -f kit.env
