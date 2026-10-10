# lightd

Cover image in, MQTT scene out.

`lightd` is the piece that joins the palette extractor to the LP jacket stand.
It takes a cover image over HTTP, asks [`palette`](../palette) for the colours
in it, maps those onto a scene the stand can render, and publishes that scene
retained to the stand's MQTT topic.

```
cover image ──▶ lightd ──▶ palette ──▶ lightd ──▶  mqtt  ──▶ LP stand
```

`POST /v1/cover` was built as the interface an album cover resolver would
call, so that nothing here would be thrown away when one arrived. It arrived
as a topic rather than a caller.

### Covers from a topic

Set `LIGHTD_COVER_TOPIC` and lightd also subscribes to that topic and runs
each image it receives down the same path as `POST /v1/cover`, for the default
stand ([ADR-0009](../../docs/content/docs/architecture/decisions/0009-lightd-takes-covers-from-a-topic.md)).
This is how the now-playing service lights the stand: it publishes the current
cover, retained, and lightd follows it.

- lightd still receives **image bytes and nothing else**. It does not learn
  what the image is a cover of; the topic is a setting, not knowledge.
- The topic is retained, so a lightd that restarts lights the room from the
  current cover by itself.
- Covers are handled one at a time and **the newest wins**: one that arrives
  while another is being handled replaces any still waiting.
- A cover that cannot be used — not an image, palette down — is logged and
  ignored, and the stand keeps the last good scene.
- The subscription is made again on every reconnect. The broker account needs
  the topic granted for subscribe, and a refused subscription is silent.

## Running it

```bash
make broker          # development MQTT broker in podman
make run             # needs the palette service on :8731

make check           # fmt, vet, unit tests
make test-integration # adds the tests that need a real broker
make broker-watch    # tail every eds/# topic
```

```bash
# The whole vertical in one request.
curl -F image=@jacket.jpg http://localhost:8732/v1/cover

# What lightd itself will send: raw bytes, no multipart envelope.
curl --data-binary @jacket.jpg -H 'Content-Type: image/png' \
     'http://localhost:8732/v1/cover?stand=lp-stand-02&effect=sweep'

# Publish a scene by hand - how the stand gets exercised before any image path
# exists, and how a specific effect gets tested without hunting for cover art.
curl -X POST http://localhost:8732/v1/scene \
  -d '{"effect":"solid","brightness":1,"palette":[{"rgb":[255,0,0],"weight":1}]}'

curl 'http://localhost:8732/v1/scene?stand=lp-stand-01'   # what was last sent
```

## Configuration

All environment, no config file: the broker credentials must never land on disk
in a repository, and the same binary has to work under a systemd unit or a
container without knowing which.

**The broker settings fall back to the platform's names**, the ones in a
Deevnet tenant's `kit.env`: `MQTT_HOST` (dialed as `tls://MQTT_HOST:MQTT_PORT`,
port 8883 by default), `MQTT_USERNAME`, `MQTT_PASSWORD`, `MQTT_CA_FILE`, and
`DEEVNET_TENANT` as the topic prefix. So `kit.env` alone is enough, on a
workload or a Pi. A `LIGHTD_*` variable, when set, wins.

| Variable | Default | |
|---|---|---|
| `LIGHTD_ADDR` | `:8732` | listen address |
| `LIGHTD_PALETTE_URL` | `http://127.0.0.1:8731` | palette service |
| `LIGHTD_STAND_ID` | `lp-stand-01` | default stand |
| `LIGHTD_COVER_TOPIC` | unset | a topic of cover images to light the stand from, relative to the prefix, e.g. `nowplaying/current/art` |
| `LIGHTD_TOPIC_PREFIX` | `$DEEVNET_TENANT`, else `eds` | root of the topic tree |
| `LIGHTD_MQTT_URL` | `tls://$MQTT_HOST:$MQTT_PORT`, else `tcp://127.0.0.1:1883` | the broker |
| `LIGHTD_MQTT_CLIENT_ID` | `lightd` | must be unique on the broker; `eds-lightd` in production |
| `LIGHTD_MQTT_USERNAME` / `_PASSWORD` | `$MQTT_USERNAME` / `$MQTT_PASSWORD` | broker credentials |
| `LIGHTD_MQTT_CA_FILE` | `$MQTT_CA_FILE` | CA for the broker's certificate |
| `LIGHTD_MQTT_INSECURE` | `false` | bring-up only; never with a real cert |
| `LIGHTD_EFFECT` | `breathe` | default effect |
| `LIGHTD_SPEED` / `LIGHTD_BRIGHTNESS` | `0.4` / `0.8` | |
| `LIGHTD_SWATCHES` | `6` | swatches requested from palette |
| `LIGHTD_MIN_CHROMA` | `0.05` | below this a swatch is a grey |
| `LIGHTD_SATURATION` | `1.25` | chroma multiplier for the strip |
| `LIGHTD_MAX_COLORS` | `4` | palette entries per scene |

Invalid values stop the daemon rather than lighting the room wrong quietly.

## Topics

| Topic | Direction | Retained | |
|---|---|---|---|
| `eds/lightstand/<id>/scene` | lightd → device | **yes** | what to render |
| `eds/lightstand/<id>/status` | device → broker | yes (LWT) | the stand's presence |
| `eds/lightd/status` | lightd → broker | yes (LWT) | this daemon's presence |

**Retention is load-bearing, not an optimisation.** A stand that reboots — power
blip, reflash, WiFi drop — pulls the current scene down on reconnect instead of
sitting dark until the next record. Without it the failure mode is a black stand
in the middle of an album. There is an integration test for exactly this: it
publishes, *then* connects a subscriber, and requires the scene to arrive.

lightd publishes its own presence separately from the stands', because a dark
stand and a dead publisher look identical from the room.

## The scene descriptor

```json
{
  "v": 1,
  "effect": "breathe",
  "speed": 0.4,
  "brightness": 0.8,
  "palette": [
    { "rgb": [232, 77, 0], "weight": 0.5594 },
    { "rgb": [182, 159, 0], "weight": 0.3661 }
  ]
}
```

**Never pixel frames.** Streaming per-pixel data over WiFi at any useful frame
rate turns latency spikes into visible stutter, and a broker outage would freeze
an animation mid-sweep. The device renders named effects locally at its own
rate; the network stays out of the frame loop.

## What lightd decides, and what it does not

The palette service returns raw perceptual data — ranked swatches with weights,
no roles, no correction — because it has no idea what is on the other end.
lightd does, so the hardware-facing decisions live here:

- **Rejecting neutrals.** A swatch below `LIGHTD_MIN_CHROMA` will not read as a
  colour on a strip regardless of how much of the sleeve it covers. Weight alone
  is not enough: asking for few swatches from busy art forces clusters to merge,
  and a merged centroid lands near the average of what it merged — a high-weight
  grey. Filtering never returns nothing, though; genuinely monochrome art keeps
  its most colourful swatch, because the stand still has to light up.
- **Saturation.** A palette that looks right on a monitor reads washed out on a
  strip. Chroma is scaled in OKLab, so a boosted colour stays recognisably the
  same colour.
- **Gamut mapping.** Boosting chroma pushes colours outside sRGB. Rather than
  clipping channels — which shifts hue, so a boosted orange arrives as a
  different colour than the one on the sleeve — chroma is walked back down until
  the colour is representable, holding hue and lightness fixed.

**Gamma is not here.** It is a property of the specific LED driver, so it
belongs in firmware next to the hardware it corrects. Keeping it out of the
payload means `mosquitto_sub` shows the colours actually intended, which matters
the first time something looks wrong.

## The broker is not in the tenant

The broker lives on the substrate's IoT Backend segment (VLAN 35), and lightd
connects outbound to it from the EdS tenant.

MQTT clients always dial the broker, so whichever segment holds it must accept
inbound — and the tenant fabric has no inbound path by design (Deevnet ADR-0001,
Deevnet ADR-0003). IoT Backend is the segment already defined to accept exactly this,
with the broker named in the model as its typical inhabitant. Putting the broker
on a tenant VM would have meant inventing an inbound path to tenant address
space, which is an ADR, not a config change.
