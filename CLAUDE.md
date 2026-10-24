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

Independent projects, each with its own `Makefile` and README. Nothing
builds the monorepo as a whole; work inside one project directory at a time.

| Path | Language | Build entry |
|---|---|---|
| `services/palette` | Python 3.11+ / FastAPI | `make venv test serve` |
| `services/lightd` | Go 1.25 | `make check build run` |
| `services/nowplaying` | Go 1.25 | `make check test-integration run` |
| `services/console` | Go 1.25 + a static page | `make check run feed` |
| `services/scribe` | Python 3.11+ / FastAPI / faster-whisper | `make venv PYTHON=python3.12 test serve` |
| `firmware/lp-stand` | C / ESP-IDF | `make test` (host), `make build flash` (device) |
| `firmware/cover-screen` | C / ESP-IDF | `make test pictures` (host), `make build flash` (device) |
| `firmware/power-beacon` | C / ESP-IDF | `make test` (host), `make build flash` (device) |
| `firmware/ears` | C / ESP-IDF (ESP32-S3) | `make test` (host), `make build flash provision` (device) |
| `tools/ota` | Go 1.25 | `make check test-integration`; run by `make ota` in a firmware |
| `infra/deevnet-tenant-eds` | Terraform | `make paths init plan apply` |
| `images` | Ansible / shell | `make -C deploy/relay image fetch`, `make -C deploy/backend image fetch` (build on builder) |

Each project's README carries the design rationale in depth. Read the relevant
one before changing behavior — the choices there are argued, not incidental.

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
- **A bare `ADR-0006` means an EdS record.** Deevnet IoTaaS uses the same
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

**Spotify is shown as iTunes is, and the stand does not follow it**
(ADR-0031). The driver takes the album's picture from the answer it already
asks for, once for each album and without the account's token; `spotify` is
in `NP_DARK_SOURCES` beside `itunes`, so the screens show its cover and the
stand stays dark, but for a look that is for any source (ADR-0030). It was
coverless until 2026-10-13 (ADR-0016); `NP_COVERLESS_SOURCES` remains, with
no source in it. The Spotify authorization asks to read playback and to
change it (`user-modify-playback-state`, ADR-0032) and for nothing else; do
not widen it further without a decision. The app's Client ID lives in
gitignored `deploy/relay/spotify.conf` and is never committed.

EdS's Pi images are its own (`images/`, ADR-0015): Deevnet IoTaaS's base image
with an EdS playbook applied offline. Do not add EdS images to Deevnet IoTaaS's
`deevnet-image-factory`. An image carries no secret and no hostname; those are
written to a flashed card by `deploy/relay` (`make card`). "The relay" is an
agent Pi given a second network leg, not a separate image. The Pi is reached
through builder as `a_autoprov` (`make -C deploy/relay status|pair|logs`);
`make push` updates a Pi in service, `make spotify` authorizes it for Spotify,
`make drivers DRIVERS=…` chooses which drivers it runs.

**The whole back end can also be one Pi, and that is not proven yet**
(ADR-0033, `Proposed`). `images/backend` is Deevnet IoTaaS's published
take-home image, `pi-backend`, with EdS's four services run natively and the
agent added; `deploy/backend` builds it, writes a card and looks after the Pi,
which is reached directly as its owner's user and not through builder. It is
beside the tenant: nothing there, on the workload or on the relay Pi changes
for it, and the devices stay with the tenant until they are provisioned
again. A backend card makes its own secrets on first boot (`deevnet-kit`,
then `eds-backend-setup`); none comes from the tenant. **Its broker accounts
are `images/backend/files/accounts.conf`, a copy of the tenant's in
`main.tf`**: change the Terraform first, then the list, and
`make -C deploy/backend check` holds them together. `images/agent` is not
edited for the backend's sake until it is proven: the backend's playbook has
its own copy of the agent's tasks and reads the agent's files from where they
are.

**The turntable driver listens, and asks a recognizer nobody promised**
(ADR-0021). It hears the amplifier's tape output through a sound card on the
agent Pi (`arecord`, no cgo) and reports only what it is sure of: silence is
`stopped`, music it cannot name is `playing` with no track. A second of
quiet with music after it is a gap between two tracks: the listening starts
over, and the one clip that follows is believed alone, as the first after the
needle comes down is (ADR-0026); no other single answer replaces a track. Its recognizer is
a program, `eds-recognize` in `images/agent/files`, which is Shazam's through
an unpublished endpoint and may stop working any day. Keep every trace of
Shazam in that file and its pinned requirements; the driver knows only "a WAV
in, a name out". `make -C deploy/relay turntable DEVICE=…` names the input.
**On, on the agent Pi, beside iTunes** (CHG-0013): the Pi runs
`itunes,turntable`. Spotify is off again, for the calls it makes (CHG-0024);
`make -C deploy/relay drivers DRIVERS=itunes,spotify,turntable` undoes it.

**The record on the turntable is named by its owner's collection** (ADR-0022).
For sources in `NP_COLLECTION_SOURCES`, nowplayd asks the owner's Discogs
collection first (`internal/discogs`), and its answer replaces the album the
source named: a source that hears a track cannot know the record, and the
collection can. Do not extend this to a source that knows its album. The
token is `NP_DISCOGS_TOKEN`, kept by hand in gitignored `deploy/discogs.env`
and never committed or logged. Discogs' terms ask for "Data provided by
Discogs." beside its pictures, which the console says, and that nothing of
theirs is kept longer than needed: one cover is held, in memory.

**What is asked of other people's services is counted** (ADR-0027). Every
HTTP client in `services/nowplaying` that reaches an outside service is made
by `calls.Meter.Client`, and the recognizer is wrapped the same way; once a
minute each service asked anything gets one `api.calls` line. `npagent`
reports it as an event. `nowplayd` posts it to the tenant's log store itself
(`internal/logstore`, the workload partition), and that is all it sends
there: its log stays in the journal. A new client for an outside service is
made the same way, and its host named in `calls.Name`. The counter limits
nothing; do not make it. The dashboard is **Outside Services**, beside the
stand's. The ingest token is in `kit.env` and is never logged.

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

**It sleeps when nothing plays** (ADR-0036): five minutes after `current`
last said `playing` with a `source`, the panel goes dark; playing, a source
named, a touch or a lost broker lights it. A missing `source` is nowplayd's
arbiter saying every source is offline or switched off; do not give the
screen the presence topics.

### power-beacon (`firmware/power-beacon`)

```bash
make test                      # host tests for the event lines, gcc only
. ~/.espressif/tools/activate_idf_v6.1.sh
make build flash provision monitor   # PORT defaults to /dev/cu.usbserial-2120
```

A plain ESP32 with nothing attached (an ESP32-S2 until CHG-0027), on the
outlet that is switched with the turntable (ADR-0023). **Its connection is its message**: `online`, retained,
when it connects, and a last will of `offline`. It knows nothing of
turntables; its name and `NP_SOURCE_POWER` say what it speaks for. Its
keepalive is ten seconds, on purpose: that is how soon the will is sent.
`flash` and `provision` check the board's MAC first (`MAC=` in its
Makefile): the stand and the screen are ESP32s too, on the same ports. The
old S2 was erased; never provision two boards as one beacon. Its credentials, WiFi, event log and partition table
are copies of the stand's too.

### ears (`firmware/ears`)

```bash
make test                      # host tests for the sound and the events, gcc only
. ~/.espressif/tools/activate_idf_v6.1.sh
make build flash provision monitor   # PORT defaults to /dev/cu.usbmodem2101
```

The reSpeaker Lite's XIAO ESP32-S3 (ADR-0043). The XMOS chip beside it
sends one cleaned-up voice over I2S and drives the clock; the firmware
measures the rate and prints both channels' levels, and while the XIAO's
BOOT button is held keeps what is said as 16 kHz mono, sent on release as one
WAV to `voice/respeaker-01/utterance` (ADR-0042). `app/audio/pcm.c` is the
host-tested half. Its credentials, WiFi, event log and provisioning are
copies of the screen's. `flash` and `provision` check its MAC.

### Updates over the air (`tools/ota`, all three firmwares)

```bash
make build ota                 # in a firmware directory: this build, to its device
```

**A build reaches a device through the broker, and a build that cannot
reach the broker is undone** (ADR-0035). The devices can reach nothing of
EdS's but the broker, so `eds-ota` runs on the services workload with a
broker account of its own, `ota`, offers the build on the device's `…/ota`
topic and sends what the device asks for on `…/ota/chunk`, 8 KB at a time.
The device writes it to its spare slot (`app/ota` decides which bytes, host
tested; `network/ota` writes them) and restarts into it with a clean
disconnect, so the beacon's restart is not a will. **ESP-IDF's rollback is
on in every bootloader**: a new build is kept once it has connected to the
broker, and one that has not within two minutes boots the old one again.
Do not weaken that test: reaching the broker is exactly what the next
update needs. The bootloader is not updated over the air, so turning
rollback on took one flash by cable; `network/ota` refuses to build with an
`sdkconfig` that has it off. The `ota` account names each device, and a new
device is added to it when it can take an update. The images are not
signed; anyone with the `ota` password can run code on the devices. It is
EdS's alone: Deevnet IoTaaS only enforces the grants in `main.tf`.

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
  perceptual color data out. No role assignment (primary/accent), no LED
  correction. This boundary is what lets it be tuned against a folder of JPEGs
  with zero infrastructure. Do not add hardware awareness here.
- **`lightd` owns the hardware-facing color decisions**: rejecting low-chroma
  swatches, chroma boost in OKLab, gamut mapping by walking chroma down (never
  clipping channels, which shifts hue). It maps swatches → scene.
- **Firmware owns gamma**, because the right curve is a property of *this*
  driver and diffuser. Keeping it out of the wire format means `mosquitto_sub`
  shows the colors actually intended.
- **The device renders effects locally, never receives frames.** Per-pixel
  streaming over WiFi turns latency into stutter and freezes mid-sweep on a
  broker outage.

OKLab math is implemented twice, independently (`palette/oklab.py`,
`lightd/internal/oklab`); changing conversion behavior in one means changing
the other, and the reference values in lightd's `oklab_test.go`. The firmware
has none: it mixes the RGB it is sent.

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

The stand follows the record that is playing: the agent Pi reports the
turntable and iTunes to the broker, `nowplayd` on the workload publishes the
current track and its cover, and `lightd` lights the stand from the cover.
**The stand is lit for records only** (ADR-0028): `NP_DARK_SOURCES=itunes`
makes `nowplayd` publish iTunes' cover with the look `dark`, which `lightd`
draws as `scene.Dark`. The screen and the console still show that cover. Do
not "fix" the dark stand; lights for iTunes are to be their own thing. The
console can switch it (ADR-0039): the retained setting
`nowplaying/stand`, `{"v":1,"sources":"records"|"all"}`, which `nowplayd`
reads; `all` treats `NP_DARK_SOURCES` as empty until switched back, and
nothing, or anything unreadable, is `records`. `make -C deploy deploy WHERE=builder`
deploys the six services (the Mac has no container tool; the Builder builds).
**A deploy moves only what changed** (INC-0002): each image carries
`eds.source`, a hash of its sources; unchanged ones are neither built nor
sent, and changed ones go compressed at 5 MB/s (`deploy/load.sh`).
**scribe is two images**: `eds-scribe-base` (runtime, libraries, model),
sent when `Containerfile.base` or `pyproject.toml` change, and `eds-scribe`,
its 60 KB of code, built on the workload on that base
(`deploy/build-there.sh`). Never stream images to the workload by hand:
two gigabytes through the core router hung it.

The console (`services/console`, ADR-0018) is the page a person looks at:
`https://console.eds.mobile.deevnet.net/`. It subscribes to the bus and serves
what it heard. **It looks, and switches one thing** (ADR-0039): its broker
account reads, and publishes only its own presence and the stand's setting,
and its one route that is not `GET` is `POST /api/stand`, JSON from its own
origin, with no login. Adding another control is a decision to record first,
not a handler to add. Its subscriptions (`view.Topics`) and the
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
never a color in nowplayd. One look is a single track's and is shown from
any source, a dark one included (ADR-0030): the theme from *Knight Rider*,
`scanner`, matched on its title alone, drawn by the firmware's `scanner`
effect in red. It is the one exception to the stand being for records;
another is a decision, not a line added to `looks.AnySource`.

**Playback can be controlled, by one route** (ADR-0020). A controller
publishes `{"v":1,"command":"next"}` to `nowplaying/command`; `nowplayd`
passes it to the current source's `nowplaying/source/<id>/command`; the
iTunes driver carries it out over its pairing, and the Spotify driver through
Spotify's Web API, only for playback on a device that counts (ADR-0032). The commands are `play_pause`,
`next`, `previous`, `volume_up`, `volume_down`, and adding one is a decision.
**A command is never retained and a retained one is ignored**; the screen
sends at QoS 0 and never queues one. The turntable takes none. A controller never
names a source, and the console sends no command.
**`duck` and `unduck` turn the music down while a request is heard**
(ADR-0044): the device publishes `voice/<id>/woken`, voiced sends `duck` to
whatever is current, and `unduck` once it has answered; the iTunes driver
goes to a quarter and comes back on `unduck`, before a `play_album`, or by
itself after ten seconds. A volume press while ducked ends it where it is.
**The one command that is not a button is `play_album`** (ADR-0040): it
names an artist and an album, and `nowplayd` passes it to `NP_ALBUM_SOURCE`
(`itunes`) whatever is current. iTunes for Windows refuses this remote its
library (403 on `databases`), so the album is played by name and the driver
asks what plays afterward; a miss is a failed command, never a nearer guess.
For an album asked for by its place, voiced names up to three of the
artist's next albums in `instead`, and the driver plays the first the
library has (ADR-0045): "Genesis' first album" is the first of theirs the
owner has. A miss empties iTunes' queue, so missing them all stops the music.
**With the owner's library indexed, voiced chooses from it** (ADR-0046):
`cmd/library index` makes `library.json` from iTunes' XML (artist, album,
year, tracks, compilation; nothing else), `make -C deploy library XML=…`
copies it to `/var/lib/eds-voiced`, and `internal/library` finds the
nearest artist and album, so `play_album` names titles the library has.
**Neither the XML nor the index is ever committed**; without one, voiced
guesses as above.

**A request can be spoken** (ADR-0042). A device sends what followed its
wake word as one WAV on `voice/<id>/utterance`; `voiced`
(`services/nowplaying/cmd/voiced`, its own service and broker account) has
`scribe` write it down, reads it with `internal/voice`'s short grammar
("play X's first album", "play the Nth album by X", "play ALBUM by X"),
counts the artist's studio albums in MusicBrainz when a place is named
(ADR-0041, `resolve.Originals.Discography`), sends `play_album`, and says
what it made of it on `voice/<id>/heard`. **scribe knows nothing about
music**, as palette knows nothing about it: a WAV in, words out. Words that
are not a request are said back as not understood; nothing nearer is
guessed. `go run ./cmd/discography ARTIST [N]` checks a count by hand.

**A source whose equipment is switched on holds the stand** (ADR-0023). For
the sources `NP_SOURCE_POWER` names, `nowplayd` reads a beacon's status:
on, that source is current before playing, paused and stopped are compared;
off, it is not heard; never heard from, nothing changes. This is the
arbiter's one exception (`internal/arbiter`), and it is why a lifted needle
does not hand the room to a paused iTunes. Do not make the beacon publish a
source's own status topic: that is the driver's word about its sound card.

**Paused long enough is not playing** (ADR-0024). After `NP_IDLE_AFTER`
(15 s) of a paused winner, or of nobody reporting, `nowplayd` publishes
`current` as stopped with no track, still naming the source so play can be
pressed, and **empties** the cover topics. An emptied cover topic is the one
way `lightd` is told there is no cover. **It answers dark, and dim only
while something is switched on** (ADR-0028): before emptying the topics
`nowplayd` publishes a look that names no cover, `standby` when any
`NP_SOURCE_POWER` beacon is on and none otherwise, and `lightd` draws
`standby` as `scene.Idle` and anything
else as `scene.Dark`. `lightd` still knows no beacon; do not teach it one.
A cover merely not sent
still leaves the room as it was (ADR-0016); do not confuse the two. A source
that is stopped and held has its own wait, `NP_IDLE_AFTER_STOPPED` (1 s): the
turntable's driver has already heard twenty seconds of silence, and the
lights are on for music, so turning a record over dims the room (ADR-0026).

**`current` says when there are no controls** (ADR-0025):
`"no_controls": true` for the sources in `NP_NO_CONTROLS_SOURCES`, the
turntable. The cover screen draws no buttons then. It still knows no source;
do not teach the firmware the word "turntable".

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
| `eds/nowplaying/stand` | console → nowplayd, console | yes (a setting) |
| `eds/nowplaying/current/art/small` | nowplayd → cover screen | yes |
| `eds/nowplaying/command` | cover screen → nowplayd | **never** |
| `eds/nowplaying/source/<id>/command` | nowplayd → driver | **never** |
| `eds/voice/<id>/utterance` | device → voiced | **never** |
| `eds/voice/<id>/woken` | device → voiced | **never** |
| `eds/voice/<id>/heard` | voiced → device | **never** |
| `eds/voiced/status` | voiced → broker | yes (LWT) |
| `eds/screen/<id>/status` | device → broker | yes (LWT) |
| `eds/beacon/<id>/status` | device → nowplayd, console | yes (LWT) |
| `eds/console/status` | console → broker | yes (LWT) |
| `eds/log/<id>` | device or agent Pi → broker → log bridge | no |
| `eds/{lightstand,screen,beacon}/<id>/ota` | eds-ota → device | **never** |
| `eds/{lightstand,screen,beacon}/<id>/ota/chunk` | eds-ota → device | **never** |
| `eds/{lightstand,screen,beacon}/<id>/ota/state` | device → eds-ota | **never** |

`eds/log/<id>` is Deevnet IoTaaS's shape, not EdS's: `<tenant>/log/<device>` is
the one topic under `log/` a device account may publish, and Deevnet IoTaaS's
bridge carries it into the tenant's device log partition. The stand sends a few
JSON events there (boot, network, scenes, health), not its serial log; the
dashboard that reads them is `infra/deevnet-tenant-eds/dashboards/`. The agent
Pi is a registered device too (`np-relay`, ADR-0017) and reports the same way:
in `npagent`, a log line with an `event` attribute is also published there.
Workloads (`lightd`, `nowplayd`) are not devices and cannot use this route.

**Presence is heard, not remembered** (ADR-0034). Everything that publishes
a `…/status` topic says `online` again, retained, every thirty seconds; a
reader believes an online for ninety seconds after hearing it **live**. A
stored one (`Retained()` on delivery) is only a claim: the broker keeps it
across restarts after its sender is gone, which is how a stand unplugged
during a broker outage showed green for days. The console draws a claim
gray; `nowplayd` keeps one provisionally and lets it lapse. A new publisher
of presence repeats it, or readers will gray it out.

**Retention on `scene` is load-bearing, not an optimization.** A stand that
reboots pulls the current scene down instead of sitting dark mid-album. There is
an integration test that publishes *then* connects a subscriber specifically to
protect this.

### Network placement

The broker `mqtt01` is on Deevnet IoTaaS's IoT Backend segment (VLAN 35), **not**
a tenant VM, and the stand sits on IoT (VLAN 30). This is forced, not preferred:
MQTT clients always dial the broker, so its segment must accept inbound, and the
tenant fabric has no inbound path by design (Deevnet ADR-0001, Deevnet ADR-0003). Moving the
broker into the tenant is an ADR, not a config change.

## Conventions that matter

- **Write American English**, everywhere a person reads and in the code
  itself: the docs site, the READMEs, records, comments, log lines, the
  console's page, and the names of things. "Color", "catalog", "recognizer",
  "gray", "behavior", "center"; "around", not "round"; "parentheses", not
  "brackets". Its owner is American; the docs and then the code were swept
  on 2026-10-12. **A name that something outside its own source refers to
  keeps the spelling it has**: a setting (`NP_CATALOGUES`), a topic, an
  event name already in the log store, a page's URL
  (`0008-the-stand-reports-its-colours`), or text quoted from what a program
  once printed. Spelling is not a reason to break a reference.
- **The platform EdS runs on is "Deevnet IoTaaS"**, not "the substrate":
  renamed throughout on 2026-10-12. The page
  `0003-broker-is-the-substrates` keeps its URL, for the reason above.
- **Configuration is all environment variables in lightd**, no config file — the
  broker credentials must never land on disk in the repository, and one binary
  must work under systemd or a container. Invalid values stop the daemon rather
  than lighting the room wrong quietly. Full table in `services/lightd/README.md`.
- **Firmware fails soft, deliberately.** A malformed scene is ignored and the
  previous one keeps rendering; an unknown effect name falls back to `solid`;
  WiFi never gives up retrying; a frame is skipped rather than stalled on
  lock contention. Preserve these when editing — each is a choice against a
  stand that needs a person.
- **The stand is dark until it is sent a scene, and blinks when it cannot
  be** (ADR-0029): after twenty seconds without the broker its first LED
  blinks red, 1 for unprovisioned, 2 for no WiFi, 3 for no broker, 4 for a
  refused login. The logic is `app/status/status.c`, host tested. The counts
  are in the README for people to learn; do not renumber them, and do not
  light more than the one LED. Between dark and lit the strip opens from the
  middle and closes to it (`app/transition`); that is the stand's own and is
  never sent, and the console does not draw it.
- **Host tests build with `-Wall -Wextra -Werror`.** Only `scene.c`,
  `effects.c`, `status.c`, `transition.c`, `gamma.c`, `event_format.c` and
  `ota_receive.c` are covered; the ESP-IDF glue
  (`scene_json.c`, `led_strip_out.c`, `wifi.c`, `mqtt.c`, `ota.c`,
  `event_log.c`, `lp_credentials.c`, `main.c`) builds with IDF 6.1 and runs on one board but
  has no host tests. Keep hardware-independent logic out of the
  ESP-IDF-dependent files so it stays testable.
- **Palette tests use synthetic sleeves** (`tests/conftest.py`) built to exhibit
  specific pathologies — two-tone, heavy black field, blown white field,
  monochrome — because real cover art cannot be committed. Add fixtures there
  rather than committing images.
- **k-means in palette uses a fixed seed.** A stable input must not repaint the
  room; do not introduce nondeterminism.
- **Terraform: state is never committed** (it lives in Deevnet IoTaaS's store,
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
