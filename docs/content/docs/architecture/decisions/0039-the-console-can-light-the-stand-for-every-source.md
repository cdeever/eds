---
title: "ADR-0039: The Console Can Light the Stand for Every Source"
weight: 39
---

# ADR-0039: The Console Can Light the Stand for Every Source, Until It Is Switched Back

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until it has been switched both ways from the console in the room; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-19 |
| **Scope** | A switch on the console that lights the stand from iTunes and Spotify as well as records; where its state is kept; who may change it. |
| **Extends** | [ADR-0018](/docs/architecture/decisions/0018-a-console-of-eds-own/): the console gets its first control, and its account its second topic to publish. [ADR-0028](/docs/architecture/decisions/0028-the-stand-is-for-records/): records-only stays the default, and becomes a setting that can be changed while running. |
| **Related** | [ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/), whose commands are never retained; this is not one of them. [ADR-0030](/docs/architecture/decisions/0030-a-piece-of-music-can-have-a-look-from-any-source/), whose any-source look is unchanged. [ADR-0009](/docs/architecture/decisions/0009-lightd-takes-covers-from-a-topic/), whose boundary this keeps. |

---

## Context

The stand is lit for records only
([ADR-0028](/docs/architecture/decisions/0028-the-stand-is-for-records/)):
`NP_DARK_SOURCES=itunes,spotify` makes nowplayd give those sources' covers
the look `dark`, and lightd draws it as a dark stand. That is right for the
room and stays the default.

It is in the way when working on the stand itself. Trying an effect, tuning
lightd's colors or watching how a cover comes out needs music on demand, and
iTunes or Spotify is the music that can be put on from a chair. Today that
means changing a setting in nowplayd's environment and restarting it, and
remembering to change it back.

The console is where a person already looks at the stand. It only looks
([ADR-0018](/docs/architecture/decisions/0018-a-console-of-eds-own/)), and
that record says controls would be a decision of their own: they change what
the console's account may publish, and who may reach the page. It also says
a page with no login is "not acceptable for a page with controls". This
record is that decision for one control.

## Options considered

**Leave it a setting.** Edit `NP_DARK_SOURCES` and restart nowplayd. Nothing
new, and nothing a person can change without a shell on the Pi.

**A command, like the screen's** (ADR-0020). The console publishes "all
sources" once; nowplayd holds it in memory. A restart of nowplayd forgets
it, and the console cannot know what nowplayd holds without being told
something back.

**The console keeps the state** and tells nowplayd. The console would hold
something nobody else can read, and one that restarts would have to be
asked again. It is a page, not a store.

**lightd decides**, lighting a `dark` look anyway while switched. lightd
would know that `dark` means a source, which is what
[ADR-0009](/docs/architecture/decisions/0009-lightd-takes-covers-from-a-topic/)
and ADR-0028 keep from it.

**A retained setting on the broker, read by nowplayd.** The broker already
keeps what must outlive a restart (scenes, presence, the current track). One
retained message is the state; whoever writes it last decides; nowplayd and
the console both read it, so the page shows what is in force rather than
what it last sent.

## Decision

**The stand's sources are a retained setting on the broker, which the
console can change and nowplayd obeys. It stays as set until it is set
again.**

- **The topic is `eds/nowplaying/stand`, retained**, holding
  `{"v":1,"sources":"records"}` or `{"v":1,"sources":"all"}`. A word, not a
  boolean, so a third choice is a new word and not a second flag.
- **`records` is ADR-0028**: the sources in `NP_DARK_SOURCES` get the look
  `dark`. **`all` treats that list as empty**: every source's cover lights
  the stand from its colors, and from an album's own look
  ([ADR-0019](/docs/architecture/decisions/0019-an-album-can-have-a-look-of-its-own/)).
  Nothing else in nowplayd changes: arbitration, the beacon's hold, idle and
  standby are as they were.
- **No message, an empty one, a wrong version or a word nowplayd does not
  know is `records`**, and nowplayd logs the last three. The default needs
  nothing on the broker.
- **It applies at once.** When the setting changes, nowplayd publishes the
  current cover's look again, so a track already playing lights or darkens
  then, not at the next track.
- **The console shows it and switches it.** It subscribes to the topic and
  draws the setting in force beside the stand: "Stand: records" or
  "Stand: all sources", the second marked so it is not missed. The switch
  publishes the other word, retained.
- **It stays on until it is switched off.** No timer and no reset at the
  next record. A setting for working on the stand that turned itself off
  would turn off mid-work; one that is left on is plain on the page that
  shows the stand.
- **It is a setting, not a command.** Commands
  ([ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/))
  are never retained, because a pause replayed later is wrong. A setting is
  retained because a setting forgotten is wrong. The two do not share
  topics.
- **The console's account may publish this one topic** besides its
  presence, and **nowplayd's may read it**. Both are changed in the tenant's
  `main.tf` and in `images/backend/files/accounts.conf`, as one list kept in
  two places.
- **The page's one route that changes anything is
  `POST /api/stand`**, with a JSON body. It refuses any other content type,
  and a request whose `Origin` is not the console's own, so another site
  open in the same browser cannot switch it. It publishes, and answers with
  what it published; the page shows the setting when the broker echoes it
  back, not before.
- **There is no login.** Anyone who can open the console can switch it. The
  console is reached only from the site's trusted networks and the home
  network, and this control changes which covers light a strip of LEDs and
  nothing else: no playback, no volume, nothing that leaves the house.
  ADR-0018's "not acceptable for a page with controls" was written for
  controls in general; this is accepted for this one. A control that does
  more is a new decision, and that question comes back with it.

## Consequences

- **The stand can be worked on with iTunes or Spotify** from a browser, and
  the room put back from the same place.
- **Left on, the stand lights for iTunes in an empty room**, as it did
  before ADR-0028. The console says so; nothing else does.
- **The console is no longer only a reader.** Its account can publish one
  topic, and its server has a route that is not `GET`. CLAUDE.md, the
  console's README and ADR-0018's "it only looks" are qualified to "it
  looks, and switches the stand's sources".
- **`NP_DARK_SOURCES` remains** and still says which sources are dark when
  the setting is `records`. Changing which sources those are is still a
  setting and a restart.
- **The setting outlives everything but the broker's store.** A broker that
  loses its retained messages (a new card) is back to `records`, which is
  the safe way to forget.
- **A nowplayd from before this ignores the topic**, and the console's
  switch does nothing visible. They are deployed together.
- lightd, the firmware and the cover screen do not change. lightd is sent
  covers and looks as it always was; it does not know there is a setting.

## What would settle it

To be tested in nowplayd (the setting's words, an unknown one, the look
published again on a change, both orders of setting and track) and in the
console (the route refuses a wrong type and a foreign origin; the page shows
the echoed setting).

Not yet shown:

- [ ] iTunes playing, switched to `all` from the console: the stand lights
  from its cover within a few seconds, without the next track.
- [ ] Switched back to `records`: the stand goes dark, and the screen and
  the console still show the cover.
- [ ] nowplayd restarted while `all`: still `all`.
- [ ] A record played while `all`: lit as before.
