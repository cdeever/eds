# deevnet-tenant-eds

The EdS tenant, as code: its network, its workload and its DNS records, built
through the Deevnet API and rebuildable without a substrate commit
([ADR-0015][adr15]).

| | |
|---|---|
| Zone | `eds.mobile.deevnet.net` |
| Services | `palette` and `lightd`, on one workload |
| Provider | `deevnet/deevnet` |

[adr15]: https://deevnet.github.io/deevnet-docs/docs/architecture/decisions/0015-tenant-onboarding-through-api/
[adr6]: https://deevnet.github.io/deevnet-docs/docs/architecture/decisions/0006-tenant-code-boundary/
[adr7]: https://deevnet.github.io/deevnet-docs/docs/architecture/decisions/0007-terraform-state-custody/
[adr4]: https://deevnet.github.io/deevnet-docs/docs/architecture/decisions/0004-tenant-dns-publication/
[adr12]: https://deevnet.github.io/deevnet-docs/docs/architecture/decisions/0012-iot-platform-api/

## What EdS holds

One credential: its Deevnet API token. No index, no Proxmox token, no vault
access. The API issues the index, the network numbering, the DNS zone and key,
the state-store credential and each workload's address, and they land in this
tenant's Terraform state, which is their authoritative copy.

## Status: written, never applied

The configuration validates against the provider. It has **not** been applied,
and cannot be until the API is deployed (CHG-0010). When it is:

1. The operator admits `eds` and sends back a single-use enrollment token, the
   API's address and its CA certificate.
2. `export DEEVNET_API_TOKEN=<enrollment token>` and `make init && make apply`.
   That spends the token and returns EdS's own.
3. `export DEEVNET_API_TOKEN=$(terraform output -raw api_token)` from then on.
4. `make state-backend`, then `terraform init -migrate-state`, to keep state in
   the substrate's store ([ADR-0007][adr7]) - or leave it out and keep custody
   here.

**Its zones and its state-store user already exist**, created by CHG-0008 from
the inventory. The API adopts them rather than recreating them. The **TSIG
secret is replaced** by the one the API issues, and this state becomes its
authoritative copy - nothing held the old one except the substrate vault, and
EdS has never applied.

## Publishing names

The API publishes `services.eds.mobile.deevnet.net` and the two service names
beside it. EdS's TSIG key is still issued, so anything it would rather publish
itself over RFC 2136 still works ([ADR-0004][adr4]):
`terraform output -json dns_publication`.

## This diverges from ADR-0006, on purpose

[ADR-0006][adr6]'s pattern is that the repository *is* the tenant — a standalone
`deevnet-tenant-<name>` repository. This tenant instead lives two directories
inside the EdS monorepo, so that the application and the infrastructure it runs
on stay in one place.

That used to cost something: the Makefile had to find the deevnet repositories
to render credentials. It no longer does. A tenant reaches one API with one
token, so this directory is self-contained wherever it sits.

## Why the broker is not in this tenant

EdS's MQTT broker belongs to the substrate, in the device messaging VM
`dv02msg001v01` on the **IoT Backend** segment (VLAN 35), not in a VM here.
That is forced rather than preferred. The VM exists, but the broker (VerneMQ,
ADR-0012 §8) isn't built yet.

MQTT clients always initiate the connection: the ESP32 in the LP stand dials
the broker, never the reverse. So whichever segment holds the broker must
accept **inbound**. The tenant fabric has no inbound path by design — ADR-0003
delivered egress only, and ADR-0001's central promise is that the core router
never learns tenant address space. Putting the broker on a tenant VM would mean
inventing that path, which is an ADR rather than a config change.

IoT Backend is already defined to *"accept inbound connections from IoT segment
(sensor data, MQTT publish)"*. So the stand sits on IoT (VLAN 30) and talks to IoT Backend, and
`lightd` in this tenant connects **outbound** to the same broker.

That last hop needs its own perimeter rule, `tenant_transit -> iot_backend`,
because tenants are otherwise granted Platform only.
- **Declared:** in the inventory's `firewall.yml`, added with this tenant's
  onboarding.
- **Not enforced:** the core router's zone policy (CHG-0007) hasn't been
  applied yet, and the router passes all traffic until it is.
- **Where lightd's credentials will come from:** a broker account issued
  through the Deevnet API (ADR-0012 §3), not the substrate vault.

## Site: built on mobile, designed for home

The site is no longer named here at all. Each site runs its own API
([ADR-0015][adr15] §8), so relocating EdS means pointing `DEEVNET_API_ENDPOINT`
at the home site's API and being admitted there; its index and addressing come
from that site.

That matters for this tenant in particular. ADR-0008 records that only the
mobile site has hosts, and `addressing.md` describes a "mobile co-located with
home" WAN mode — the mobile rack travels. A turntable does not. **The lights
stop when the rack leaves.** Accepted for now, because mobile is the only
fabric that exists.

## Until the provider is published

The provider is not in the public registry yet, so `terraform init` needs it
locally:

```bash
git clone git@github.com:deevnet/terraform-provider-deevnet.git
cd terraform-provider-deevnet && make build
D=~/.terraform.d/plugins/registry.terraform.io/deevnet/deevnet/0.1.0/linux_amd64
mkdir -p $D && cp terraform-provider-deevnet $D/
```

The site's provider mirror replaces this ([ADR-0012][adr12] §7).

## State

State is **not** committed here. It holds every credential the substrate issued
EdS ([ADR-0015][adr15] §4), so it belongs in the substrate's state store under
`tenants/eds/terraform.tfstate`, which also gives the locking a repository
cannot — or somewhere with the same care, if EdS keeps its own custody
([ADR-0007][adr7]).

`.terraform.lock.hcl` will be committed once the provider is published and the
site mirror serves it; until then it would pin a local build.
