---
title: "ADR-0016: A Source Can Be Reported Without Its Cover"
weight: 16
---

# ADR-0016: A Source Can Be Reported Without Its Cover, and Spotify Is

| | |
|---|---|
| **Status** | {{< adr-status "Accepted" >}} |
| **Date** | 2026-10-11 |
| **Scope** | Whether everything that is reported as playing also lights the stand. |
| **Extends** | [ADR-0008](/docs/architecture/decisions/0008-nowplayd-arbitrates-and-enriches/), which promised that `current` carries a cover whenever something is playing |
| **Extended by** | [ADR-0028](/docs/architecture/decisions/0028-the-stand-is-for-records/): a source can also be reported with its cover and still not lit, by a look that says so. [ADR-0031](/docs/architecture/decisions/0031-spotify-is-shown-like-itunes/): Spotify is no longer reported this way. It has its cover, and is kept off the stand as iTunes is; the setting remains, with no source in it |
| **Related** | [ADR-0009](/docs/architecture/decisions/0009-lightd-takes-covers-from-a-topic/), [ADR-0012](/docs/architecture/decisions/0012-spotify-by-web-api/) |

---

## Context

The stand was built to stand beside a record that is playing. iTunes plays a
library of albums the owner chose, and the stand follows it.

Spotify is a different kind of listening, and the owner decided the lights
should not follow it. It should still be *known*: the next thing wanted from
Spotify is a voice asking for an album, and whatever answers "what is
playing?" or starts playback will need the same report.

There is also a reason that is not a preference.
[ADR-0012](/docs/architecture/decisions/0012-spotify-by-web-api/) recorded a
line from Spotify's API pages about not synchronizing recordings with visual
media, and left it as the owner's judgment. Not lighting the stand for
Spotify, and never fetching its artwork, steps around that question.

`lightd` reacts to one thing: a cover arriving on `nowplaying/current/art`
([ADR-0009](/docs/architecture/decisions/0009-lightd-takes-covers-from-a-topic/)).
It does not know which source a cover came from, and should not.

## Options considered

**Leave Spotify out of arbitration.** Simplest: `nowplayd` would ignore the
source. But then `current` would name a paused iTunes track while Spotify
plays, and nothing downstream could ever know otherwise.

**Teach `lightd` which sources to follow.** It would have to read `current`
as well as the cover, and learn what a source is. That is the knowledge
ADR-0009 kept out of it.

**Publish no cover for the source.** `current` tells the truth about what is
playing; the cover topic, which is the only thing the stand follows, is left
alone.

## Decision

A source can be named as **coverless** (`NP_COVERLESS_SOURCES` on `nowplayd`).
When such a source is current, `nowplayd`:

- publishes `current` with the track and **no `art`**;
- publishes **nothing** to `current/art`, which keeps whatever it last held;
- does **not look a cover up** in a catalog. A catalog's cover would light
  the stand exactly as the player's own would.

Spotify is named. Its driver goes further and never asks Spotify for an image
at all, so there is no artwork to mishandle anywhere in the path.

## Consequences

- **ADR-0008's promise now has an exception.** A subscriber to `current` may
  see a playing track with no `art`. The contract always allowed `art` to be
  absent; what changes is that it can now be absent on purpose.
- While Spotify plays, the stand holds the last cover it was given. It shows
  a record that is not the one playing. That is the decision, and it will
  look like a fault to anyone who has not read this.
- `current/art` and `current` can disagree: the cover is the last *lit* one,
  not the current track's. A subscriber that wants the current track's cover
  must check that `current.art.sha256` exists and matches, as the contract
  already said to.
- **It rests on one setting.** Without `NP_COVERLESS_SOURCES=spotify` on the
  workload, `nowplayd` would look Spotify's tracks up and the stand would
  follow them. The setting is committed with the deployment, and the check
  that proves it is in
  [CHG-0004](/docs/changes/2026/0004-spotify-on-the-agent-pi/).
- The relay's broker account is granted Spotify's `art` topic although nothing
  publishes to it, so that reversing this is a setting and not also a change
  of account.
- Reversing it is small: remove the setting, and the stand follows Spotify
  through catalog covers. Using Spotify's own artwork would be a further
  decision, with the terms question to answer first.
