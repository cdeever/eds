# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

EdS is a monorepo for an intelligent music experience platform. The first
vertical is an LP jacket stand lit by an RGB strip that reacts to the
record's cover art. Beside it is a touch screen that shows the cover and can
pause, skip and set the volume (ADR-0020).

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
| `services/nowplaying` | Go 1.25 | `make check test-integration run` |
| `services/console` | Go 1.25 + a static page | `make check run feed` |
| `firmware/lp-stand` | C / ESP-IDF | `make test` (host), `make build flash` (device) |
| `firmware/cover-screen` | C / ESP-IDF | `make test pictures` (host), `make build flash` (device) |
| `infra/deevnet-tenant-eds` | Terraform | `make paths init plan apply` |
| `images` | Ansible / shell | `make -C deploy/relay image fetch` (builds on builder) |

Each project's README carries the design rationale in depth. Read the relevant
one before changing behaviour — the choices there are argued, not incidental.

## Decisions are recorded — read them first

Design decisions, changes and incidents are kept as records on the docs site,
so a settled question is found rather than argued again:

| Record | Where | Write one when |
|---|---|---|
| **ADR-NNNN** | `docs/content/docs/architecture/decisions/` | two approaches were seriously considered; before the code |
| **CHG-NNNN** | `docs/content/docs/changes/<year>/` | something running changes: a `terraform apply`, a deployment, a device flashed or reprovisioned, a credential renewed |
| **INC-NNNN** | `docs/content/docs/incidents/<year>/` | something failed that a person would notice |

- **Before proposing a design, read the decision index**
  (`docs/content/docs/architecture/decisions/_index.md`). If a record answers
  the question, that answer stands. To change it, write a new record that
  extends or supersedes the old one; never rewrite an accepted record.
- A record resting on something unproven is `Proposed`, and lists what would
  settle it. Put the evidence in the record when it arrives.
- Records move in the same commit as the work, and each needs a row in its
  index table.
- **A bare `ADR-0006` means an EdS record.** The substrate uses the same
  names: always write `Deevnet ADR-0012` for one of its.
- Rules and templates: `docs/content/docs/develop/records/`.

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

### nowplaying (`services/nowplaying`)

```bash
make check                     # gofmt check + go vet + go test ./...
make broker                    # dev mosquitto (native, not podman) on tcp://127.0.0.1:21883
make test-integration          # broker + `go test -race -count=1 ./...`
make run                       # nowplayd on the dev broker, under edsdev/
make broker-watch              # every edsdev/nowplaying topic; covers shown by size
```

`bin/npagent` runs the drivers `NP_DRIVERS` names (`itunes`, `spotify`,
`turntable`), each with its own broker connection; `pair` and `spotify-auth`
set the first two up once.
`make build-arm64` builds it for the agent Pi.

**The stand does not follow Spotify, on purpose** (ADR-0016). The Spotify
driver never requests artwork, and `NP_COVERLESS_SOURCES=spotify` in
`deploy/nowplayd.env` stops nowplayd publishing or looking up a cover for it.
Do not "fix" the missing cover. The Spotify authorization asks for read-only
scopes; do not widen them without a decision. The app's Client ID lives in
gitignored `deploy/relay/spotify.conf` and is never committed.

EdS's Pi images are its own (`images/`, ADR-0015): the substrate's base image
with an EdS playbook applied offline. Do not add EdS images to the substrate's
`deevnet-image-factory`. An image carries no secret and no hostname; those are
written to a flashed card by `deploy/relay` (`make card`). "The relay" is an
agent Pi given a second network leg, not a separate image. The Pi is reached
through builder as `a_autoprov` (`make -C deploy/relay status|pair|logs`);
`make push` updates a Pi in service, `make spotify` authorizes it for Spotify,
`make drivers DRIVERS=…` chooses which drivers it runs. **Spotify is turned
off there for now** (CHG-0011): the Pi runs `itunes` only. Everything it
needs is kept; `DRIVERS=itunes,spotify` turns it back on.

**The turntable driver listens, and asks a recogniser nobody promised**
(ADR-0021). It hears the amplifier's tape output through a sound card on the
agent Pi (`arecord`, no cgo) and reports only what it is sure of: silence is
`stopped`, music it cannot name is `playing` with no track. Its recogniser is
a program, `eds-recognize` in `images/agent/files`, which is Shazam's through
an unpublished endpoint and may stop working any day. Keep every trace of
Shazam in that file and its pinned requirements; the driver knows only "a WAV
in, a name out". `make -C deploy/relay turntable DEVICE=…` names the input.
**On, on the agent Pi, beside iTunes** (CHG-0013): the Pi runs
`itunes,turntable`.

**The record on the turntable is named by its owner's collection** (ADR-0022).
For sources in `NP_COLLECTION_SOURCES`, nowplayd asks the owner's Discogs
collection first (`internal/discogs`), and its answer replaces the album the
source named: a source that hears a track cannot know the record, and the
collection can. Do not extend this to a source that knows its album. The
token is `NP_DISCOGS_TOKEN`, kept by hand in gitignored `deploy/discogs.env`
and never committed or logged. Discogs' terms ask for "Data provided by
Discogs." beside its pictures, which the console says, and that nothing of
theirs is kept longer than needed: one cover is held, in memory.

Integration tests are gated on `NP_TEST_BROKER`. This Makefile runs on macOS
(make 3.81, `/bin/bash`); the lightd and palette ones hardcode `/usr/bin/bash`
and `.ONESHELL` and do not.

### lp-stand (`firmware/lp-stand`)

```bash
make test                      # host tests, gcc only — no ESP-IDF, no device
. $IDF_PATH/export.sh          # required before any target below
make menuconfig build flash monitor PORT=/dev/ttyUSB0
make provision PORT=/dev/ttyUSB0   # creds partition from the tenant's Terraform outputs
make logs ARGS=-f              # the stands' events, from the tenant's log store
```

### cover-screen (`firmware/cover-screen`)

```bash
make test                      # host tests, gcc only
make pictures                  # each screen drawn to test/host/pictures/*.bmp
. ~/.espressif/tools/activate_idf_v6.1.sh
make build backup flash provision monitor   # PORT defaults to /dev/cu.usbserial-2120
```

An ESP32-2432S028R: 320x240 ILI9341, XPT2046 touch, no external memory. Its
Makefile runs under macOS make 3.81. **Nothing in it holds a frame**: the
cover is decoded a row of blocks at a time and the buttons are painted into
each band as it passes (`app/ui/draw.h`). Anything new on the screen must be
drawable that way. `make pictures` is how its look is checked without a
board. Its credentials, WiFi, event log and partition table are copies of
the stand's; change one, change the other.

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

lightd's own HTTP surface: `GET /healthz`, `POST /v1/cover` (image bytes),
`POST /v1/scene` (publish a scene by hand — how the stand gets exercised with
no image path), `GET /v1/scene` (last sent).

The stand follows what is playing: the agent Pi reports iTunes to the broker,
`nowplayd` on the workload publishes the current track and its cover, and
`lightd` lights the stand from the cover. `make -C deploy deploy WHERE=builder`
deploys all four services (the Mac has no container tool; the Builder builds).

The console (`services/console`, ADR-0018) is the page a person looks at:
`https://console.eds.mobile.deevnet.net/`. It subscribes to the bus and serves
what it heard. **It only looks**: its broker account reads and publishes only
its own presence, and it answers only `GET`. Adding a control is a decision to
record first, not a handler to add. Its subscriptions (`view.Topics`) and the
account in `main.tf` are one list kept in two places; the broker refuses a
mismatch silently. `internal/web/static/effects.js` is a port of the
firmware's `effects.c` - change one, change the other. A console test holds
the port to pixels printed from the firmware's own code.

The console's certificate is from the site's Tenant Service CA (Deevnet
ADR-0037), declared in the tenant's Terraform as `deevnet_certificate.console`.
**Its key is made there, lives in the tenant's state and in gitignored
`deploy/tls/`, and must never be committed or sent anywhere but the
workload.** It expires 2027-10-11 and nothing renews it but a person: `make
plan` shows the renewal from sixty days before; apply, then `make -C deploy
units`. Replacing the key does not revoke the old certificate.

lightd has a second input for covers: with `LIGHTD_COVER_TOPIC` set it
subscribes to that topic and runs each image down the same path as
`POST /v1/cover` (ADR-0009). It still sees only image bytes. Do not teach it
about tracks or albums; that knowledge stays on the other side of the topic.

A few albums have a look of their own (ADR-0019). `nowplayd` holds the list
(`internal/looks`) and publishes a look's **name** with the hash of the cover
it belongs to; `lightd` holds what the name looks like
(`internal/scene/looks.go`) and sends it as an ordinary scene. Add an album on
the music side and a preset on the light side; never an album name in lightd,
never a colour in nowplayd.

**Playback can be controlled, by one route** (ADR-0020). A controller
publishes `{"v":1,"command":"next"}` to `nowplaying/command`; `nowplayd`
passes it to the current source's `nowplaying/source/<id>/command`; the
iTunes driver carries it out over its pairing. The commands are `play_pause`,
`next`, `previous`, `volume_up`, `volume_down`, and adding one is a decision.
**A command is never retained and a retained one is ignored**; the screen
sends at QoS 0 and never queues one. Spotify takes none. A controller never
names a source, and the console still only looks.

`nowplayd` also publishes each cover small (`nowplaying/current/art/small`:
baseline JPEG, 240 px, at most 32 KB; empty when it could not be made), for
subscribers that cannot decode the full one. It knows nothing about any
screen; do not teach it.

The stand has 18 LEDs: `LP_LED_COUNT` in the firmware, `CONSOLE_STRIP_LEDS`
on the console, which is told and cannot find out.

### MQTT topics

| Topic | Direction | Retained |
|---|---|---|
| `eds/lightstand/<id>/scene` | lightd → device | yes |
| `eds/lightstand/<id>/status` | device → broker | yes (LWT) |
| `eds/lightstand/<id>/state` | device → broker | yes |
| `eds/lightd/status` | lightd → broker | yes (LWT) |
| `eds/nowplaying/current/look` | nowplayd → lightd | yes |
| `eds/nowplaying/current/art/small` | nowplayd → cover screen | yes |
| `eds/nowplaying/command` | cover screen → nowplayd | **never** |
| `eds/nowplaying/source/<id>/command` | nowplayd → driver | **never** |
| `eds/screen/<id>/status` | device → broker | yes (LWT) |
| `eds/console/status` | console → broker | yes (LWT) |
| `eds/log/<id>` | device or agent Pi → broker → log bridge | no |

`eds/log/<id>` is the substrate's shape, not EdS's: `<tenant>/log/<device>` is
the one topic under `log/` a device account may publish, and the substrate's
bridge carries it into the tenant's device log partition. The stand sends a few
JSON events there (boot, network, scenes, health), not its serial log; the
dashboard that reads them is `infra/deevnet-tenant-eds/dashboards/`. The agent
Pi is a registered device too (`np-relay`, ADR-0017) and reports the same way:
in `npagent`, a log line with an `event` attribute is also published there.
Workloads (`lightd`, `nowplayd`) are not devices and cannot use this route.

**Retention on `scene` is load-bearing, not an optimisation.** A stand that
reboots pulls the current scene down instead of sitting dark mid-album. There is
an integration test that publishes *then* connects a subscriber specifically to
protect this.

### Network placement

The broker `mqtt01` is on the substrate's IoT Backend segment (VLAN 35), **not**
a tenant VM, and the stand sits on IoT (VLAN 30). This is forced, not preferred:
MQTT clients always dial the broker, so its segment must accept inbound, and the
tenant fabric has no inbound path by design (Deevnet ADR-0001, Deevnet ADR-0003). Moving the
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
  Deevnet ADR-0007), but `.terraform.lock.hcl` **is** — a module pinned by tag with
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
