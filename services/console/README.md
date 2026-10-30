# console

One page that shows what EdS is doing: what is playing and on which player,
its cover, each stand drawn in the colors and motion it was sent, and whether
each part is up.

```
                        ┌──────────────────────────── broker ───────────────────────────┐
 nowplaying/current ───▶│                                                               │
 nowplaying/current/art▶│   consoled  ──▶  one page  ──▶  a browser on the site         │
 nowplaying/source/+/…─▶│   (subscribes, and serves what it heard; publishes nothing    │
 lightstand/+/…  ──────▶│    but its own presence)                                      │
 lightd/status ────────▶│                                                               │
 beacon/+/status ──────▶│                                                               │
 screen/+/status ──────▶│                                                               │
                        └───────────────────────────────────────────────────────────────┘
```

The decision, and what was tried first, is
[ADR-0018](../../docs/content/docs/architecture/decisions/0018-a-console-of-eds-own.md).

## It looks, and switches two settings

The console learns everything from the bus. It calls no other service, and
none calls it. Its broker account is granted subscriptions and three topics
to publish: its own presence, which sources the stand is lit for
([ADR-0039](../../docs/content/docs/architecture/decisions/0039-the-console-can-light-the-stand-for-every-source.md)),
and whether Spotify's driver runs
([ADR-0051](../../docs/content/docs/architecture/decisions/0051-spotify-is-switched-on-and-off-from-the-console.md)).

The stand's setting is the first. Beside the stands the page says
"lit for records" or, marked, "lit for every source", and a button switches
it: `POST /api/stand` with `{"sources":"all"}` or `{"sources":"records"}`.
The server publishes `{"v":1,"sources":…}` to `nowplaying/stand`, retained,
and `nowplayd` lights iTunes and Spotify from their covers while it is `all`.
The page shows the setting when it hears it back from the broker, not when
it asked; a setting not heard back within five seconds is said so. It stays
as set until it is switched again.

Spotify's switch is the second. Under the stand's, the page says
"followed" or "off: Spotify is asked nothing", and a button switches it:
`POST /api/spotify` with `{"driver":"on"}` or `{"driver":"off"}`. The server
publishes `{"v":1,"driver":…}` to `nowplaying/source/spotify/switch`,
retained, and the agent runs the Spotify driver only while it is `on`.
Nothing on the broker is `off`. Switched on and not online after ninety
seconds, the page says the driver could not start.

The routes take JSON only, from the console's own origin, so another site
open in the same browser cannot post to them. Every other route answers
`GET` and nothing else. There is no login: neither setting changes what
plays. Another control is not a feature to add here quietly: it
changes what the account may publish, and that is a decision to record
first.

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
- **The header** has a line for each kind of part: services, sources (each
  source's driver; the turntable's is "Song detector", because it says the
  sound card is heard, not that the turntable is on), devices, and power
  switches. Green is a part heard from live in the last ninety seconds;
  gray is one only claimed by a stored `online`, or not heard lately; red
  is one that said `offline`
  ([ADR-0034](../../docs/content/docs/architecture/decisions/0034-presence-is-heard-not-remembered.md)).
- **A beacon** is a device on a switch, lit on the page while the switch is
  on ([ADR-0023](../../docs/content/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand.md)).
  Unlit is not a fault: it is the turntable being off.
- **The playing time** counts on from what the bus said: where playback was
  when the track or the play state last changed, plus the time since, by the
  server's clock and the page's own stopwatch. The viewer's clock is not
  used. **It does not know about a seek.** Dragging the playhead changes
  neither the track nor the play state, so nothing is reported, and the count
  is wrong by the size of the jump until the next pause, resume or track. It
  never runs past the end of the track.
- **Recently played** is what the console has seen since it started. The
  history that lasts is in the tenant's log store.

## The effects are written twice

`internal/web/static/effects.js` is a port of
`firmware/lp-stand/main/app/effects/effects.c`, and of the validation in
`scene.c`: all five effects. If an effect changes in the firmware it has to change here, or the
page draws a stand that does not exist. `internal/web/effects.test.mjs` asks
the port the firmware's own questions (`make test`, where node is installed).
The last of those tests holds the port to pixels printed from the firmware's
own code built for the host; when an effect changes, print them again.
`clock.js`, which works out the playing time, is tested beside it.

Colors are drawn as sent, before gamma. That is what the wire format carries
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
| `CONSOLE_TLS_CERT_FILE` / `CONSOLE_TLS_KEY_FILE` | none | a certificate with its chain, and its key. Set together, the page is served over TLS and `CONSOLE_ADDR` only redirects to it |
| `CONSOLE_TLS_ADDR` | `:443` | where TLS is served |
| `CONSOLE_STRIP_LEDS` | `18` | how many LEDs the page draws for a stand, 1 to 300. The firmware's `LP_LED_COUNT`, which nothing on the bus reports |
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
- **TLS when it is given a certificate.** With `CONSOLE_TLS_CERT_FILE` and
  `CONSOLE_TLS_KEY_FILE` the page is served over TLS, the plain port sends
  every request to the name the certificate is for (not the name the request
  used), and a browser is told to stay for a day. The certificate is read
  once, at start; a renewed one is put in place and the console restarted. A
  certificate that has expired, or names nothing, stops it before it connects
  to anything. It warns at every start once fewer than thirty days remain.
- **No login.** Acceptable for a page that looks and switches which covers
  light a strip of LEDs, on the site's trusted networks and at home
  (ADR-0039). A control that does more brings the question back.
- **Viewers are capped at 64.** Each is a request that never ends.
