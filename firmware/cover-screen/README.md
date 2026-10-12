# cover-screen

Firmware for a small touch screen that shows the cover of what is playing
and, when touched, five buttons: play or pause, previous, next, volume up and
volume down.

```
nowplayd ──▶ mqtt01 ──▶ cover screen          the track, and its cover at 240 px
cover screen ──▶ mqtt01 ──▶ nowplayd ──▶ agent Pi ──▶ iTunes      a button press
```

Why it is built this way is
[ADR-0020](../../docs/content/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays.md).

## Status

**Running on one board since 2026-10-11** ([CHG-0012](../../docs/content/docs/changes/2026/0012-the-cover-screen.md)).
It shows the cover, and its buttons pause, resume and skip the real iTunes.
The pure half has 144 host checks. Still open: a day connected, and why its
connection once stalled for ten seconds.

## The board

An ESP32-2432S028R, the "cheap yellow display": a plain ESP32, 4 MB of flash,
no external memory.

| Part | | Pins |
|---|---|---|
| Panel | ILI9341, 320 x 240, SPI | clock 14, data 13, select 15, data/command 2, backlight 21 |
| Touch | XPT2046, resistive, SPI on pins of its own | clock 25, in 32, out 39, select 33 |

All of them are settings (`make menuconfig`), with these as the defaults.

## How it works

The screen subscribes to two retained topics and publishes to one.

- `eds/nowplaying/current` says what is playing and whether it has a cover.
- `eds/nowplaying/current/art/small` is that cover as a small baseline JPEG.
- `eds/nowplaying/command` is where a button press goes.

It knows no player. Which track is current, and which player a command
reaches, are nowplayd's to say.

### Drawing without a frame

A whole screen is 150 KB at 16 bits a pixel, and the board has about 100 KB
free with WiFi and TLS up. So nothing here ever holds a frame.

The cover is kept as the JPEG it arrived as, at most 32 KB. To draw, the
ROM's decoder hands over one row of 16-pixel blocks at a time; that row is a
**band**, the buttons that fall in it are painted over it, and it is sent to
the panel. The next row reuses the same 10 KB buffer.

The picture is decoded again each time the buttons come or go. How long
that takes has not been timed on the board.

### What a touch means

- The cover is the screen. Nothing is drawn over it until it is touched.
- **The first touch only brings the buttons up**, and has to lift before a
  button will act.
- A button acts when pressed, not when released. Volume repeats while held.
- The whole screen is a button: the middle is play and pause, and the rest is
  split along its diagonals. A finger does not have to find the icon.
- The buttons go away five seconds after the last touch.
- **When what is playing has no controls, a touch does nothing.**
  `current` says so with `no_controls`
  ([ADR-0025](../../docs/content/docs/architecture/decisions/0025-current-says-when-there-are-no-controls.md)),
  as it does for a record on the turntable. The screen still knows no
  player: it is told there is nothing to offer, not why.

### No cover

A drawn record, its label lit while something plays. It is shown when the
track names no cover, as a Spotify track does not, even though the cover
topic still holds an earlier album's.

### Failure behaviour

Each is a choice against a screen that looks broken:

- **No network or no broker**: the screen keeps what it was showing and marks
  a corner, red for no WiFi, amber for no broker. The buttons still come up,
  drawn faint, and send nothing.
- **A press is never queued.** It is sent at QoS 0 and only while connected.
  A press delivered after a reconnect would skip a track nobody is asking to
  skip.
- **A press is sent only to a broker that is answering.** A connection can
  stop carrying anything and still accept what is written to it, and deliver
  it all when it recovers. So if the broker has not been heard from in three
  seconds, the screen asks it something harmless first and waits up to
  800 ms. No answer: the press is dropped, the corner goes amber, and the
  connection is made again.
- **A cover that is too large or cannot be decoded** is dropped and the
  record is shown, with a `cover.rejected` event.
- **A `current` of another version, or that is not JSON**, is ignored and the
  screen keeps what it had, with a `current.rejected` event.
- **An unprovisioned screen** boots, lights and shows the record with a red
  mark.

## Host tests

```bash
make test        # gcc only: no ESP-IDF, no board
make pictures    # draws each screen to test/host/pictures/*.bmp
```

`ui.c` (what a touch means), `draw.c` (what is painted), `touch_map.c` and
`event_format.c` are pure C and are covered. `make pictures` uses the same
drawing code as the board, on a made-up cover, so the look of the buttons can
be changed and judged without flashing.

The panel, the touch controller, the JPEG decoder, WiFi and MQTT are ESP-IDF
glue with no host tests.

## Build, flash, provision

```bash
. ~/.espressif/tools/activate_idf_v6.1.sh
make build
make backup                 # once: saves what the board came with to backup/
make flash
make provision              # the WiFi key, broker account and CA, from the tenant
make monitor
```

`make provision` needs the tenant applied with this screen's account in it,
and writes only the `creds` partition. The two secrets have no build option
and cannot be compiled in ([ADR-0004](../../docs/content/docs/architecture/decisions/0004-firmware-secrets-nvs-only.md)).

`PORT` defaults to `/dev/cu.usbserial-2120`.

## First flash

What to check, in order, with the settings that fix each:

1. **The picture is upside down**: Panel → *Turn the picture upside down*.
2. **Red and blue are swapped**: Panel → *wired blue-green-red*.
3. **It looks like a negative**: Panel → *Invert colours*.
4. **Touches land in the wrong place.** The defaults are right for this
   board: axes swapped, both turned round. For another, each press prints a
   line on the serial console:

   ```
   I (5123) screen: touch: read 1890,2105, taken as 154,125
   ```

   Press somewhere you can name, a corner, and compare. **Do not judge it by
   pressing the buttons**: they sit on the axes, where a swapped pair of
   axes reads as a plausible button, just the wrong one.
5. **It wakes by itself, or wants a fingernail**: Touch → *Pressure that
   counts as a touch*.

## Events

As the stand's, to `eds/log/<id>`: `system.boot`, `wifi.connected`,
`wifi.disconnected`, `mqtt.connected`, `mqtt.disconnected`, `screen.health`,
and its own:

| Event | When | Fields |
|---|---|---|
| `command.sent` | a button press was published | `command` |
| `command.not_sent` | it could not be | `command` |
| `cover.rejected` | a cover was too large or could not be drawn | `bytes`, `limit` |
| `current.rejected` | a track message could not be read | `bytes` |
| `mqtt.stalled` | the broker did not answer when a press asked | |

## Shared with the stand

`config/cs_credentials.*`, `network/wifi`, `app/events` and the partition
table are copies of `firmware/lp-stand`'s, renamed. A fix to one is owed to
the other.
