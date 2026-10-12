---
title: "ADR-0019: An Album Can Have a Look of Its Own"
weight: 19
---

# ADR-0019: An Album Can Have a Look of Its Own, Named on the Music Side and Drawn on the Light Side

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until the first one has been seen on the stand |
| **Date** | 2026-10-11 |
| **Scope** | How one particular album gets a scene that is not derived from its cover, without the part that makes scenes learning what an album is. |
| **Extends** | [ADR-0009](/docs/architecture/decisions/0009-lightd-takes-covers-from-a-topic/), which gave `lightd` covers and kept tracks and albums on the other side of the topic |
| **Related** | [ADR-0008](/docs/architecture/decisions/0008-nowplayd-arbitrates-and-enriches/), [ADR-0001](/docs/architecture/decisions/0001-device-renders-effects/) |

---

## Context

Every scene so far is made the same way: the cover's colors, picked by
`palette`, turned into a scene by `lightd`. That is right for almost every
record and wrong for a few. *The Dark Side of the Moon* is a black sleeve with
a thin spectrum across it. The colors a fair extraction finds on it are not
what anyone would light a room with for that album, and what they would light
it with is obvious: the spectrum.

That is a fact about an album, not about an image. And the two halves of the
system were built not to share such facts: `lightd` sees image bytes and
knows nothing of tracks or albums
([ADR-0009](/docs/architecture/decisions/0009-lightd-takes-covers-from-a-topic/)),
and `nowplayd` knows what is playing and nothing of scenes, colors or LEDs.

## Options considered

**`lightd` reads the current track and has a list of albums.** One place to
change. It is exactly what ADR-0009 kept out: `lightd` would know what an
album is, and every future special case would be argued there.

**Recognize the cover image.** `lightd` stays image-only. But the same album
arrives as different bytes from every library, edition and catalog, so it
means fuzzy image matching to answer a question the track's own tags answer
outright.

**A third program that publishes scenes for special albums.** It and `lightd`
would both publish to the stand's scene topic for the same track change, and
which won would be a race.

**The music side names a look; the light side draws it.** `nowplayd` knows
the album and says, in one word, that this cover has a look. `lightd` knows
what that word looks like. Neither learns the other's subject.

## Decision

**A look is a name.** `nowplayd` holds a short list of albums that have one,
matched on the artist and album it already knows. When the current track's
album is on it, `nowplayd` publishes the look's name on
`nowplaying/current/look`, retained, beside the cover.

**A look belongs to a cover, by its hash.** The message carries the SHA-256 of
the cover it is for. `lightd` applies a look only to the cover with that hash,
so the two messages can arrive in either order and a look can never land on
the album after it. An album with no look has a message that says so for its
cover.

**`lightd` owns what a look is.** A look is a scene preset: an effect and a
palette, in `lightd`, beside the other decisions about color and hardware. A
cover with a look gets the preset instead of its own colors. A look `lightd`
does not know is ignored, and the cover is lit from its own colors, as the
stand shows an effect it does not know as `solid`.

**The first look is `spectrum`**: six colors, red to violet, in equal
shares. It is drawn with an effect the stand already has. Which one is a
setting: `sweep` moves the whole spectrum around the strip, `morph` shows each
color in turn.

**No look reaches a stand as anything but a scene.** The stand and the scene
contract are unchanged.

## Consequences

- Dark Side of the Moon lights the room as a spectrum, from any library and
  any edition whose tags say what it is.
- `lightd` still knows nothing of tracks or albums. It knows covers, and now
  that a cover may come with a word.
- **The list of special albums is in `nowplayd`'s code.** Adding one is a
  change and a deployment, and there should not be many: this is for records
  whose look is as well known as their name.
- **Matching is on tags.** A copy tagged *Dark Side Of The Moon (2011
  Remaster)* matches; one tagged *DSOTM* does not, and is lit from its cover
  like any other.
- A look is not applied to a source that is reported without its cover
  ([ADR-0016](/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover/)).
  No cover is published for it, so there is nothing for a look to belong to.
  Spotify playing this album changes nothing in the room.
- At start-up, when both retained messages arrive at once and the cover is
  first, the stand may show the cover's own colors for a moment before the
  look.
- Two more grants: `nowplayd` publishes the look topic and `lightd` reads it.
- The colors of a look are fixed, not measured from anything. They are still
  `lightd`'s to choose, and are sent to the stand before gamma like any
  others.
