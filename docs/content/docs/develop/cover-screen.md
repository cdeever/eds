---
title: "Firmware: cover-screen"
weight: 2
---

# Firmware: cover-screen

The touch screen that shows the cover and has the five buttons
([ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/)).
ESP-IDF in C, laid out as [lp-stand](../lp-stand/) is, for an
ESP32-2432S028R.

## The split that matters

What a touch means and what is painted over the cover are pure C, because
that is where a bug shows up only as *it did the wrong thing when I touched
it*:

```bash
make test        # needs gcc; no ESP-IDF, no hardware
make pictures    # draws each screen to a BMP, with a made-up cover
```

`make pictures` runs the board's own drawing code. Change an icon, run it,
and look.

## Nothing holds a frame

The board has no memory for one. The cover is kept as a JPEG and decoded a
row of blocks at a time, straight to the panel, with the buttons painted
into each row as it passes. Anything added to the screen has to be drawable
a band at a time: `draw.h` says how.

## Build and flash

```bash
. ~/.espressif/tools/activate_idf_v6.1.sh
make build
make backup      # once, before the first flash
make flash
make provision
```

The Makefile's recipes are one line each, so it runs under macOS's make 3.81.

The first flash of a board needs its panel and touch settings checked; the
[firmware's README](https://github.com/cdeever/eds/tree/main/firmware/cover-screen#first-flash)
lists what to look for.

## What it shares with the stand

Credentials, WiFi, the event log and the partition table are copies of the
stand's. Change one, change the other.

## The contract it speaks

[Now Playing](../../reference/now-playing/): it reads `current` and the small
cover, and publishes commands. A host test pins the command's wire form to
the one `nowplayd`'s own test pins.
