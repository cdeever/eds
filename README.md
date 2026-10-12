# EdS

EdS is where the old analog world meets the convenience of the new one. A
record plays as it always has, analog from the stylus to the speakers, and
EdS works beside that path, never in it: a stand that lights the room from
the cover art, a small touch screen that shows the sleeve and controls what
plays, and a system that knows what is playing without being told.

The product, in plain words, is on the
[docs site](https://cdeever.github.io/eds/).

This is a monorepo. Each project below is a distinct piece of that system.

| Project | Language | What it is |
|---|---|---|
| [`services/palette`](services/palette) | Python | Album cover art in, ranked color swatches out |
| [`services/lightd`](services/lightd) | Go | Cover image in, MQTT scene out |
| [`services/nowplaying`](services/nowplaying) | Go | What is playing, on the bus: drivers per player, and the one current track with its cover |
| [`services/console`](services/console) | Go, and a static page | One page that shows what EdS is doing; it only looks |
| [`firmware/lp-stand`](firmware/lp-stand) | C / ESP-IDF | The LP jacket stand: MQTT subscriber driving an addressable RGB strip |
| [`firmware/cover-screen`](firmware/cover-screen) | C / ESP-IDF | The touch screen: the sleeve, and five buttons |
| [`firmware/power-beacon`](firmware/power-beacon) | C / ESP-IDF | A board on the turntable's switched outlet that says only that it has power |
| [`infra/deevnet-tenant-eds`](infra/deevnet-tenant-eds) | Terraform | The EdS tenant on the deevnet Proxmox substrate |
| [`images`](images) | Ansible, shell | EdS's Raspberry Pi images, built on the substrate's base; the first is the now-playing agent Pi |
| [`deploy`](deploy) | Make, systemd | The four services as containers on the tenant's workload; the agent Pi's settings |
| [`docs`](docs) | Hugo | The developer and user guide, published to [cdeever.github.io/eds](https://cdeever.github.io/eds/) |

## The first vertical

A stand that props up the LP jacket while vinyl spins, lit by an RGB strip that
reacts to the record's cover art. It touches every layer — firmware, a
tenant-hosted service, the MQTT boundary between house and datacenter, and the
deevnet substrate — while depending on none of the still-unscoped parts
(listening, identification, playback, personality).

```
cover image ──▶ lightd ──▶ palette ──▶ lightd ──▶  mqtt  ──▶ LP stand
                (Go)       (Python)               (VLAN 35)   (VLAN 30)
```

The broker is `mqtt.mobile.deevnet.net` on the substrate's IoT Backend segment,
not a tenant VM.
MQTT clients always dial the broker, so whichever segment holds it must accept
inbound — and the tenant fabric has no inbound path by design (Deevnet ADR-0001,
Deevnet ADR-0003). IoT Backend is the segment already defined to accept exactly this.

## Substrate

EdS runs as a tenant on [deevnet](https://github.com/deevnet/deevnet-docs).
Unlike the reference pattern in Deevnet ADR-0006, where the repository *is* the tenant,
the EdS tenant lives inside this monorepo as `infra/deevnet-tenant-eds` so the
application and the infrastructure it runs on stay in one place.
