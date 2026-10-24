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
was decided, and they are not rewritten afterward. The
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

EdS runs on Deevnet IoTaaS, which keeps its own ADRs, change records
and incidents under the same names. To keep the two apart:

- A bare **ADR-0006**, **CHG-0001** or **INC-0001** anywhere in this
  repository means an **EdS** record, on this site.
- A Deevnet IoTaaS record is always written **Deevnet ADR-0012**, and links to
  [deevnet-docs](https://deevnet.github.io/deevnet-docs/docs/architecture/decisions/).

## Records

| ADR | Date | Decision | Status |
|---|---|---|---|
| [0001](0001-device-renders-effects/) | 2026-09-07 | The stand renders effects locally and never receives frames | {{< adr-status "Accepted" >}} |
| [0002](0002-gamma-in-firmware/) | 2026-09-07 | Gamma belongs to the firmware, not the wire | {{< adr-status "Accepted" >}} |
| [0003](0003-broker-is-the-substrates/) | 2026-09-07 | The MQTT broker is Deevnet IoTaaS's, not a tenant VM | {{< adr-status "Accepted" >}} |
| [0004](0004-firmware-secrets-nvs-only/) | 2026-10-10 | Firmware secrets are NVS-only, and the broker's CA is provisioned | {{< adr-status "Accepted" >}} |
| [0005](0005-device-events-over-mqtt/) | 2026-10-10 | A device reports key events over MQTT, not its serial log | {{< adr-status "Accepted" >}} |
| [0006](0006-now-playing-bus-of-drivers/) | 2026-10-10 | Now playing is a bus of small drivers behind one contract | {{< adr-status "Accepted" >}} |
| [0007](0007-drivers-are-portable-go/) | 2026-10-10 | Drivers are portable Go, in one agent binary | {{< adr-status "Accepted" >}} |
| [0008](0008-nowplayd-arbitrates-and-enriches/) | 2026-10-10 | nowplayd picks the current track and guarantees title, album and art | {{< adr-status "Accepted" >}} |
| [0009](0009-lightd-takes-covers-from-a-topic/) | 2026-10-10 | lightd takes covers from a topic as a second input | {{< adr-status "Accepted" >}} |
| [0010](0010-home-lan-relay/) | 2026-10-10 | Home-LAN drivers reach the broker through a dual-homed mobile Pi | {{< adr-status "Accepted" >}} |
| [0011](0011-itunes-by-remote-pairing/) | 2026-10-10 | iTunes is read by remote pairing, the way the phone remote does it | {{< adr-status "Accepted" >}} |
| [0012](0012-spotify-by-web-api/) | 2026-10-10 | Spotify is read from the account, through the Web API | {{< adr-status "Accepted" >}} |
| [0013](0013-musicbee-by-remote-protocol/) | 2026-10-10 | MusicBee is read through the MusicBee Remote plugin's protocol | {{< adr-status "Proposed" >}} |
| [0014](0014-tidal-by-lastfm/) | 2026-10-10 | TIDAL is read through Last.fm, because nothing better is offered | {{< adr-status "Proposed" >}} |
| [0015](0015-eds-builds-its-own-pi-images/) | 2026-10-10 | EdS builds its own Pi images, on Deevnet IoTaaS's base; a relay is an agent Pi with a second leg | {{< adr-status "Accepted" >}} |
| [0016](0016-a-source-can-be-reported-without-its-cover/) | 2026-10-11 | A source can be reported without its cover, and Spotify is: known, and not lit | {{< adr-status "Accepted" >}} |
| [0017](0017-an-agent-pi-is-a-registered-device/) | 2026-10-11 | An agent Pi is a registered device, and reports its events like one | {{< adr-status "Accepted" >}} |
| [0018](0018-a-console-of-eds-own/) | 2026-10-11 | EdS has a console of its own, fed from the bus, and it only looks | {{< adr-status "Accepted" >}} |
| [0019](0019-an-album-can-have-a-look-of-its-own/) | 2026-10-11 | An album can have a look of its own, named on the music side and drawn on the light side | {{< adr-status "Proposed" >}} |
| [0020](0020-a-touch-screen-that-can-change-what-plays/) | 2026-10-11 | A touch screen shows the cover and can change what plays, by asking nowplayd | {{< adr-status "Proposed" >}} |
| [0021](0021-the-turntable-is-identified-by-listening/) | 2026-10-11 | The turntable is identified by listening, with a recognizer that can be replaced | {{< adr-status "Proposed" >}} |
| [0022](0022-the-owners-collection-names-the-record/) | 2026-10-12 | For a source that plays the owner's records, the owner's collection names the record | {{< adr-status "Proposed" >}} |
| [0023](0023-a-source-that-is-switched-on-holds-the-stand/) | 2026-10-12 | A source's equipment can say it is on, and a source that is on holds the stand | {{< adr-status "Proposed" >}} |
| [0024](0024-paused-long-enough-is-not-playing/) | 2026-10-12 | A track left paused is not playing for ever, and a room with nothing playing is dim | {{< adr-status "Proposed" >}} |
| [0025](0025-current-says-when-there-are-no-controls/) | 2026-10-12 | The current track says when there is nothing a controller could do | {{< adr-status "Proposed" >}} |
| [0026](0026-what-silence-means-on-the-turntable/) | 2026-10-12 | A gap between tracks is listened for, and a silent turntable is idle in twenty seconds | {{< adr-status "Proposed" >}} |
| [0027](0027-what-is-asked-of-outside-services-is-counted/) | 2026-10-12 | What EdS asks of other people's services is counted, a minute at a time, and nowplayd writes its counts to the log store itself | {{< adr-status "Proposed" >}} |
| [0028](0028-the-stand-is-for-records/) | 2026-10-13 | The stand is lit for records only, and is dim only while the turntable is on | {{< adr-status "Proposed" >}} |
| [0029](0029-the-stand-starts-dark-and-blinks-what-is-wrong/) | 2026-10-13 | The stand starts dark, and says in red blinks how far it got | {{< adr-status "Proposed" >}} |
| [0030](0030-a-piece-of-music-can-have-a-look-from-any-source/) | 2026-10-13 | One piece of music can have a look of its own, shown whatever plays it, and the stand learns to scan | {{< adr-status "Proposed" >}} |
| [0031](0031-spotify-is-shown-like-itunes/) | 2026-10-13 | Spotify is reported with its cover, and treated as iTunes is | {{< adr-status "Proposed" >}} |
| [0032](0032-spotify-takes-commands/) | 2026-10-13 | The Spotify driver carries out commands, with a wider authorization, for what is playing here | {{< adr-status "Proposed" >}} |
| [0033](0033-the-whole-back-end-on-one-pi/) | 2026-10-13 | The whole back end can run on one Pi, as an image built on Deevnet's take-home one | {{< adr-status "Proposed" >}} |
| [0034](0034-presence-is-heard-not-remembered/) | 2026-10-16 | Presence is heard, not remembered: everything says it is there every thirty seconds | {{< adr-status "Proposed" >}} |
| [0035](0035-firmware-is-updated-through-the-broker/) | 2026-10-16 | Firmware is updated through the broker, and a build that cannot reach it is undone | {{< adr-status "Accepted" >}} |
| [0036](0036-the-screen-sleeps-when-nothing-plays/) | 2026-10-17 | The cover screen sleeps when nothing has played for five minutes, and wakes for music | {{< adr-status "Accepted" >}} |
| [0037](0037-hand-kept-secrets-are-encrypted-in-the-repository/) | 2026-10-18 | The secrets nothing else can issue are kept encrypted in the repository, with sops and age | {{< adr-status "Proposed" >}} |
| [0038](0038-the-turntable-names-the-original-album-by-what-is-on-it/) | 2026-10-19 | The turntable's album is the one the track is on, and the original one, not a time since the last | {{< adr-status "Proposed" >}} |
| [0039](0039-the-console-can-light-the-stand-for-every-source/) | 2026-10-19 | The console can light the stand for every source, a retained setting that stays until switched back | {{< adr-status "Proposed" >}} |
| [0040](0040-a-request-can-name-an-album-to-play/) | 2026-10-23 | A request can name an album to play, and iTunes plays it from its own library | {{< adr-status "Proposed" >}} |
| [0041](0041-an-artists-nth-album-is-counted-in-musicbrainz/) | 2026-10-24 | An artist's first album is their first studio album, counted in MusicBrainz | {{< adr-status "Proposed" >}} |
