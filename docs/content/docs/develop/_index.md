---
title: "Developer Guide"
weight: 30
---

# Developer Guide

Building and changing EdS. Each project has its own `Makefile` and README;
nothing builds the monorepo as a whole, and you work inside one project
directory at a time.

| Path | Language | Entry point |
|---|---|---|
| `services/palette` | Python 3.11+ / FastAPI | `make venv test serve` |
| `services/lightd` | Go 1.25 | `make check build run` |
| `services/nowplaying` | Go 1.25 | `make check test-integration run` |
| `firmware/lp-stand` | C / ESP-IDF 6.x | `make test`, `make build flash` |
| `infra/deevnet-tenant-eds` | Terraform | `make paths init plan apply` |
| `docs` | Hugo | `make server` |
