---
title: "CHG-0003: The Stand Follows What Is Playing"
weight: 3
---

# CHG-0003: The Stand Follows What Is Playing

| | |
|---|---|
| **Date** | 2026-10-11 |
| **Change type** | Deployment |
| **Status** | {{< status-badge "complete" "Complete" >}} 2026-10-11 |
| **Systems** | the `services` workload of the `eds` tenant: `eds-lightd`, and a new `eds-nowplayd`; by consequence, `lp-stand-01` |
| **How** | `make -C deploy deploy WHERE=builder` |
| **Risk** | Low. `lightd` gains an input and loses none. Most likely to go wrong: a settings file that leaves `nowplayd` unable to log in to the broker, which shows as a unit restarting |
| **Decisions** | [ADR-0006](/docs/architecture/decisions/0006-now-playing-bus-of-drivers/), [ADR-0008](/docs/architecture/decisions/0008-nowplayd-arbitrates-and-enriches/), [ADR-0009](/docs/architecture/decisions/0009-lightd-takes-covers-from-a-topic/) |
| **Related** | [CHG-0002](/docs/changes/2026/0002-relay-pi-and-now-playing-accounts/), whose follow-up this is |

---

## Summary

Since [CHG-0002](/docs/changes/2026/0002-relay-pi-and-now-playing-accounts/)
the agent Pi has reported what iTunes is playing to the broker, and nothing
has acted on it. The stand shows whatever cover was last sent to it by hand.

After this change `nowplayd` runs on the workload beside `palette` and
`lightd`. It reads the agent's reports, publishes the current track and its
cover, and `lightd` lights the stand from that cover. Playing an album in
iTunes changes the stand.

## Goal

- Three units are active on the workload: `eds-palette`, `eds-lightd`,
  `eds-nowplayd`.
- `eds/nowplaying/current` on the broker names what iTunes is playing, with a
  title, an album and `art`.
- Changing album in iTunes changes the stand, and the stand reports
  `scene.applied`.
- Pausing leaves the stand as it is.
- `POST /v1/cover` still works.

## Scope

**In scope:** a new image and unit for `nowplayd`; its two settings files; a
rebuilt `lightd` image and one more line in `lightd.env`.

**Out of scope:** the agent Pi, which is unchanged; Spotify; any change to
the tenant's broker accounts, which CHG-0002 made.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| `nowplayd` cannot log in | the workload | its login is generated from the tenant's state, and the account was checked against the broker in CHG-0002 |
| The stand repaints the moment `lightd` restarts | `lp-stand-01` | intended: the current cover is retained, and a restarted `lightd` lights the room from it |
| A catalog lookup hangs or fails | `nowplayd` | lookups are bounded at 10 s, and a track is published without a cover rather than withheld. iTunes supplies its own covers, so no lookup is expected here at all |
| The workload cannot reach the catalogs | the tenant's egress | not exercised by this change; noted for the first source that has no cover of its own |
| `lightd` is briefly down | the workload | the stand keeps rendering its retained scene; nothing depends on `lightd` between covers |

## Procedure

### Step 1: Build and load the images

The development Mac has no container tool, so the images are built on the
Builder and loaded from there.

```bash
make -C deploy images load WHERE=builder
```

### Step 2: Settings and units

```bash
make -C deploy units
```

Writes `kit.env` and `nowplayd-broker.env` from the tenant's state, copies
them with `lightd.env`, `nowplayd.env`, the CA and the three units, and
restarts the services.

## Verification

- `make -C deploy status` shows three active units.
- `nowplayd` logs `connected to broker`, then `current` with `art_from=player`.
- `lightd` logs `listening for covers`, then `published scene via=topic`.
- With a different album playing in iTunes, the stand changes, and
  `make -C firmware/lp-stand logs` shows `scene.applied`.

## Undo

- Remove `LIGHTD_COVER_TOPIC` from `lightd.env` and run `make -C deploy units`:
  `lightd` is back to HTTP only.
- `sudo systemctl disable --now eds-nowplayd` on the workload stops the rest.
- The previous `lightd` image is replaced by the load. It is rebuilt from the
  commit before this one if it is ever wanted.

## Outcome

| When (UTC) | Step | What happened |
|---|---|---|
| 15:1x | 1 | Three images built on the Builder and loaded onto the workload. The workload reached a public catalog over HTTPS, the first time its egress was used for one. |
| 15:20 | 2 | Units installed and started. All three active. **The stand was not lit**: see the two departures below. |
| 15:23 | 1, 2 | Both fixed, rebuilt, shipped. On the restart `nowplayd` published the current track once with the player's own cover, and `lightd` retried twice while `palette` came up, then published a scene. |
| 15:26 | — | The stand had been powered off. Powered on, it joined the network, reached the broker and applied the retained scene within three seconds. |
| 15:29–15:32 | — | Four album changes in iTunes, each a new scene of four colors from the library's own cover, sent 0.26–0.59 s after the track change and applied by the stand within about a second. A pause sent nothing. The same album played twice produced the same scene. |
| 15:34 | 1 | Shipped again with the colors of each scene in `lightd`'s log. |

All of the goal's checks hold, with one caveat: the stand has no LEDs attached
yet, so "the stand changes" was confirmed by its own `scene.applied` reports
and by the scene `lightd` sent, not by eye.

### Departures from the plan

- **`lightd` dropped the cover at start-up.** The three services restart
  together, the retained cover arrives milliseconds after `lightd`
  subscribes, and `palette` was not yet listening. `lightd` logged *ignored a
  cover* and did not try again. A cover from the topic is not sent twice, so
  the stand would have shown the wrong record until the album changed.
  `lightd` now retries a cover that failed for a reason that may pass, and
  gives up at once on one that will not.
- **`nowplayd` published a catalog's cover, then the player's.** A broker
  handing a new subscriber its retained messages promises no order. `nowplayd`
  was given iTunes's state before the cover that state names, took the track
  to have none, fetched one from Deezer and published it, and published
  iTunes's own 24 ms later. It now holds a state that names a cover for up to
  two seconds for that cover to arrive.
- Both were assumptions about order that held on a desk, where things were
  started one at a time. Each now has a test named for what was seen.
- **The colors a cover produced were not recorded.** `lightd` logged how many
  and not which, and only the latest scene is kept. Answering "what did it
  pick?" meant asking for an album to be played again. The log line now
  carries each color and its share.
- **The first cover read as a single gray.** A mostly black-and-white sleeve,
  99.99% of it discarded as background, and the fallback taken. Not a fault,
  and not a good first impression of the palette.
- The development Mac's record of the workload's SSH key was stale, from
  before the tenant was recreated. The key now presented matched the one the
  Builder trusts, and the stale entry was replaced.

## Follow-ups

- [ ] Attach the LED strip, and look.
- [ ] Each service's log line appears twice in the journal: once from the
  container and once from the unit. Harmless, and noisy.
- [ ] A dashboard panel showing what the stand is showing: the colors, and
  if Grafana allows it, the pattern. The stand's `scene.applied` event would
  need to carry its colors, and whether a tenant's Grafana can draw a color
  taken from data wants a short spike.
- [ ] `nowplayd` and `lightd` log to the journal only. Shipping their logs to
  the tenant's log store would put them beside the stand's.
