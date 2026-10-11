# console

One page that shows what EdS is doing: what is playing and on which player,
its cover, each stand drawn in the colours and motion it was sent, and whether
each part is up.

```
                        ┌──────────────────────────── broker ───────────────────────────┐
 nowplaying/current ───▶│                                                               │
 nowplaying/current/art▶│   consoled  ──▶  one page  ──▶  a browser on the site         │
 nowplaying/source/+/…─▶│   (subscribes, and serves what it heard; publishes nothing    │
 lightstand/+/…  ──────▶│    but its own presence)                                      │
 lightd/status ────────▶│                                                               │
                        └───────────────────────────────────────────────────────────────┘
```

The decision, and what was tried first, is
[ADR-0018](../../docs/content/docs/architecture/decisions/0018-a-console-of-eds-own.md).

## It only looks

The console learns everything from the bus and changes nothing. It calls no
other service, and none calls it. Its broker account is granted subscriptions
and one topic to publish, its own presence. It answers `GET` and nothing else.

Controls are expected one day. They are not a feature to add here quietly:
they change what the account may publish and who should be able to reach the
page, and that is a decision to record first.

## What the page shows, and what it does not claim

- **What is playing** is what `nowplayd` says is current. The console decides
  nothing.
- **The cover** is the image on the cover topic, the one the stand is lit
  from. It is shown as the track's cover only when the track names it. A
  source reported without its cover (Spotify,
  [ADR-0016](../../docs/content/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover.md))
  shows none, and the stand is shown still lit from the earlier one.
- **The stand** is drawn from the scene `lightd` sent it, animated by a port
  of the firmware's effects. It is a picture of the scene, not a measurement
  of the LEDs. Whether the stand took the scene is its own `state` and
  presence, which the page also shows.
- **Recently played** is what the console has seen since it started. The
  history that lasts is in the tenant's log store.

## The effects are written twice

`internal/web/static/effects.js` is a port of
`firmware/lp-stand/main/app/effects/effects.c`, and of the validation in
`scene.c`. If an effect changes in the firmware it has to change here, or the
page draws a stand that does not exist. `internal/web/effects.test.mjs` asks
the port the firmware's own questions (`make test`, where node is installed).

Colours are drawn as sent, before gamma. That is what the wire format carries
on purpose ([ADR-0002](../../docs/content/docs/architecture/decisions/0002-gamma-in-firmware.md)).

## Running it

```bash
make -C ../nowplaying broker     # the development broker, shared
make run                         # http://127.0.0.1:8089, under edsdev/
make feed                        # a sample of the bus: a track, a cover-less stand
make feed WHAT=spotify EFFECT=sweep
make feed COVER=~/some-cover.jpg # with a cover
```

`make check` runs the formatter check, vet and the tests.

## Configuration

Environment only. Each broker setting falls back to the platform's name from
`kit.env`; a `CONSOLE_*` variable wins.

| Variable | Default | |
|---|---|---|
| `CONSOLE_ADDR` | `:8080` | where it listens; `:80` on the workload |
| `CONSOLE_MQTT_URL` | `tls://$MQTT_HOST:$MQTT_PORT`, else `tcp://127.0.0.1:1883` | broker |
| `CONSOLE_MQTT_CLIENT_ID` | `eds-console` | must be unique on the broker |
| `CONSOLE_MQTT_USERNAME` / `CONSOLE_MQTT_PASSWORD` | `$MQTT_USERNAME` / `$MQTT_PASSWORD` | `kit.env` carries lightd's login, so the console names its own |
| `CONSOLE_MQTT_CA_FILE` | `$MQTT_CA_FILE` | the Deevnet Root CA |
| `CONSOLE_MQTT_INSECURE` | `false` | bring-up only; anything but a boolean is an error |
| `CONSOLE_TOPIC_PREFIX` | `$DEEVNET_TENANT`, else `eds` | one topic level |
| `CONSOLE_DASHBOARD_URL` | none | where "Events in Grafana" goes; left out of the page if not a web address |
| `CONSOLE_DOCS_URL` | this project's docs site | where "Decisions and records" goes |

## Things worth knowing

- **Every subscription has to be granted as written.** The broker matches a
  subscription against what was granted as a whole and says nothing when it
  refuses one. `view.Topics` and the account in
  `infra/deevnet-tenant-eds/main.tf` are the same list, and a part of the page
  stuck at "unknown" is what a difference looks like.
- **Only pictures are kept as covers**: JPEG, PNG, WebP and GIF, by their
  bytes. An SVG is a document and can carry script; it is refused.
- **The page loads nothing from anywhere else**, and a content policy says
  so. There are no inline styles or scripts for it to make an exception for.
- **No login and no TLS.** Acceptable for a page that only looks, on the
  site's trusted networks. Not acceptable for a page with controls.
- **Viewers are capped at 64.** Each is a request that never ends.
