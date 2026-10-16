# lp-stand

Firmware for the LP jacket stand: a stand that props up the record sleeve while
vinyl spins, lit by an addressable RGB strip that reacts to the cover art.

ESP-IDF in C, following the conventions of
[`esp32-ma-bell-gateway`](https://github.com/cdeever/esp32-ma-bell-gateway) —
`main/app/`, `main/config/`, `main/hardware/`, `main/network/`, with a Makefile
wrapper over `idf.py`.

## Status

**The hardware-independent half is tested. The device half runs on one board.**

| | |
|---|---|
| `scene.c`, `effects.c`, `status.c`, `transition.c`, `gamma.c`, `event_format.c` | 59 tests, 6052 checks, `-Wall -Wextra -Werror` |
| `scene_json.c`, `led_strip_out.c`, `wifi.c`, `mqtt.c`, `event_log.c`, `lp_credentials.c`, `main.c` | built with ESP-IDF 6.1 and running on an ESP32-D0WD-V3: joins the IoT network, verifies the broker over TLS, takes its retained scene and reports its events. No host tests |

The split is deliberate rather than incidental. Scene validation, effect
rendering and gamma are where a bug is subtle and shows up only as *the light
looks wrong*, and event formatting is where one shows up as nothing at all, so
they are pure C with no ESP-IDF dependency and are tested with a compiler:

```bash
make test        # needs gcc and nothing else
```

The ESP-IDF glue is thin by design and needs a device to trust:

```bash
. $IDF_PATH/export.sh
make menuconfig  # stand id, strip length and pin
make build flash monitor PORT=/dev/ttyUSB0
make provision PORT=/dev/ttyUSB0   # what the EdS tenant issued this stand
```

`make provision` reads the tenant's Terraform outputs
([`infra/deevnet-tenant-eds`](../../infra/deevnet-tenant-eds)) and writes the
WiFi key, the broker address, the stand's own broker account and the Deevnet
Root CA into the `creds` partition. Nothing is typed and nothing is compiled
in, so the same image serves every stand and a re-rooted PKI costs a
reprovision rather than a rebuild. It refuses to write if the account was not
granted the topics this firmware uses, because the broker would not say so: an
ungranted publish is dropped silently. `make wifi-config` writes the same
partition by hand, for a stand that is not on Deevnet IoTaaS.

## How it works

```
 mqtt  ──▶ eds/lightstand/<id>/scene ──▶ parse ──▶ validate ──▶ render task ──▶ strip
                    (retained)                                    (60 fps)
```

The stand subscribes to a **scene descriptor** and animates it locally:

```json
{ "v": 1, "effect": "breathe", "speed": 0.4, "brightness": 0.8,
  "palette": [ {"rgb": [232, 77, 0], "weight": 0.56} ] }
```

**It never receives frames.** Streaming pixels over WiFi turns latency spikes
into visible stutter, and a broker outage would freeze an animation mid-sweep.
Rendering here means a stand whose network has gone away keeps doing something
sensible, and the payload stays small enough to be read by eye in
`mosquitto_sub`.

**The scene topic is retained**, which is why a stand that reboots mid-album
picks the record back up instead of waiting for the next one. Getting the
retained scene moments after subscribing is the normal case, not an edge one.

### Effects

`solid`, `breathe`, `sweep`, `morph`, `chase`, `scanner` — deliberately few, because
adding one means reflashing every stand.

`sweep` lays the palette out in proportion to the weights lightd sent, so a
color covering 60% of the sleeve covers 60% of the strip, and cross-fades at
the seams: hard edges on a diffused strip look like a fault in the diffuser.

`morph` and `chase` use the accents, the colors of a cover that are not its
dominant one. `morph` turns the whole strip from the dominant color to each
accent and back, giving each a turn that is half by its weight and half equal.
`chase` keeps the dominant color as the ground and sends each accent around
over it as a soft-edged patch, alternate ones in opposite directions.

`scanner` is one point of the dominant color going from end to end and back
in one cycle, with a fading tail, on a dark strip (ADR-0030). The eye is a
fifth of the strip wide, three or four LEDs of eighteen, for the same reason
`chase` has a least width; `SCANNER_EYE`, `SCANNER_TRAIL` and
`SCANNER_TRAIL_STEP` in `effects.c` are its shape.

The strip is 18 LEDs (`LP_LED_COUNT`), the number that fit across the front
of the stand. `chase` is sized for it: the smallest patch is about two LEDs
wide, because a soft-edged patch narrower than that does not glide from one
LED to the next. It dims to nearly nothing halfway and reads as a twinkle.

**Time is taken to the whole millisecond before it is divided.** A float
holds a millisecond count exactly for about four and a half hours. An
animation timed from `(float)t_ms` moves in steps that grow with uptime, and
after a week advances in jumps. There is a test that renders five weeks in.

An **unknown effect name falls back to `solid`** rather than being refused. A
publisher that learns a new effect before the stands do is expected; a lit stand
showing the right colors with the wrong animation beats a dark one.

### Gamma lives here

lightd deliberately does not gamma-correct. WS2812 PWM is linear in duty cycle
while perception is not, so an uncorrected fade rushes through the dark end and
then stalls — but the right curve is a property of *this* driver and diffuser.
Correcting at the output stage keeps it next to the hardware it corrects, and
keeps the wire format showing the colors actually intended.

### Failure behavior

Everything below is a deliberate choice, not an accident:

- **A malformed or unparseable scene is ignored**, and the previous one keeps
  rendering. A stand that goes dark on a bad payload is worse than one that
  ignores it.
- **A scene with a different contract version is refused**, not guessed at.
- **WiFi never gives up.** An unattended stand that stops retrying is one that
  needs a person to walk over and power-cycle it.
- **The strip is dark until a scene arrives**, and a stand that cannot get
  that far says so: see [Blink codes](#blink-codes).
- **A frame is skipped rather than stalled** if the render task cannot take the
  scene lock. A skipped frame is invisible; a stalled strip is not.
- **Last will on `status`**, so the room learns a stand died rather than merely
  went quiet.

## Coming on and going off

Sent a lit scene while dark, the strip opens from its middle to its ends in
1.4 seconds, behind an edge at least 0.6 bright; sent a dark one while lit,
it closes to the middle in 1.1 and goes out (ADR-0029). Nothing asks for
this: it is what the stand does between a scene with no brightness and one
with some. `LP_TRANSITION_ON_MS`, `LP_TRANSITION_OFF_MS` and
`LP_TRANSITION_FLARE` in `main/app/transition/transition.h` are the three
numbers to change if it should feel different. Host tested.

## Blink codes

A stand that has been unable to reach the broker for twenty seconds blinks
its **first LED red**
([ADR-0029](../../docs/content/docs/architecture/decisions/0029-the-stand-starts-dark-and-blinks-what-is-wrong.md)).
Count the blinks between the two-second pauses: the number is the step the
stand did not get past, so more blinks is nearer to working.

| Blinks | What is wrong | What to do |
|---|---|---|
| 1 | Nothing provisioned: the `creds` partition is empty | `make provision` |
| 2 | No WiFi: not on the network, or no address | is the access point up and in range? If it is, the key is wrong: `make provision` |
| 3 | No broker: on the network, and the broker is not answering | is the broker up, and reachable from the IoT segment? A CA that does not match the broker's certificate looks like this too |
| 4 | Refused: the broker answered and would not take the login | the account's password was replaced: `make provision` |
| none, and dark | Connected and sent nothing to show, or no power | the console says which: the stand's presence, and the scene it was sent |

The rest of the strip is left as it was: a stand that loses its network
during a record goes on showing the record, with the first LED blinking. The
blinking stops when the broker is back. `make monitor` says the same thing
in words, and why.

The logic is `main/app/status`, and is host tested. The counts are not to be
renumbered: people learn them.

## Topics

| Topic | Direction | Retained | |
|---|---|---|---|
| `eds/lightstand/<id>/scene` | broker → stand | yes | what to render |
| `eds/lightstand/<id>/status` | stand → broker | yes (LWT) | `online` / `offline` |
| `eds/lightstand/<id>/state` | stand → broker | yes | what it is rendering now |
| `eds/log/<id>` | stand → broker | no | its key events, for the tenant's log store |
| `eds/lightstand/<id>/ota` | `eds-ota` → stand | **never** | a build offered (ADR-0035) |
| `eds/lightstand/<id>/ota/chunk` | `eds-ota` → stand | **never** | the build, in pieces |
| `eds/lightstand/<id>/ota/state` | stand → `eds-ota` | **never** | its answers, and on every connection what it runs |

## Events

The stand reports the few things worth knowing once the USB cable is gone, as
one JSON object each on `eds/log/<id>`. Deevnet IoTaaS's log bridge carries them
into EdS's device log partition; the stand holds no log store credential and
has no route to the store.

```json
{"level":"info","event":"scene.applied","msg":"Showing breathe in 4 colours","uptime_ms":3525,"effect":"breathe","colors":4,"brightness":0.80}
```

| Event | When | Fields |
|---|---|---|
| `system.boot` | the stand starts | `reason`, `firmware`, `idf` |
| `wifi.connected` / `wifi.disconnected` | it joins or loses the network | `ssid`, `ip`, `rssi`, `attempts` / `reason` |
| `mqtt.connected` / `mqtt.disconnected` | it reaches or loses the broker | `ip`, `rssi` |
| `scene.applied` | a scene takes effect | `effect`, `colors`, `brightness` |
| `scene.rejected` | a scene is ignored | `bytes` |
| `stand.health` | every five minutes | `rssi`, `heap_free`, `heap_min` |

This is not the serial log, and deliberately: it is a handful of events a day,
not a stream. Numbers are fields rather than text because the platform has no
metrics service, and a number in a log line is how something gets graphed.

**The stand has no clock**, so the store dates each event when it arrives.
Events recorded while the broker was away wait in a queue of sixteen and arrive
together; `uptime_ms` gives their order. When the queue overflows the oldest
go, and the next event to get through carries a `dropped` count.

It fails soft like everything else here: recording an event never blocks on
the network, a loss is recorded once rather than once per retry, and a stand
with no broker carries on rendering.

Two ways to read them:

| | |
|---|---|
| `make logs` | prints them in a terminal: `ARGS='--since 1d'`, `ARGS=-f` to follow, or a LogsQL filter such as `ARGS="'event:scene.*'"` |
| Grafana | the dashboard **LP Stand** in EdS's organization, declared in the tenant's `dashboards.tf` |

## Updates through the broker

A build reaches the stand without a cable ([ADR-0035](../../docs/content/docs/architecture/decisions/0035-firmware-is-updated-through-the-broker.md)):

```bash
make build ota
```

`ota` sends `build/lp-stand.bin` to the stand this build is for (its id,
read from `sdkconfig`), through `eds-ota` on the services workload
(`tools/ota`). The stand writes it to its spare slot as it arrives,
answering each piece with the next byte it wants, and restarts into it.
The new build is **on probation** until it reaches the broker; one that
has not within two minutes is given up, and the build before it boots
again, by itself. One that crashes first is put back by the bootloader.

The strip stutters while a build is written: writing flash pauses the
frames for a few milliseconds a piece.

**The first build with this goes on by cable**, because the bootloader's
rollback is not updated over the air. An `sdkconfig` from before it has
rollback off and stops the build with an `#error`: delete it and build
again.

| Event | When | Fields |
|---|---|---|
| `ota.offered` | a build is offered, and taken | `bytes`, `elf` |
| `ota.refused` | it is not this firmware's, does not fit, stops arriving or is not whole | `why` |
| `ota.applied` | it is whole and checked; restarting into it | `version`, `elf` |
| `ota.kept` | a new build reached the broker, and stays | `version`, `elf` |

## Where it sits on the network

The stand belongs on Deevnet IoTaaS's **IoT segment (VLAN 30)** — the model's
slot for "custom-developed embedded devices with controlled firmware" — and
dials **`mqtt.mobile.deevnet.net` on IoT Backend (VLAN 35)**.

That placement is forced, not chosen: MQTT clients always initiate, so whichever
segment holds the broker must accept inbound, and the tenant fabric has no
inbound path by design (Deevnet ADR-0001, Deevnet ADR-0003). IoT Backend is the segment already
defined to accept exactly this.

## Roadmap

- **Per-device mutual TLS.** Today the stand authenticates with a username and
  password. They are provisioned into NVS and never compiled in, but they are
  still a shared-secret credential in plaintext flash, which is the weak
  point. Client
  certificates per stand, with broker ACLs by CN, fit the "firmware built and
  managed through the Deevnet pipeline" model. Deferred because it means a
  certificate lifecycle for one device — but wanted, and `main/certs/` is where
  that material lands.
- **Signed builds.** Updates come through the broker (ADR-0035), and only
  the `ota` account can send one, but the images are not signed: a stolen
  broker password is enough. `CONFIG_SECURE_SIGNED_APPS_NO_SECURE_BOOT`
  would close that, for a key to keep.
- **More effects**, once there is something driving them beyond cover art.
- **A `deevnet-docs` roadmap page**, matching how ma-bell is tracked, since this
  firmware lives in the EdS monorepo rather than its own repository.
