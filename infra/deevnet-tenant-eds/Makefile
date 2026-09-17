# EdS tenant.
#
# EdS holds one credential: its Deevnet API token. No vault, no Proxmox
# credential, no index. The substrate issues the rest through the API
# (ADR-0015), and it lands in this tenant's Terraform state.
.ONESHELL:
SHELL := /usr/bin/bash
.SHELLFLAGS := -euo pipefail -c

export DEEVNET_API_ENDPOINT ?= https://api.mobile.deevnet.net:8080
export DEEVNET_API_CACERT   ?= $(CURDIR)/site-ca.pem

TF_APPROVE := $(if $(AUTO),-auto-approve,)

.PHONY: help require-token init plan apply destroy state-backend fmt validate

help:
	@echo "eds - tenant (inside the EdS monorepo)"
	@echo
	@echo "  init           terraform init"
	@echo "  plan           terraform plan"
	@echo "  apply          terraform apply    (AUTO=1 to skip approval)"
	@echo "  destroy        terraform destroy  (AUTO=1 to skip approval)"
	@echo "  state-backend  print the backend block this tenant was issued"
	@echo "  validate       fmt check + validate"
	@echo
	@echo "Needs DEEVNET_API_TOKEN: the enrollment token on the first apply,"
	@echo "then the api_token output. Endpoint: $(DEEVNET_API_ENDPOINT)"

require-token:
	@if [[ -z "$${DEEVNET_API_TOKEN:-}" ]]; then
		echo "DEEVNET_API_TOKEN is not set." >&2
		echo "  First apply:  the enrollment token the substrate issued at admission." >&2
		echo "  After that:   terraform output -raw api_token" >&2
		exit 2
	fi

init:
	terraform init

plan: require-token
	terraform plan

apply: require-token
	terraform apply $(TF_APPROVE)

destroy: require-token
	terraform destroy $(TF_APPROVE)

state-backend:
	@terraform output -json state_backend | python3 -c '
	import json, sys
	d = json.load(sys.stdin)
	print("# Uncomment the backend block in main.tf with these values, then")
	print("# export AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY and run")
	print("#   terraform init -migrate-state")
	print()
	print("  bucket    = \"%s\"" % d["bucket"])
	print("  key       = \"%s\"" % d["key"])
	print("  endpoints = { s3 = \"%s\" }" % d["endpoint"])
	print("  access_key: %s" % d["access_key"])
	'

fmt:
	terraform fmt -recursive

validate:
	terraform fmt -check -recursive
	terraform validate
