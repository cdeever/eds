---
title: "ADR-0040: A Request Can Name an Album to Play"
weight: 40
---

# ADR-0040: A Request Can Name an Album to Play, and iTunes Plays It From Its Own Library

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until `play_album` sent to `nowplaying/command` has played an album through nowplayd and the driver (the driver's half is shown); see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-23 |
| **Scope** | A sixth command, `play_album`, that names an album rather than pressing a button; which player carries it out; and what the iTunes driver asks of the player's library to do it. The first thing to send it is meant to be a voice ("Hey Ed, play Van Halen's first album"), which is a decision of its own. |
| **Extends** | [ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/), whose five commands press buttons on whatever is playing, and [ADR-0011](/docs/architecture/decisions/0011-itunes-by-remote-pairing/), whose pairing has only read what plays and pressed those buttons |
| **Related** | [ADR-0032](/docs/architecture/decisions/0032-spotify-takes-commands/), which added no command; [ADR-0038](/docs/architecture/decisions/0038-the-turntable-names-the-original-album-by-what-is-on-it/), which already knows an artist's original albums |

---

## Context

Every command so far is a button. It acts on whatever is playing, so a
controller never needs to say where it is sent, and
[ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/)
made that a rule: a controller never names a source. A request to play an
album is not a button. Nothing may be playing at all, and if something is,
it may be the turntable, which takes no command.

The phone remote that iTunes pairs with
([ADR-0011](/docs/architecture/decisions/0011-itunes-by-remote-pairing/))
can do this: it queues an album and plays it. The pairing EdS already holds
is the same credential, so nothing new is granted by the player. What the
phone also does, browsing the library, iTunes for Windows refuses this
remote: `databases` and `update` answer 403 on a session whose buttons work.

## Options considered

**Leave the five commands as they are and send the request as a search**
(type the words into the player's search). iTunes has no such request over
DACP, and a search answers with tracks, not an album in its order.

**Play from an outside catalog**, such as Spotify's, which can play any
album by name. It is not the owner's library, it needs the authorization
widened again ([ADR-0032](/docs/architecture/decisions/0032-spotify-takes-commands/)
asked for no more than control), and Spotify is switched off (CHG-0024).

**A sixth command that names an album, carried out by iTunes from its own
library.** The player is asked to play the songs whose artist and album have
those names, from the first track, in the album's order. Looking the album
up first, and playing it by its id, was the plan until the player refused
the lookup.

## Decision

**`play_album` names an album, and the iTunes driver plays it from the
library it is paired with. It is the one command that is not sent to the
current source.**

- **The command** is
  `{"v":1,"command":"play_album","artist":"Van Halen","album":"Van Halen","from":"…"}`
  on `nowplaying/command`, never retained, as the others. It names the
  album in words. Which album that is in the library is the driver's to
  find, because only the driver can ask the library.
- **It goes to the player that has a library**, `NP_ALBUM_SOURCE`
  (default `itunes`), whatever is playing. This is the exception to "a
  controller never names a source": it still names none, and `nowplayd`
  chooses the one source that can carry it out.
- **The player matches the names, and what it then plays is the proof.**
  The library cannot be asked first, and the player answers a request that
  matched nothing as it answers one that matched. So the driver asks what is
  playing afterwards, and a request whose artist and album are not what
  plays is logged as not found. It never guesses a nearer name, so a
  misheard request is not answered with the wrong record.
- **Playing replaces what plays.** The queue is emptied and the album
  queued from its first track, sorted as the album is. This is what the
  phone does.
- **Which album "the first album" is**, and turning words into the command,
  is not the driver's and is not decided here.

The client sends two requests it has not sent before, as the phone remote
sends them: `ctrl-int/1/cue?command=clear`, then
`cue?command=play&query='daap.songartist:<artist>'+'daap.songalbum:<album>'&index=0&sort=album`.
A name's apostrophes and plus signs are escaped inside its quotes.
`npagent play-album ARTIST ALBUM` sends them by hand.

## Consequences

- **EdS can choose what plays**, where it could only press buttons on what
  was already playing. The pairing allowed this all along; now it is used.
- **A request reaches iTunes while a record is on the turntable.** iTunes
  then plays over it. Whether a request should wait while the turntable's
  beacon is on is left to whatever sends it.
- **Names have to match the library's, and EdS cannot read the library to
  find out what they are.** "Van Halen" finds Van Halen; "Van Halen I" does
  not. Making a person's words into the library's names is the sender's job,
  from what it knows elsewhere (a catalog, the Discogs collection), and a
  miss is only learned by trying.
- **The match is on the track's artist.** An album whose tracks name guests
  ("Santana feat. …") is found only in part, or not at all.
- **The broker account of whatever sends `play_album`** needs
  `nowplaying/command`, as the cover screen has. The agent's already reads
  its source's command topic.

## What would settle it

Tested against a stand-in for the player: the two requests as the remote
sends them, a name's apostrophe or plus sign kept inside its quotes, and
nothing sent without both names.

Shown on 2026-10-23, from the relay Pi against the real iTunes for Windows:

- [x] `databases` and `update` answer 403 to this remote, with or without
      `Viewer-Only-Client`; without it, the buttons fail too (501). The
      library cannot be listed.
- [x] `npagent play-album "Van Halen" "Van Halen"` starts *Van Halen*
      (1978) at "Runnin' With The Devil", and the player says so.

Not yet shown:

- [ ] What the player does when nothing matches: whether it stops, or plays
      on from where it was.
- [ ] A name with an apostrophe (*Guns N' Roses*), and an album whose
      tracks name guests.
- [x] `play_album` on the bus, carried out by the driver
      ([CHG-0033](/docs/changes/2026/0033-an-album-asked-for-on-the-bus/)):
      *Aerosmith* by Aerosmith, from "Make It", within a second.
- [ ] `play_album` on `nowplaying/command`, passed on by nowplayd from a
      sender with an account of its own.
