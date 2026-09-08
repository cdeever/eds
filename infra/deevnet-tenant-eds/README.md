# deevnet-tenant-eds

The EdS tenant, as code: its overlay network, its workload and its DNS records,
rebuildable from scratch against the substrate without a substrate commit
([ADR-0006](https://github.com/deevnet/deevnet-docs)).

| | |
|---|---|
| Index | 1 (to be allocated in the factory's `TENANTS.md`) |
| Subnet | `10.20.129.0/24`, gateway `10.20.129.1` |
| Zone | `eds.mobile.deevnet.net` |
| Service host | `10.20.129.10` — `palette` and `lightd` |
| Module | `tenant-module-v1.1.0` |

## Status: written, never applied

This has been `terraform validate`d against the real module at
`tenant-module-v1.1.0`, and its derived addressing checked against ADR-0002.
It has **not** been planned or applied, and three things must happen first:

1. **Allocate index 1** in `deevnet-tenant-factory`'s `TENANTS.md`. It is free —
   `tdemo` was destroyed on 2026-09-05 and released it — but the allocation is
   an act, not an assumption.
2. **Onboard the tenant**: egress (ADR-0003) and DNS zones plus a TSIG key
   (ADR-0004), both driven from `deevnet_tenants` in the inventory.
3. **Issue the fabric attachment**: `make tenant-attachment TENANT=$(pwd)` in
   the factory, which writes `fabric.auto.tfvars` here.

## This diverges from ADR-0006, on purpose

ADR-0006's pattern is that the repository *is* the tenant — a standalone
`deevnet-tenant-<name>` repository with the deevnet repositories as siblings.
This tenant instead lives two directories inside the EdS monorepo, so that the
application and the infrastructure it runs on stay in one place.

The practical cost is one assumption: the Makefile expects the deevnet
repositories three levels up rather than one. That is inspectable rather than
something to discover from a failure:

```bash
make paths        # says where it is looking, and whether each exists
make plan DEEVNET_ROOT=/path/to/checkouts   # if yours live elsewhere
```

Nothing else about the pattern changes. State still lives in the substrate's
store, the module is still pinned by tag, and the secrets still arrive as
environment variables.

## What the substrate issues, and what you author

| Issued | Arrives as | Re-issue with |
|---|---|---|
| Fabric attachment | `fabric.auto.tfvars` | `make tenant-attachment TENANT=$(pwd)` in the factory |
| TSIG key | `TF_VAR_tsig_key_secret` | read from the inventory vault |
| Tenant index | the value in `main.tf` | allocated in `TENANTS.md` |

## Running it

```bash
export TF_VAR_tsig_key_secret=$(ansible-vault view \
  ../../../ansible-inventory-deevnet/mobile/group_vars/all/vault.yml \
  | yq -r .vault_tenant_tsig_keys.eds)

make init      # fetches the tagged module - needs GitHub and your ssh-agent
make plan
make apply
```

Proxmox credentials are rendered from the inventory vault by the targets
themselves; nothing is stored here.

## Why the broker is not in this tenant

EdS's MQTT broker is `mqtt01` on the substrate's **IoT Backend** segment
(VLAN 35), not a VM in here — and that is forced rather than preferred.

MQTT clients always initiate the connection: the ESP32 in the LP stand dials
the broker, never the reverse. So whichever segment holds the broker must
accept **inbound**. The tenant fabric has no inbound path by design — ADR-0003
delivered egress only, and ADR-0001's central promise is that the core router
never learns tenant address space. Putting the broker on a tenant VM would mean
inventing that path, which is an ADR rather than a config change.

IoT Backend is already defined to *"accept inbound connections from IoT segment
(sensor data, MQTT publish)"*, with `mqtt01` named in the model as its typical
inhabitant. So the stand sits on IoT (VLAN 30) and talks to IoT Backend, and
`lightd` in this tenant connects **outbound** to the same broker.

That last hop needs **one new perimeter rule**: `Tenant → IoT Backend` is not
in the current allow matrix (tenants are granted Platform). It is the cheap
direction — outbound over the transit path that already works — but it is not
free, and it is not authored here.

## Site: built on mobile, designed for home

The site selectors (`site_octet`, `vrf_vni_base`, `vnet_vni_base`,
`dns_substrate`) are passed explicitly in `terraform.tfvars` rather than left
to the module's defaults, so relocating EdS to the home site is a change there
rather than an edit to `main.tf`.

That matters for this tenant in particular. ADR-0008 records that only the
mobile site has hosts, and `addressing.md` describes a "mobile co-located with
home" WAN mode — the mobile rack travels. A turntable does not. **The lights
stop when the rack leaves.** Accepted for now, because mobile is the only
fabric that exists.

## State and pins

State is **not** in this repository; it lives in the substrate's state store
(ADR-0007) under `tenants/eds/terraform.tfstate`, which also provides the
locking a repository cannot.

`.terraform.lock.hcl` **is** committed. A module pinned by tag with providers
left to float is half a pin, and the floating half is the dangerous one.

The module is pinned to `tenant-module-v1.1.0` rather than the `v1.0.0` tdemo
used; the only difference is a validation rejecting a `tenant_index` outside
1–63. `terraform init` vendors the module and neither `plan` nor `apply`
re-fetches it, so moving to a newer tag needs an explicit
`terraform init -upgrade` — a feature and a trap in equal measure.
