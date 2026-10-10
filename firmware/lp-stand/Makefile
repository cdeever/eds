# LP jacket stand - firmware.
#
# The hardware-independent half (scene validation, effects, gamma) is tested
# with `make test` and needs nothing but a C compiler. Everything else needs
# ESP-IDF on the PATH and, to be certain, a device.
.ONESHELL:
# bash sits at /usr/bin/bash on most Linux distributions and /bin/bash on
# macOS, so resolve it rather than hardcoding one and failing on the other.
SHELL := $(shell command -v bash)
.SHELLFLAGS := -euo pipefail -c

PORT ?= /dev/ttyUSB0

# Must match LP_CRED_NAMESPACE in main/config/lp_credentials.h.
LP_CRED_NAMESPACE ?= lp-stand

# The ESP-IDF Installation Manager exposes idf.py as a shell function rather
# than adding it to PATH, so a recipe's subshell cannot see it. Resolve the
# script out of IDF_PATH when it is there, and fall back to PATH for the
# classic export.sh layout.
IDF_PY := $(if $(wildcard $(IDF_PATH)/tools/idf.py),$(IDF_PATH)/tools/idf.py,idf.py)

.PHONY: help test clean build flash monitor menuconfig wifi-config provision logs

help:
	@echo "lp-stand - available targets"
	@echo
	@echo "  test        host tests for scene/effects/gamma (no ESP-IDF needed)"
	@echo "  menuconfig  configure WiFi, broker and strip"
	@echo "  build       idf.py build"
	@echo "  flash       idf.py flash        (PORT=$(PORT))"
	@echo "  monitor     idf.py monitor      (PORT=$(PORT))"
	@echo "  provision   write what the EdS tenant issued this stand into flash (PORT=$(PORT))"
	@echo "  wifi-config the same partition, typed by hand - for a stand off the substrate"
	@echo "  logs        the stands' events from the tenant's log store (ARGS='-f', ARGS='--since 1d')"
	@echo "  clean       remove host test artifacts"
	@echo
	@echo "ESP-IDF targets need . \$$IDF_PATH/export.sh first."

test:
	$(MAKE) -C test/host test

menuconfig:
	"$(IDF_PY)" menuconfig

build:
	"$(IDF_PY)" build

flash:
	"$(IDF_PY)" -p "$(PORT)" flash

monitor:
	"$(IDF_PY)" -p "$(PORT)" monitor

# --- provisioning ----------------------------------------------------------

# The usual way: everything the stand is flashed with was issued by the EdS
# tenant and is in its Terraform outputs, so read it from there. The WiFi key,
# the broker account and the CA the broker is verified against go into the
# creds partition; nothing is typed, and nothing reaches the build.
#
# Needs the tenant directory able to read its state: see
# ../../infra/deevnet-tenant-eds.
provision:
	python3 tools/provision_tenant.py -p "$(PORT)" $(if $(STAND),--stand "$(STAND)",)

# What the stands have reported, read back from the tenant's device log
# partition. ARGS passes through: -f to follow, --since 1d, a LogsQL filter.
logs:
	python3 tools/stand_logs.py $(ARGS)

# Read out of partitions.csv rather than repeated here, so the table stays the
# only place the flash layout is written down.
CREDS_OFFSET := $(shell awk -F, '$$1=="creds"{gsub(/[ \t]/,"",$$4); print $$4}' partitions.csv)
CREDS_SIZE   := $(shell awk -F, '$$1=="creds"{gsub(/[ \t]/,"",$$5); print $$5}' partitions.csv)
NVS_GEN      := $(IDF_PATH)/components/nvs_flash/nvs_partition_generator/nvs_partition_gen.py

# Writes credentials straight into the creds partition, so the secret never
# reaches sdkconfig, the build directory or the firmware image - and a stand
# can be reprovisioned without rebuilding or reflashing the application.
#
# Values are prompted for rather than taken from make variables or the
# environment: a variable on the command line is visible in the process list
# and lands in shell history. Leaving a field blank omits its key: the
# firmware then uses the Kconfig default for the SSID, URI or username, and
# has no key at all for a password, which has no other source.
wifi-config:
	if [[ -z "$(CREDS_OFFSET)" ]]; then
		echo "partitions.csv has no creds partition." >&2
		exit 2
	fi
	if [[ ! -f "$(NVS_GEN)" ]]; then
		echo "ESP-IDF not found. Run . \$$IDF_PATH/export.sh first." >&2
		exit 2
	fi

	read -r -p "WiFi SSID: " LP_SSID
	read -r -s -p "WiFi password: " LP_WIFI_PASS; echo
	read -r -p "Broker URI (blank for the built-in default): " LP_MQTT_URI
	read -r -p "Broker username (blank for the default): " LP_MQTT_USER
	read -r -s -p "Broker password: " LP_MQTT_PASS; echo
	export LP_SSID LP_WIFI_PASS LP_MQTT_URI LP_MQTT_USER LP_MQTT_PASS
	export LP_NAMESPACE="$(LP_CRED_NAMESPACE)"

	# Outside the repository, and removed on any exit: this file holds the
	# plaintext credentials for as long as it takes to build the image.
	work="$$(mktemp -d)"
	trap 'rm -rf "$$work"' EXIT
	(umask 077 && touch "$$work/creds.csv")

	# Written with the csv module rather than by echo, so a password holding a
	# comma, a quote or a space survives into the image intact.
	python3 - "$$work/creds.csv" <<-'PYGEN'
		import csv, os, sys
		fields = [
		    ("wifi_ssid", "LP_SSID"),
		    ("wifi_pass", "LP_WIFI_PASS"),
		    ("mqtt_uri",  "LP_MQTT_URI"),
		    ("mqtt_user", "LP_MQTT_USER"),
		    ("mqtt_pass", "LP_MQTT_PASS"),
		]
		with open(sys.argv[1], "w", newline="") as fh:
		    out = csv.writer(fh)
		    out.writerow(["key", "type", "encoding", "value"])
		    out.writerow([os.environ["LP_NAMESPACE"], "namespace", "", ""])
		    for key, env in fields:
		        value = os.environ.get(env, "")
		        if value:
		            out.writerow([key, "data", "string", value])
	PYGEN

	python3 "$(NVS_GEN)" generate "$$work/creds.csv" "$$work/creds.bin" $(CREDS_SIZE) >/dev/null
	python3 -m esptool -p "$(PORT)" write-flash $(CREDS_OFFSET) "$$work/creds.bin"
	echo
	echo "Provisioned $(CREDS_SIZE) bytes at $(CREDS_OFFSET). Reset the board to pick them up."

clean:
	$(MAKE) -C test/host clean
