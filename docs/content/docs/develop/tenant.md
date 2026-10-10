---
title: "Tenant: deevnet-tenant-eds"
weight: 3
---

# Tenant: deevnet-tenant-eds

The EdS tenant as code: its overlay network, its workload and its DNS records,
rebuildable from scratch against the substrate without a substrate commit
(Deevnet ADR-0006).

| | |
|---|---|
| Index | 1 (to be allocated in the factory's `TENANTS.md`) |
| Subnet | `10.20.129.0/24`, gateway `10.20.129.1` |
| Zone | `eds.mobile.deevnet.net` |
| Service host | `10.20.129.10` — `palette` and `lightd` |
| Module | `tenant-module-v1.1.0` |

## Status: written, never applied

`terraform validate`d against the real module, with derived addressing checked
against Deevnet ADR-0002. It has **not** been planned or applied, and three things must
happen first:

1. **Allocate index 1** in `deevnet-tenant-factory`'s `TENANTS.md`.
2. **Onboard the tenant**: egress (Deevnet ADR-0003) and DNS zones plus a TSIG key
   (Deevnet ADR-0004).
3. **Issue the fabric attachment**: `make tenant-attachment TENANT=$(pwd)` in
   the factory, which writes `fabric.auto.tfvars`.

## Divergence from Deevnet ADR-0006

Deevnet ADR-0006's pattern is that the repository *is* the tenant — a standalone
`deevnet-tenant-<name>` repository with the deevnet repositories as siblings.
This tenant instead lives two directories inside the EdS monorepo, so that the
application and the infrastructure it runs on stay in one place.

The practical cost is one assumption: the Makefile expects the deevnet
repositories three levels up rather than one. That is inspectable rather than
something to discover from a failure:

```bash
make paths                                  # where it is looking, and whether each exists
make plan DEEVNET_ROOT=/path/to/checkouts   # if yours live elsewhere
```

## State and pins

State is **not** in this repository; it lives in the substrate's state store
(Deevnet ADR-0007) under `tenants/eds/terraform.tfstate`, which also provides the
locking a repository cannot.

`.terraform.lock.hcl` **is** committed. A module pinned by tag with providers
left to float is half a pin, and the floating half is the dangerous one.

## The one outstanding perimeter rule

`lightd` connects outbound from the tenant to `mqtt01` on IoT Backend.
`Tenant → IoT Backend` is not in the current allow matrix — tenants are granted
Platform. It is the cheap direction, over the transit path that already works,
but it is not free and it is not authored here.

## Site: built on mobile, designed for home

Site selectors are passed explicitly in `terraform.tfvars` rather than left to
the module's defaults, so relocating EdS to the home site is a change there
rather than an edit to `main.tf`.

That matters for this tenant in particular. Only the mobile site has hosts, and
the mobile rack travels. A turntable does not. **The lights stop when the rack
leaves** — accepted for now, because mobile is the only fabric that exists.
