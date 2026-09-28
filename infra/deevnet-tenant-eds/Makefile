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

# The state store's credentials (secret, gitignored). State lives in the store,
# so every command needs them; `make backend-env` writes the file.
-include .backend.env
export AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY

TF_APPROVE := $(if $(AUTO),-auto-approve,)

.PHONY: help require-token init plan apply destroy state-backend backend-env fmt validate

help:
	@echo "eds - tenant (inside the EdS monorepo)"
	@echo
	@echo "  init           terraform init"
	@echo "  plan           terraform plan"
	@echo "  apply          terraform apply    (AUTO=1 to skip approval)"
	@echo "  destroy        terraform destroy  (AUTO=1 to skip approval)"
	@echo "  state-backend  print the backend block this tenant was issued"
	@echo "  backend-env    write .backend.env (the state store's credentials)"
	@echo "                 from terraform.tfstate.backup, the pre-migration state"
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

# The state store's credentials are outputs of the tenant, and the tenant's
# state is IN the store - so they can't be read from it without them. The
# local state left behind by the migration still holds them.
backend-env:
	@python3 -c '
	import json, os
	s = json.load(open("terraform.tfstate.backup"))
	t = next(r for r in s["resources"] if r["type"] == "deevnet_tenant")["instances"][0]["attributes"]
	fd = os.open(".backend.env", os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
	os.write(fd, ("AWS_ACCESS_KEY_ID=%s\nAWS_SECRET_ACCESS_KEY=%s\n" % (t["state_access_key"], t["state_secret_key"])).encode())
	os.close(fd)
	print("wrote .backend.env")
	'

fmt:
	terraform fmt -recursive

validate:
	terraform fmt -check -recursive
	terraform validate
