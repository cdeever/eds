---
title: "Decisions"
weight: 90
bookCollapseSection: true
---

# Architecture Decision Records

The design forks: where more than one option was viable, the reasoning that
picked one, and what was accepted in doing so. Each record is the durable
answer to "why is it built this way, and what did we turn down?"

**Read this index before reopening a question.** If a record already answers
it, the answer stands until a new record changes it. That is the point of the
section: a decision argued once should not have to be argued again from
memory.

Records are **point-in-time**. They say what was decided and why on the day it
was decided, and they are not rewritten afterwards. The
[architecture](/docs/architecture/) and [reference](/docs/reference/) pages are
kept current; the decision log is kept honest.

## When a decision changes

Two different things happen to an old record, and they are not the same:

- **Extended.** The record was right but incomplete: building the thing
  exposed a question it never asked. A new record answers that question, the
  old one stays `Accepted`, and each links to the other.
- **Superseded.** Acting on the record would now be a mistake. A new record
  replaces it, the old one is marked `Superseded` and links forward.

The test is whether the earlier record is still safe to act on. If yes, extend
it. If not, supersede it.

## Format

| Field | Purpose |
|---|---|
| **Status** | `Proposed`, `Accepted`, `Superseded` or `Deprecated` |
| **Date** | The day the decision was made, not the day it was written up |
| **Context** | The situation and the goals that forced a choice |
| **Options considered** | The viable alternatives, each with what it would cost |
| **Decision** | What was chosen |
| **Consequences** | What it commits us to, good and bad |

- Records are numbered in the order they are **opened** and never renumbered.
  The number is claimed when the question is written down, not when it is
  answered.
- A record sits at `Proposed` while it rests on something not yet proven: a
  protocol not yet tried against the real player, a limit not yet measured.
  The evidence is added to the record when it arrives, and the status moves
  with it.
- Some early records are **backfilled**: the decision was made and argued in
  a README before this section existed. They carry the date of the decision
  and say that they are backfilled.

## EdS records and Deevnet records

EdS runs on the Deevnet substrate, which keeps its own ADRs, change records
and incidents under the same names. To keep the two apart:

- A bare **ADR-0006**, **CHG-0001** or **INC-0001** anywhere in this
  repository means an **EdS** record, on this site.
- A substrate record is always written **Deevnet ADR-0012**, and links to
  [deevnet-docs](https://deevnet.github.io/deevnet-docs/docs/architecture/decisions/).

## Records

| ADR | Date | Decision | Status |
|---|---|---|---|
| [0001](0001-device-renders-effects/) | 2026-09-07 | The stand renders effects locally and never receives frames | {{< adr-status "Accepted" >}} |
| [0002](0002-gamma-in-firmware/) | 2026-09-07 | Gamma belongs to the firmware, not the wire | {{< adr-status "Accepted" >}} |
| [0003](0003-broker-is-the-substrates/) | 2026-09-07 | The MQTT broker is the substrate's, not a tenant VM | {{< adr-status "Accepted" >}} |
| [0004](0004-firmware-secrets-nvs-only/) | 2026-10-10 | Firmware secrets are NVS-only, and the broker's CA is provisioned | {{< adr-status "Accepted" >}} |
| [0005](0005-device-events-over-mqtt/) | 2026-10-10 | A device reports key events over MQTT, not its serial log | {{< adr-status "Accepted" >}} |
| [0006](0006-now-playing-bus-of-drivers/) | 2026-10-10 | Now playing is a bus of small drivers behind one contract | {{< adr-status "Accepted" >}} |
| [0007](0007-drivers-are-portable-go/) | 2026-10-10 | Drivers are portable Go, in one agent binary | {{< adr-status "Accepted" >}} |
| [0008](0008-nowplayd-arbitrates-and-enriches/) | 2026-10-10 | nowplayd picks the current track and guarantees title, album and art | {{< adr-status "Accepted" >}} |
| [0009](0009-lightd-takes-covers-from-a-topic/) | 2026-10-10 | lightd takes covers from a topic as a second input | {{< adr-status "Accepted" >}} |
| [0010](0010-home-lan-relay/) | 2026-10-10 | Home-LAN drivers reach the broker through a dual-homed mobile Pi | {{< adr-status "Accepted" >}} |
| [0011](0011-itunes-by-remote-pairing/) | 2026-10-10 | iTunes is read by remote pairing, the way the phone remote does it | {{< adr-status "Accepted" >}} |
| [0012](0012-spotify-by-web-api/) | 2026-10-10 | Spotify is read from the account, through the Web API | {{< adr-status "Proposed" >}} |
| [0013](0013-musicbee-by-remote-protocol/) | 2026-10-10 | MusicBee is read through the MusicBee Remote plugin's protocol | {{< adr-status "Proposed" >}} |
| [0014](0014-tidal-by-lastfm/) | 2026-10-10 | TIDAL is read through Last.fm, because nothing better is offered | {{< adr-status "Proposed" >}} |
| [0015](0015-eds-builds-its-own-pi-images/) | 2026-10-10 | EdS builds its own Pi images, on the substrate's base; a relay is an agent Pi with a second leg | {{< adr-status "Accepted" >}} |
