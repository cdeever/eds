# Tenant: eds
#
# Everything that recurs - adding a record, rebuilding, destroying - happens
# here and touches no substrate repository (ADR-0006).
#
# Two things arrive from the substrate at onboarding and are not authored here:
#   fabric.auto.tfvars       make tenant-attachment TENANT=$(CURDIR)
#   TF_VAR_tsig_key_secret   read from the inventory vault, exported
#
.ONESHELL:
SHELL := /usr/bin/bash
.SHELLFLAGS := -euo pipefail -c

# The tdemo pattern assumes the deevnet repositories are siblings of the tenant
# repository. This tenant lives two directories inside the EdS monorepo, so
# they are three levels up instead. Override DEEVNET_ROOT if your checkouts
# live somewhere else.
DEEVNET_ROOT  ?= $(CURDIR)/../../..
IMAGE_FACTORY ?= $(DEEVNET_ROOT)/deevnet-image-factory
INVENTORY     ?= $(DEEVNET_ROOT)/ansible-inventory-deevnet

SITE     ?= mobile
PVE_NODE ?= pve2
PVE_ENV  := $(IMAGE_FACTORY)/build/pve-env/$(PVE_NODE).env
VAULT    := $(INVENTORY)/$(SITE)/group_vars/all/vault.yml

TF_APPROVE := $(if $(AUTO),-auto-approve,)

.PHONY: help paths creds require-secret init plan apply destroy fmt validate

help:
	@echo "eds - tenant repository (inside the EdS monorepo)"
	@echo
	@echo "  paths     show where this expects the deevnet repositories"
	@echo "  init      terraform init (fetches the tagged module; needs GitHub + ssh-agent)"
	@echo "  plan      terraform plan"
	@echo "  apply     terraform apply         (AUTO=1 to skip approval)"
	@echo "  destroy   terraform destroy       (AUTO=1 to skip approval)"
	@echo "  validate  fmt check + validate"
	@echo
	@echo "Requires TF_VAR_tsig_key_secret in the environment - see README."

# The sibling-path assumption is the one thing the monorepo layout changed, so
# make it inspectable rather than something to discover from a failure.
paths:
	@echo "DEEVNET_ROOT  $(DEEVNET_ROOT)"
	@echo "IMAGE_FACTORY $(IMAGE_FACTORY)  $$([[ -d "$(IMAGE_FACTORY)" ]] && echo OK || echo MISSING)"
	@echo "INVENTORY     $(INVENTORY)  $$([[ -d "$(INVENTORY)" ]] && echo OK || echo MISSING)"
	@echo "VAULT         $(VAULT)  $$([[ -f "$(VAULT)" ]] && echo OK || echo MISSING)"

check-paths:
	@if [[ ! -d "$(IMAGE_FACTORY)" ]]; then
		echo "deevnet-image-factory not found at $(IMAGE_FACTORY)" >&2
		echo "This tenant lives inside the EdS monorepo, so the deevnet repositories" >&2
		echo "are expected three levels up rather than one. Set DEEVNET_ROOT if yours" >&2
		echo "are elsewhere:  make plan DEEVNET_ROOT=/path/to/checkouts" >&2
		exit 2
	fi

$(PVE_ENV): check-paths
	$(MAKE) creds

creds: check-paths
	$(MAKE) -C "$(IMAGE_FACTORY)" $(PVE_NODE)-env

# The TSIG secret is delivery-by-environment on purpose: it is a secret this
# repository must never hold.
require-secret:
	@if [[ -z "$${TF_VAR_tsig_key_secret:-}" ]]; then
		echo "TF_VAR_tsig_key_secret is not set." >&2
		echo "It is issued by the substrate and lives in the inventory vault." >&2
		echo "  export TF_VAR_tsig_key_secret=\$$(ansible-vault view \\" >&2
		echo "    $(VAULT) \\" >&2
		echo "    | yq -r .vault_tenant_tsig_keys.eds)" >&2
		exit 2
	fi

init: $(PVE_ENV)
	terraform init

plan: require-secret $(PVE_ENV)
	source "$(PVE_ENV)"
	terraform plan

apply: require-secret $(PVE_ENV)
	source "$(PVE_ENV)"
	terraform apply $(TF_APPROVE)

destroy: require-secret $(PVE_ENV)
	source "$(PVE_ENV)"
	terraform destroy $(TF_APPROVE)

fmt:
	terraform fmt -recursive

validate:
	terraform fmt -check -recursive
	terraform validate
