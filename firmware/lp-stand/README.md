# lp-stand

Firmware for the LP jacket stand: a stand that props up the record sleeve while
vinyl spins, lit by an addressable RGB strip that reacts to the cover art.

ESP-IDF in C, following the conventions of
[`esp32-ma-bell-gateway`](https://github.com/cdeever/esp32-ma-bell-gateway) —
`main/app/`, `main/config/`, `main/hardware/`, `main/network/`, with a Makefile
wrapper over `idf.py`.

## Status

**The hardware-independent half is tested. The device half is not yet built.**

| | |
|---|---|
| `scene.c`, `effects.c`, `gamma.c` | 23 tests, 651 checks, `-Wall -Wextra -Werror` |
| `scene_json.c`, `led_strip_out.c`, `wifi.c`, `mqtt.c`, `main.c` | written, **never compiled** — no ESP-IDF or hardware yet |

The split is deliberate rather than incidental. Scene validation, effect
rendering and gamma are where a bug is subtle and shows up only as *the light
looks wrong*, so they are pure C with no ESP-IDF dependency and are tested with
a compiler:

```bash
make test        # needs gcc and nothing else
```

The ESP-IDF glue is thin by design and needs a device to trust:

```bash
. $IDF_PATH/export.sh
make menuconfig  # WiFi, broker, strip length and pin
make build flash monitor PORT=/dev/ttyUSB0
```

## How it works

```
mqtt01 ──▶ eds/lightstand/<id>/scene ──▶ parse ──▶ validate ──▶ render task ──▶ strip
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

`solid`, `breathe`, `sweep` — deliberately few, because adding one means
reflashing every stand.

`sweep` lays the palette out in proportion to the weights lightd sent, so a
colour covering 60% of the sleeve covers 60% of the strip, and cross-fades at
the seams: hard edges on a diffused strip look like a fault in the diffuser.

An **unknown effect name falls back to `solid`** rather than being refused. A
publisher that learns a new effect before the stands do is expected; a lit stand
showing the right colours with the wrong animation beats a dark one.

### Gamma lives here

lightd deliberately does not gamma-correct. WS2812 PWM is linear in duty cycle
while perception is not, so an uncorrected fade rushes through the dark end and
then stalls — but the right curve is a property of *this* driver and diffuser.
Correcting at the output stage keeps it next to the hardware it corrects, and
keeps the wire format showing the colours actually intended.

### Failure behaviour

Everything below is a deliberate choice, not an accident:

- **A malformed or unparseable scene is ignored**, and the previous one keeps
  rendering. A stand that goes dark on a bad payload is worse than one that
  ignores it.
- **A scene with a different contract version is refused**, not guessed at.
- **WiFi never gives up.** An unattended stand that stops retrying is one that
  needs a person to walk over and power-cycle it.
- **The strip lights before the network exists** — a dim warm white — so an
  unconfigured or unreachable stand looks deliberate rather than broken.
- **A frame is skipped rather than stalled** if the render task cannot take the
  scene lock. A skipped frame is invisible; a stalled strip is not.
- **Last will on `status`**, so the room learns a stand died rather than merely
  went quiet.

## Topics

| Topic | Direction | Retained | |
|---|---|---|---|
| `eds/lightstand/<id>/scene` | broker → stand | yes | what to render |
| `eds/lightstand/<id>/status` | stand → broker | yes (LWT) | `online` / `offline` |
| `eds/lightstand/<id>/state` | stand → broker | yes | what it is rendering now |

## Where it sits on the network

The stand belongs on the substrate's **IoT segment (VLAN 30)** — the model's
slot for "custom-developed embedded devices with controlled firmware" — and
dials **`mqtt01` on IoT Backend (VLAN 35)**.

That placement is forced, not chosen: MQTT clients always initiate, so whichever
segment holds the broker must accept inbound, and the tenant fabric has no
inbound path by design (ADR-0001, ADR-0003). IoT Backend is the segment already
defined to accept exactly this.

## Roadmap

- **Per-device mutual TLS.** Today the stand authenticates with a username and
  password embedded in the firmware image, which is the weak point. Client
  certificates per stand, with broker ACLs by CN, fit the "firmware built and
  managed through the Deevnet pipeline" model. Deferred because it means a
  certificate lifecycle for one device — but wanted, and `main/certs/` is where
  that material lands.
- **OTA.** The partition table already carries two app slots, because
  retrofitting them means repartitioning a device glued to a shelf.
- **More effects**, once there is something driving them beyond cover art.
- **A `deevnet-docs` roadmap page**, matching how ma-bell is tracked, since this
  firmware lives in the EdS monorepo rather than its own repository.
