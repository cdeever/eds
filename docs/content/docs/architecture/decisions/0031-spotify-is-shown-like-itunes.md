---
title: "ADR-0031: Spotify Is Shown, Like iTunes"
weight: 31
---

# ADR-0031: Spotify Is Reported With Its Cover, and Treated as iTunes Is

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until the driver has fetched a cover from the real account and the screen has shown it; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-13 |
| **Scope** | Whether Spotify's track comes with a cover, where the cover is from, and what the stand does for it. |
| **Extends** | [ADR-0012](/docs/architecture/decisions/0012-spotify-by-web-api/), whose driver took no artwork and now takes the album's. [ADR-0016](/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover/): a source can still be reported without its cover, and Spotify no longer is. [ADR-0028](/docs/architecture/decisions/0028-the-stand-is-for-records/): Spotify becomes a dark source, as iTunes is. |
| **Related** | [ADR-0030](/docs/architecture/decisions/0030-a-piece-of-music-can-have-a-look-from-any-source/), whose "any source" this makes true of Spotify; [ADR-0027](/docs/architecture/decisions/0027-what-is-asked-of-outside-services-is-counted/), which counts the new requests; [ADR-0018](/docs/architecture/decisions/0018-a-console-of-eds-own/), whose page shows the cover |

---

## Context

[ADR-0016](/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover/)
reported Spotify without a cover for two reasons. The owner did not want
the lights to follow it. And Spotify's developer pages have a line about
not synchronizing its recordings with visual media, which
[ADR-0012](/docs/architecture/decisions/0012-spotify-by-web-api/) left to
the owner's judgment; fetching no artwork and lighting nothing stepped
around it.

Both have moved.

**The first has another answer now.** A source can be shown and not lit
([ADR-0028](/docs/architecture/decisions/0028-the-stand-is-for-records/)):
that is how iTunes is, since this morning. Leaving Spotify coverless is no
longer the only way to keep the stand off it, and it costs the cover screen
and the console their picture.

**The second has been judged**, by the owner, on 2026-10-13: Spotify gives
the album's pictures in the same answer as the track, to anyone it answers;
this is one person's own account, in their own house, shown to nobody else;
and that is within what the pictures are there for. The stand's one light
show for a Spotify track, the scanner
([ADR-0030](/docs/architecture/decisions/0030-a-piece-of-music-can-have-a-look-from-any-source/)),
is included in that judgment.

What prompted it: the theme from *Knight Rider* played from Spotify does
nothing, because a look travels with a cover and Spotify has none.

## Options considered

**Leave Spotify as it is.** Then "any source" in ADR-0030 means "iTunes".

**A cover for the one track with a look, from the catalogs.** Keeps Spotify's
artwork untouched and makes one track of a source behave unlike every other.

**Stop marking Spotify coverless, and let nowplayd look every cover up.** No
change to the driver. But the catalogs guess from a title and an artist, and
Spotify knows which album it is playing: the player's own cover is the right
one, which is the rule for every source
([ADR-0008](/docs/architecture/decisions/0008-nowplayd-arbitrates-and-enriches/)).

**The driver fetches the album's picture, as iTunes' does.**

## Decision

**The Spotify driver publishes the album's picture as the track's cover.
nowplayd treats Spotify as it treats iTunes: shown, and dark, except for a
look that is for any source.**

- **The picture is the one Spotify names** in `item.album.images`, in the
  answer the driver already asks for. The widest that is not over 640
  pixels is taken.
- **One request for each album**, not each track or each poll: the picture's
  address is the album's.
- **The account's token does not go with it.** The pictures are on another
  host, are public, and are asked for with no `Authorization` header.
- **The scopes do not change.** Reading playback was already enough to be
  told where the picture is.
- **A picture that cannot be had is no cover**, and nowplayd looks one up,
  as for any track without one.
- **Its requests are counted** as `spotify-images`, apart from the API's
  ([ADR-0027](/docs/architecture/decisions/0027-what-is-asked-of-outside-services-is-counted/)).
- **On the workload, `NP_COVERLESS_SOURCES` loses `spotify` and
  `NP_DARK_SOURCES` gains it.** The setting for a coverless source stays,
  with no source in it.
- **The driver is turned back on** on the agent Pi
  ([CHG-0011](/docs/changes/2026/0011-spotify-turned-off/) turned it off).
- **Spotify still takes no commands**
  ([ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/)).
  The cover screen shows its sleeve and buttons that do nothing; that is as
  it was, and is its own question.

## Consequences

- **The cover screen and the console show what Spotify is playing.**
- **The stand is dark for Spotify, and scans for the theme.**
- **Spotify can take the room from iTunes**, and from a paused anything, by
  the arbiter's ordinary rule. It always could; there is now something to
  see when it does.
- **Only named devices count, as before.** Spotify on a phone in the car
  shows nothing.
- **Spotify's pictures are held as every cover is**: one, in memory, on a
  retained topic until the next.
- **The owner's judgment about Spotify's terms is recorded here and is the
  owner's.** If EdS were ever shown to anyone else, or made for anyone
  else, it would have to be made again.
- **ADR-0016's mechanism has no user.** It is kept: the next source that
  should be known and not seen is a setting away.
- **"The stand does not follow Spotify" is still true**, for a different
  reason: it follows nothing but records.

## What would settle it

Tested against a stand-in for Spotify: one picture for each album, published
with the state that names it; no token sent to the pictures' host; a picture
that is not there; which size is taken.

Not yet shown:

- [ ] The driver on the Pi, against the account: a cover fetched, and
  `spotify-images` counted.
- [ ] The cover screen and the console showing it, the stand dark.
- [ ] The theme from Spotify: the scanner.
- [ ] That the authorization given on 2026-10-11 still stands. It ends six
  months after it was given.
