# EdS tenant. `make help` for the targets; README.md for the steps.
#
# EdS holds one credential: its Deevnet API token. No vault, no Proxmox
# credential, no index. The substrate issues the rest through the API
# (ADR-0015), and it lands in this tenant's Terraform state.
#
# Plain recipes, one shell per line: macOS ships GNU make 3.81, which has no
# .ONESHELL. /bin/bash is on macOS and Linux alike.
SHELL := /bin/bash

export DEEVNET_API_ENDPOINT ?= https://api.mobile.deevnet.net:8080
export DEEVNET_API_CACERT   ?= $(CURDIR)/site-ca.pem

# The state store's credentials, once `make state-backend` has written them.
# Secret, gitignored, and needed by every command once state lives in the store.
-include .backend.env
export AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY

TENANT := eds

TF_APPROVE := $(if $(AUTO),-auto-approve,)

.PHONY: help require-token check-backend init plan apply destroy state-backend fmt validate

help:
	@echo "eds - tenant (inside the EdS monorepo)"
	@echo
	@echo "  init           terraform init"
	@echo "  plan           terraform plan"
	@echo "  apply          terraform apply    (AUTO=1 to skip approval)"
	@echo "  destroy        terraform destroy  (AUTO=1 to skip approval)"
	@echo "  state-backend  move state into the state store: writes backend.tf and"
	@echo "                 .backend.env from this tenant's outputs, then migrates"
	@echo "  validate       fmt check + validate"
	@echo
	@echo "Needs DEEVNET_API_TOKEN: the enrollment token on the first apply, then"
	@echo "  export DEEVNET_API_TOKEN=\$$(terraform output -raw api_token)"

require-token:
	@if [[ -z "$${DEEVNET_API_TOKEN:-}" ]]; then \
		echo "DEEVNET_API_TOKEN is not set." >&2; \
		echo "  First apply:  the enrollment token from the admission." >&2; \
		echo "  After that:   export DEEVNET_API_TOKEN=\$$(terraform output -raw api_token)" >&2; \
		exit 2; fi

# Once backend.tf exists, every command reads state from the store, and that
# needs the credentials in .backend.env.
check-backend:
	@if [[ -f backend.tf ]] && ! grep -q 'tenants/$(TENANT)/' backend.tf; then \
		echo "backend.tf does not name tenants/$(TENANT)/." >&2; exit 2; fi
	@if [[ -f backend.tf && ! -f .backend.env ]]; then \
		echo "backend.tf is here but .backend.env is not: copy .backend.env from the machine" >&2; \
		echo "that ran make state-backend (it holds the state store's credentials)." >&2; \
		exit 2; fi

init: check-backend
	terraform init

plan: check-backend require-token
	terraform plan

apply: check-backend require-token
	terraform apply $(TF_APPROVE)

destroy: check-backend require-token
	terraform destroy $(TF_APPROVE)

# The state store is offered, not mandated (ADR-0007). Its credentials are
# outputs of the tenant, so it is set up after the first apply: backend.tf
# (not secret - committed) and .backend.env (secret, gitignored), then the
# local state moves in.
state-backend: check-backend
	@if [[ -f backend.tf ]]; then echo "backend.tf already exists; state is already in the store." >&2; exit 2; fi
	terraform output -json state_backend | python3 scripts/state-backend.py
	set -a; . ./.backend.env; set +a; terraform init -migrate-state
	@echo "State is in the store. Commit backend.tf; keep .backend.env safe - it is the key to the state."

fmt:
	terraform fmt -recursive

validate:
	terraform fmt -check -recursive
	terraform validate
