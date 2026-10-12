---
title: "Architecture"
weight: 10
---

# Architecture

EdS is layered, and the layering is load-bearing rather than decorative. Each
layer is deliberately ignorant of the one below it, which is what lets the
palette extractor be tuned against a folder of JPEGs with no infrastructure at
all, and what lets the stand keep doing something sensible when the network
disappears.

## The system, as it is running

[![Every component of EdS and what each talks to: the turntable, its power strip, converter and beacon upstairs; the selector, sound card and agent Pi in the basement; iTunes and Spotify; the broker; the four services; the stand and the cover screen.](system.svg)](system.svg)

Read it left to right: where the music comes from, what hears it, the broker
where everything meets, and what acts on it. Select the picture to see it at
full size.

| Line | What it carries |
|---|---|
| Heavy blue | Sound: the record, as phono, then S/PDIF, then USB audio |
| Dashed amber | AC power, from the strip's switched outlets |
| Green | The beacon's one message: that it has power |
| Gray | Everything else, almost all of it MQTT through the broker |
| Dotted | Device events, on their way to the log store |

Gray boxes are someone else's service. A dashed box is turned off.

- **The turntable is heard, and separately known to be on.** Its sound goes
  through a converter, down the house as S/PDIF, through a selector it shares
  with the media server, and into a sound card on the agent Pi, where a
  driver names what it hears
  ([ADR-0021](/docs/architecture/decisions/0021-the-turntable-is-identified-by-listening/)).
  That feed is there whether or not a record is playing. So the power strip
  says the rest: it powers its other outlets while the turntable draws
  power, and a beacon on one of them is connected to the broker for exactly
  as long ([ADR-0023](/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand/)).
- **iTunes is asked, not heard.** A driver on the Pi pairs with it as a
  remote does, reports what it plays, and carries out the cover screen's
  commands ([ADR-0011](/docs/architecture/decisions/0011-itunes-by-remote-pairing/)).
- **Spotify is read from the account**, and is turned off for now
  ([CHG-0011](/docs/changes/2026/0011-spotify-turned-off/)).
- **Nothing on the bus talks to anything else directly.** Every MQTT line
  goes to the broker, which is the substrate's and not the tenant's
  ([ADR-0003](/docs/architecture/decisions/0003-broker-is-the-substrates/)).

The picture is drawn by GraphViz from `system.dot`, beside this page. Change
it there and run `make -C docs diagrams`; both files are committed, because
the site is built without GraphViz.
