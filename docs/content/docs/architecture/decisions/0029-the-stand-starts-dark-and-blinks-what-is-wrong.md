---
title: "ADR-0029: The Stand Starts Dark, and Blinks What Is Wrong"
weight: 29
---

# ADR-0029: The Stand Starts Dark, and Says in Red Blinks How Far It Got

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until each blink has been seen on the stand; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-13 |
| **Scope** | What the stand's strip shows from power on until it is sent a scene; what it shows when it cannot get that far; and how it comes on and goes off. |
| **Extends** | [ADR-0028](/docs/architecture/decisions/0028-the-stand-is-for-records/), which made a room with nothing playing dark and left the stand's own first seconds dim. [ADR-0001](/docs/architecture/decisions/0001-device-renders-effects/): the stand now draws one thing nobody sent it. |
| **Related** | [ADR-0005](/docs/architecture/decisions/0005-device-events-over-mqtt/), the stand's other way of saying what happened, which needs the broker this is for the lack of; [ADR-0004](/docs/architecture/decisions/0004-firmware-secrets-nvs-only/), whose unprovisioned stand gets a blink of its own |

---

## Context

The firmware has always lit the strip the moment it had power: a dim warm
white, "so an unconfigured or unreachable stand looks deliberate rather than
broken". Since
[ADR-0028](/docs/architecture/decisions/0028-the-stand-is-for-records/) the
stand is dark unless a record is on or the turntable is switched on. The
warm white at power on is then the one time the stand is lit with nobody
having asked: for a few seconds on every start, and for as long as the
network is away.

Dark from the start is what is wanted. What the warm white was for still
needs an answer: a stand that is dark because nothing is playing, one that
cannot find the WiFi, and one with no power all look the same.

## Options considered

**Dark, and nothing else.** The console and the event log say when a stand
is offline. But they say it to someone at a screen, and not which of four
things is wrong; and the events of a stand that cannot connect are, for that
reason, not arriving.

**Keep the warm white for a stand with no network only.** It says "alive"
and not what is wrong, and it is the light that was just asked to go.

**A separate status LED.** The right answer for a product. This is a board
on a strip, and the strip is there.

**A color for each fault.** Through a diffuser and a gamma table, amber and
red are closer than they sound, and a color has to be remembered. A count
can be written on a card.

**The whole strip blinking.** Unmissable, and so is a router restarting at
three in the morning.

**One LED, red, blinking a count.**

## Decision

**The stand shows nothing until it is sent a scene. When it has been unable
to reach the broker for twenty seconds, its first LED blinks red: the number
of blinks is the step it did not get past.**

| Blinks | What is wrong | Usually |
|---|---|---|
| 1 | Nothing provisioned: the `creds` partition is empty | a new or erased board; `make provision` |
| 2 | No WiFi: not on the network, or no address | out of range, the access point down, or a wrong key |
| 3 | No broker: on the network, and the broker is not answering | the broker or the way to it down; a CA that does not match |
| 4 | Refused: the broker answered and would not take the login | the account's password replaced; reprovision |

- **The count is how far it got**, in the order a start goes. More blinks is
  nearer to working.
- **Each blink is a fifth of a second**, three tenths apart, and two seconds
  of dark follow the count.
- **Twenty seconds of grace**, counted from when anything first went wrong,
  and begun again only by everything working. A start takes several seconds
  to reach the broker and says nothing meanwhile; a connection that drops
  and comes back says nothing either.
- **Only the first LED.** The rest of the strip goes on showing the scene it
  had, so a stand that loses its network in the middle of a record still
  shows the record. Between blinks that LED is black, so the count reads the
  same over a lit scene as over a dark one.
- **It stops the moment the broker is back.**
- **Which fault it is, is decided by code with no hardware in it**
  (`app/status`), tested on the host with the scenes and the effects. The
  network's code only answers questions.
- **The default scene is the idle scene at brightness 0.** lightd's
  `scene.Idle` is no longer "what the firmware starts with"; it is only what
  a waiting stand is sent.

### Coming on and going off

A stand that is dark most of the day changes between dark and lit several
times an evening, and a strip that snaps on and off looks like a fault. So:

- **Dark to lit, the light opens from the middle of the strip to its ends**,
  in 1.4 seconds. **Lit to dark, it closes from the ends to the middle** and
  goes out, in 1.1. What closes is the scene that was showing.
- **On and off are not sent.** A scene with brightness above zero after one
  with none is on, and the reverse is off, whatever either is for. The scene
  descriptor does not change, and `lightd` does not know
  ([ADR-0001](/docs/architecture/decisions/0001-device-renders-effects/)).
- **The moving edge is at least 0.6 bright**, and what it leaves behind is
  the scene as sent. The idle scene at 0.15 is one or two steps above off
  after gamma, and would otherwise open with nothing to see.
- **Between two lit scenes nothing is added**: dim to a record's colors
  changes as it always did.
- **A stand that restarts in the middle of a record opens onto it**, since
  it starts dark.
- **The console does not draw this.** It draws the scene a stand was sent
  (`effects.js`, the port of `effects.c`), and this is not in either.

## Consequences

- **"The strip lights before the network exists" is given up**, and with it
  the last of "a dark stand cannot be told from a dead one". A dead stand is
  now the one that is dark *and* not blinking, twenty seconds on.
- **A stand whose broker is down blinks all night**, one LED. That is the
  price of it saying anything.
- **For its first twenty seconds a stand with a fault looks like one with
  none.** Whoever is watching a new board waits.
- **Nothing is said about a broker that is there and sends no scene.** A
  connected stand with no retained scene is dark, correctly, and so is one
  whose `lightd` has died. That is the console's to show.
- **Nothing is said on the strip about why the stand last restarted.** A
  crash or a brownout is in `system.boot`, as before.
- **The stand draws something it was not sent**, which
  [ADR-0001](/docs/architecture/decisions/0001-device-renders-effects/)
  never ruled out: it is not a frame from anywhere, and it is drawn exactly
  when nothing can be received.
- **The counts are an interface now.** They are in the README and on the
  docs site, and a person will learn them. They are not renumbered.
- **The cover screen and the beacon are not changed.** They share the
  stand's WiFi code and not this: the screen has a screen, and the beacon
  has nothing to blink.

## What would settle it

Tested on the host: the opening and closing as rows of pixels over time, a
dim scene's bright edge, one overtaking the other; which fault each state of
the network is, the grace and
what restarts it, the number of blinks counted the way a person would, and
that only the first LED is touched. The firmware builds with IDF 6.1.

Not yet shown, on the stand:

- [ ] Power on with everything working: dark, no blink, then its scene.
- [ ] Two blinks, with the access point out of reach.
- [ ] Three, with the broker stopped. Four, with a wrong password. One, with
  the `creds` partition erased.
- [ ] The turntable switched on and off: the opening and the closing, and
  whether they are the right length and the right brightness to live with.
- [ ] The network lost during a record: the record's colors stay, and the
  first LED blinks.
