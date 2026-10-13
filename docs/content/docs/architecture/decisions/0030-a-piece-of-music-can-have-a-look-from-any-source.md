---
title: "ADR-0030: A Piece of Music Can Have a Look, From Any Source"
weight: 30
---

# ADR-0030: One Piece of Music Can Have a Look of Its Own, Shown Whatever Plays It, and the Stand Learns to Scan

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until it has been seen on the stand with the theme playing; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-13 |
| **Scope** | A look for a single track, known by its title; the one exception to the stand being lit for records only; and a sixth effect in the firmware to draw it with. |
| **Extends** | [ADR-0019](/docs/architecture/decisions/0019-an-album-can-have-a-look-of-its-own/), whose looks were albums'. [ADR-0028](/docs/architecture/decisions/0028-the-stand-is-for-records/), whose dark sources now have one thing that lights them. [ADR-0001](/docs/architecture/decisions/0001-device-renders-effects/): the stand has one more effect. |
| **Related** | [ADR-0029](/docs/architecture/decisions/0029-the-stand-starts-dark-and-blinks-what-is-wrong/), whose opening and closing are how this comes on and goes off in a dark room |

---

## Context

Its owner wants the stand to do what the front of KITT does while the theme
from *Knight Rider* is playing: one red light, going from side to side.

Three things are in the way.

**No effect does it.** `sweep` and `chase` go around, and come back in at
the other end. A scanner turns around.

**Looks belong to albums**
([ADR-0019](/docs/architecture/decisions/0019-an-album-can-have-a-look-of-its-own/)).
The theme is one track, on a soundtrack whose other tracks are not it, and
on any number of compilations of television themes.

**The stand is lit for records only**
([ADR-0028](/docs/architecture/decisions/0028-the-stand-is-for-records/)),
and the theme is as likely to be played from iTunes.

## Options considered

**For the effect: send frames.** Ruled out for every effect, long ago
([ADR-0001](/docs/architecture/decisions/0001-device-renders-effects/)).

**For the effect: `chase` with a black ground and one red accent.** It
laps; it does not turn around, and has no tail.

**For the effect: a parameter on `sweep`.** A bounce and a tail are not what
`sweep` is, and the scene descriptor would grow a field one effect reads.

**For the effect: a new one, `scanner`.** A reflash of the stand, which is
the cost every effect has had.

**For the match: the album.** Lights the whole soundtrack, and misses every
compilation.

**For the match: the recording** - an identifier from a catalog. Exact, and
there are dozens of recordings of it, and the turntable's recognizer gives
a title and an artist and no identifier.

**For the match: the title alone.**

**For iTunes: leave it dark, as decided that morning.** Consistent, and the
thing asked for would then be seen only by someone who owns the record.

**For iTunes: let any look through.** *The Dark Side of the Moon* from
iTunes would light the stand again, which ADR-0028 chose against by name.

**For iTunes: say, look by look, which are for any source.**

## Decision

**A track whose title is a known piece of music has that piece's look,
before any look its album has. A look can be marked as being for any source,
and then a dark source's cover is published with it and not with `dark`. The
one such piece is the theme from *Knight Rider*, whose look is `scanner`:
the firmware's new effect of that name, in red.**

### The match, in nowplayd

- **By title, and by nothing else.** The artist is not asked: it is the
  same piece whoever recorded it.
- **The title has to be the piece's and no more.** Lower case, without what
  is in parentheses, without punctuation, and without the words that only
  say it is a theme - `theme`, `from`, `main`, `title`, `tv`, `the`,
  `original`, `version`, `opening`, `music` - what is left must be exactly
  `knight rider`. "Theme from Knight Rider", "Knight Rider (Main Title)" and
  "KNIGHT RIDER: TV THEME" are it. "KITT Jumps", on the same soundtrack, is
  not, and nor is "My Knight Rider".
- **The look follows the track**, on a cover that does not change. The look
  is published again when the next track has a different one, with the same
  cover's hash, and `lightd` relights from the pair. A soundtrack on the
  turntable scans for the theme and goes back to its sleeve's colors.
- **Any source**: `looks.AnySource` says which looks are, and the publisher
  asks it before giving a dark source's cover the look `dark`. Everything
  else from a dark source is as dark as it was.

### The look, in lightd

- `scanner` is a scene like `spectrum`: effect `scanner`, one color,
  `255, 0, 0`.
- **Its speed and brightness are its own**, 0.9 and 1, where `spectrum`
  takes the stand's. It is there and back in a second and a half, which is
  the thing itself, and it is one point of light with a dark strip around
  it.
- lightd knows a name and a red light. It does not know a car.

### The effect, in the firmware

- **`scanner`: one eye of the dominant color, going from one end of the
  strip to the other and back in one cycle**, with what it has just passed
  fading behind it, and the rest of the strip dark. It uses the first color
  of the palette and no other.
- The eye is a fifth of the strip wide, soft-edged, three or four LEDs of
  eighteen. The tail is the eye's own last eight positions, each dimmer.
- A stand that does not know `scanner` shows it as `solid`, as with any
  name it does not know: a red strip. So the stand is flashed first.
- The console's port (`effects.js`) has it too, held to the firmware's
  pixels like the others.

## Consequences

- **With the turntable off and the theme played from iTunes, the stand
  comes on, scans, and goes off again**: dark to lit is the opening of
  ADR-0029, and lit to dark its closing. Nothing has to be switched on.
- **With the turntable on, iTunes is not current**
  ([ADR-0023](/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand/)),
  and the theme played from it is not seen. From a record, it is.
- **A look needs a cover to travel with.** A track for which no cover is
  found has no look. The catalogs find one for nearly everything.
- **The turntable has to name the track** to scan for it: thirty seconds or
  so into a theme that lasts about a minute, unless it follows a gap
  ([ADR-0026](/docs/architecture/decisions/0026-what-silence-means-on-the-turntable/)).
- **Some other track called exactly "Knight Rider" will scan.** Accepted:
  the title is not a common one.
- **"Records only" has an exception, and a place where the next is
  written.** One line in `internal/looks`, and a decision each time.
- **Six effects.** Each one is a reflash, a port to the console, and a row
  in the reference. That is still few.
- **Spotify is unchanged.** It publishes no cover
  ([ADR-0016](/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover/)),
  so it has no look.

## What would settle it

Tested: the titles that are the theme and the ones that are not; a dark
source's cover published with `scanner`, and with `dark` for the track
after; lightd's scene for the name; the effect on the host, as where the eye
is over a cycle, its tail, and the dark around it; the console's port
against pixels printed from the firmware's own code.

Not yet shown:

- [ ] The theme from iTunes, turntable off: the opening, the scanner, the
  closing.
- [ ] Whether a second and a half, and three or four LEDs, look like the
  thing. Both are constants.
- [ ] The theme from a record.
