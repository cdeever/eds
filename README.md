# EdS

EdS is an intelligent music experience platform that listens, identifies, plays,
and responds to music through sound, light, and personality.

This is a monorepo. Each project below is a distinct piece of that system.

| Project | Language | What it is |
|---|---|---|
| [`services/palette`](services/palette) | Python | Album cover art in, ranked colour swatches out |
| [`services/lightd`](services/lightd) | Go | Cover image in, MQTT scene out |
| [`firmware/lp-stand`](firmware/lp-stand) | C / ESP-IDF | The LP jacket stand: MQTT subscriber driving an addressable RGB strip |
| [`infra/deevnet-tenant-eds`](infra/deevnet-tenant-eds) | Terraform | The EdS tenant on the deevnet Proxmox substrate |

## The first vertical

A stand that props up the LP jacket while vinyl spins, lit by an RGB strip that
reacts to the record's cover art. It touches every layer — firmware, a
tenant-hosted service, the MQTT boundary between house and datacentre, and the
deevnet substrate — while depending on none of the still-unscoped parts
(listening, identification, playback, personality).

```
cover image ──▶ lightd ──▶ palette ──▶ lightd ──▶ mqtt01 ──▶ LP stand
                (Go)       (Python)               (VLAN 35)   (VLAN 30)
```

The broker is `mqtt01` on the substrate's IoT Backend segment, not a tenant VM.
MQTT clients always dial the broker, so whichever segment holds it must accept
inbound — and the tenant fabric has no inbound path by design (ADR-0001,
ADR-0003). IoT Backend is the segment already defined to accept exactly this.

## Substrate

EdS runs as a tenant on [deevnet](https://github.com/deevnet/deevnet-docs).
Unlike the reference pattern in ADR-0006, where the repository *is* the tenant,
the EdS tenant lives inside this monorepo as `infra/deevnet-tenant-eds` so the
application and the infrastructure it runs on stay in one place.
