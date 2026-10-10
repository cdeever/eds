# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

EdS is a monorepo for an intelligent music experience platform. Only the first
vertical is built: an LP jacket stand lit by an RGB strip that reacts to the
record's cover art.

```
cover image ──▶ lightd ──▶ palette ──▶ lightd ──▶ mqtt01 ──▶ LP stand
   (HTTP)        (Go)      (Python)              (VLAN 35)   (VLAN 30)
```

Four independent projects, each with its own `Makefile` and README. Nothing
builds the monorepo as a whole; work inside one project directory at a time.

| Path | Language | Build entry |
|---|---|---|
| `services/palette` | Python 3.11+ / FastAPI | `make venv test serve` |
| `services/lightd` | Go 1.25 | `make check build run` |
| `firmware/lp-stand` | C / ESP-IDF | `make test` (host), `make build flash` (device) |
| `infra/deevnet-tenant-eds` | Terraform | `make paths init plan apply` |

Each project's README carries the design rationale in depth. Read the relevant
one before changing behaviour — the choices there are argued, not incidental.

## Commands

### palette (`services/palette`)

```bash
make venv                      # .venv with editable install + dev extras
make test                      # pytest, no network
.venv/bin/pytest tests/test_extract.py::test_name -q   # a single test
make serve PORT=8731
make palette COVERS=~/covers   # terminal swatches, the tuning loop
make sheet COVERS=~/covers     # PNG contact sheet
```

### lightd (`services/lightd`)

```bash
make check                     # gofmt check + go vet + go test ./...
make test                      # unit tests only, no broker needed
make broker                    # dev mosquitto in podman on tcp://127.0.0.1:21883
make test-integration          # broker + `go test -race ./...`
go test ./internal/scene -run TestName -v               # a single test
make run                       # needs palette on :8731
make broker-watch              # tail every eds/# topic
```

Integration tests are gated on `LIGHTD_TEST_BROKER`; without it they skip.

### lp-stand (`firmware/lp-stand`)

```bash
make test                      # host tests, gcc only — no ESP-IDF, no device
. $IDF_PATH/export.sh          # required before any target below
make menuconfig build flash monitor PORT=/dev/ttyUSB0
make provision PORT=/dev/ttyUSB0   # creds partition from the tenant's Terraform outputs
make logs ARGS=-f              # the stands' events, from the tenant's log store
```

### deevnet-tenant-eds (`infra/deevnet-tenant-eds`)

```bash
make paths                     # where it expects the deevnet checkouts
make validate                  # terraform fmt -check + validate
make plan DEEVNET_ROOT=/path/to/checkouts
```

`plan`/`apply` need `TF_VAR_tsig_key_secret` exported (see that README) and the
deevnet repositories on disk. **This has never been applied**; three onboarding
steps are outstanding and listed in its README.

## Architecture: where each decision lives

The layering is the point, and violations of it are the main hazard when
editing. Each layer is deliberately ignorant of the one below.

- **`palette` knows nothing about music, MQTT or LEDs.** Image bytes in,
  perceptual colour data out. No role assignment (primary/accent), no LED
  correction. This boundary is what lets it be tuned against a folder of JPEGs
  with zero infrastructure. Do not add hardware awareness here.
- **`lightd` owns the hardware-facing colour decisions**: rejecting low-chroma
  swatches, chroma boost in OKLab, gamut mapping by walking chroma down (never
  clipping channels, which shifts hue). It maps swatches → scene.
- **Firmware owns gamma**, because the right curve is a property of *this*
  driver and diffuser. Keeping it out of the wire format means `mosquitto_sub`
  shows the colours actually intended.
- **The device renders effects locally, never receives frames.** Per-pixel
  streaming over WiFi turns latency into stutter and freezes mid-sweep on a
  broker outage.

OKLab math is implemented independently in all three (`palette/oklab.py`,
`lightd/internal/oklab`, firmware effects); changing conversion behaviour in one
place means checking the others.

### Contracts

Two versioned JSON contracts, both stamped `"v": 1`:

- `POST /v1/palette` → swatches with `rgb`, `hex`, `oklab`, `weight`, `chroma`,
  `lightness`, plus `background_dropped` and `fallback`. `GET /openapi.json` is
  how the contract stays honest across the Python/Go boundary.
- The **scene descriptor** (`{v, effect, speed, brightness, palette[]}`)
  published by lightd and parsed by firmware. `scene.ContractVersion` in Go and
  the firmware's validator must agree; a mismatched version is refused, not
  guessed at.

lightd's own HTTP surface: `GET /healthz`, `POST /v1/cover` (image bytes; the
interface the future album-cover resolver will call), `POST /v1/scene` (publish
a scene by hand — how the stand gets exercised with no image path), `GET
/v1/scene` (last sent).

### MQTT topics

| Topic | Direction | Retained |
|---|---|---|
| `eds/lightstand/<id>/scene` | lightd → device | yes |
| `eds/lightstand/<id>/status` | device → broker | yes (LWT) |
| `eds/lightstand/<id>/state` | device → broker | yes |
| `eds/lightd/status` | lightd → broker | yes (LWT) |
| `eds/log/<id>` | device → broker → log bridge | no |

`eds/log/<id>` is the substrate's shape, not EdS's: `<tenant>/log/<device>` is
the one topic under `log/` a device account may publish, and the substrate's
bridge carries it into the tenant's device log partition. The stand sends a few
JSON events there (boot, network, scenes, health), not its serial log; the
dashboard that reads them is `infra/deevnet-tenant-eds/dashboards/`.

**Retention on `scene` is load-bearing, not an optimisation.** A stand that
reboots pulls the current scene down instead of sitting dark mid-album. There is
an integration test that publishes *then* connects a subscriber specifically to
protect this.

### Network placement

The broker `mqtt01` is on the substrate's IoT Backend segment (VLAN 35), **not**
a tenant VM, and the stand sits on IoT (VLAN 30). This is forced, not preferred:
MQTT clients always dial the broker, so its segment must accept inbound, and the
tenant fabric has no inbound path by design (ADR-0001, ADR-0003). Moving the
broker into the tenant is an ADR, not a config change.

## Conventions that matter

- **Configuration is all environment variables in lightd**, no config file — the
  broker credentials must never land on disk in the repository, and one binary
  must work under systemd or a container. Invalid values stop the daemon rather
  than lighting the room wrong quietly. Full table in `services/lightd/README.md`.
- **Firmware fails soft, deliberately.** A malformed scene is ignored and the
  previous one keeps rendering; an unknown effect name falls back to `solid`;
  WiFi never gives up retrying; the strip lights dim warm white before the
  network exists; a frame is skipped rather than stalled on lock contention.
  Preserve these when editing — each is a choice against a dark stand.
- **Host tests build with `-Wall -Wextra -Werror`.** Only `scene.c`,
  `effects.c`, `gamma.c` and `event_format.c` are covered; the ESP-IDF glue
  (`scene_json.c`, `led_strip_out.c`, `wifi.c`, `mqtt.c`, `event_log.c`,
  `lp_credentials.c`, `main.c`) builds with IDF 6.1 and runs on one board but
  has no host tests. Keep hardware-independent logic out of the
  ESP-IDF-dependent files so it stays testable.
- **Palette tests use synthetic sleeves** (`tests/conftest.py`) built to exhibit
  specific pathologies — two-tone, heavy black field, blown white field,
  monochrome — because real cover art cannot be committed. Add fixtures there
  rather than committing images.
- **k-means in palette uses a fixed seed.** A stable input must not repaint the
  room; do not introduce nondeterminism.
- **Terraform: state is never committed** (it lives in the substrate's store,
  ADR-0007), but `.terraform.lock.hcl` **is** — a module pinned by tag with
  floating providers is half a pin. The module is pinned to
  `tenant-module-v1.1.0`; moving tags needs an explicit `terraform init -upgrade`.

## Secrets

**The firmware's two secrets are NVS-only.** The WiFi key and the broker
password are read from the `creds` partition (`make provision`) and have no
Kconfig option, so they cannot be compiled in. Do not add one back, and never
inline credentials into `sdkconfig.defaults` or `lp_config.h`. The SSID, broker
URI and account name are not secret and keep Kconfig defaults.

`sdkconfig` (generated) and `**/certs/*.pem` stay gitignored. Terraform secrets
arrive as environment variables only; the tenant's `.backend.env` and
`*.auto.tfvars` are gitignored too.
