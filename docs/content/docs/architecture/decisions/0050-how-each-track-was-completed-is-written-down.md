---
title: "ADR-0050: How Each Track Was Completed Is Written Down"
weight: 50
---

# ADR-0050: How Each Track's Album and Cover Were Found Is Written Down, One Line a Track, and nowplayd Sends It to the Log Store

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}}, until the lines have been read on the dashboard from a side of a record; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-27 |
| **Scope** | What `nowplayd` writes down about completing a track: who named its album, where its cover came from, how long each step took and why one gave up; the owner's collection being read; how those lines reach the tenant's log store; the dashboard that reads them. Not how a track is completed, which is unchanged. |
| **Extends** | [ADR-0027](/docs/architecture/decisions/0027-what-is-asked-of-outside-services-is-counted/), whose `api.calls` was the only line `nowplayd` sent to the store |
| **Related** | [ADR-0008](/docs/architecture/decisions/0008-nowplayd-arbitrates-and-enriches/), the completing; [ADR-0022](/docs/architecture/decisions/0022-the-owners-collection-names-the-record/), the collection; [ADR-0038](/docs/architecture/decisions/0038-the-turntable-names-the-original-album-by-what-is-on-it/), the original album |

---

## Context

On the evening of 2026-10-26 the turntable played "Over the Hills and Far
Away", the recognizer named it, and the stand did not follow it: the track
was published with no cover. Finding out why took the journal. *Houses of
the Holy* was not in the owner's Discogs collection; naming its original
album, through Deezer and then MusicBrainz at one call a second, used the
whole lookup time; and the cover's lookup had been given the same time,
already spent. That last fault is fixed (eds#101). Nothing showed any of it.

What is counted today is calls (ADR-0027): how many each service was asked
in a minute, and how many failed. Tonight's failure would have been one slow
MusicBrainz minute on Outside Services. A call is not a track: whether a
track got its cover, and from where, is in the journal only, and only for a
failure.

The owner's collection is read in two parts (ADR-0022): the list of records,
when `nowplayd` starts and every six hours, and an artist's tracklists, the
first time that artist is played. The second is the slow one, and a track
that arrives while it runs waits for it. Both are journal lines too.

## Options considered

### What is written down

**A line for each track.** Who named the album, where the cover came from,
each step's time, and why a step gave up. A side of a record is a dozen
lines; an evening, a hundred or two. A track that went without a cover is
there by name, with its reason.

**A summary a minute, as `api.calls` is.** How many tracks, how many without
a cover. Small, and it loses the one thing wanted: which track, and why.

**The journal, all of it, sent to the store.** Everything is there, and it
is the decision ADR-0027 declined: what a workload's log is for, and how
much of it.

### Which tracks

**Each time a track is completed.** It is completed again on a pause and a
resume, and the lines would say the same thing several times.

**Once for a track, and again only if it comes out differently.** A cover
found the second time a track is completed is news; a pause is not.

### How the lines are sent

**As `api.calls` is sent**, from its own goroutine, waiting on the store.
A track is completed while publishing; a store that hangs would hold the
stand up.

**Queued, and never waited on.** A line that finds the queue full is
dropped, and the journal says so. The journal still has every line.

## Decision

**Each track `nowplayd` completes is one line, the event `cover.lookup`**,
once for a track and again only if its album or cover comes out
differently:

```json
{"event":"cover.lookup","source":"turntable","artist":"Led Zeppelin","title":"Over the Hills and Far Away","heard_album":"Houses of the Holy (Remastered)","album":"Houses of the Holy (Remastered)","named_by":"source","cover_from":"none","name_ms":10002,"cover_ms":3,"name_error":"timeout","cover_error":""}
```

- **`named_by`** is who named the album: `collection`, the owner's record;
  `record`, the one already playing, which has the track; `original`, the
  original album the track is on; `catalog`, a catalog, for a track that
  came with none; `source`, left as the source named it.
- **`cover_from`** is `player`, `discogs`, a catalog's name, or `none`.
- **`name_ms` and `cover_ms`** are each step's time; zero is a step not
  taken.
- **`name_error` and `cover_error`** are `timeout` or `failed` when the
  step gave up. Not finding anything is not giving up, and is empty.
- **A source reported without its cover** (`NP_COVERLESS_SOURCES`) is
  looked up for nothing, and writes nothing.

**The collection says when it has been read**: `collection.listed`, with
the number of records and the time taken, and `collection.tracklists`, for
an artist, with how many records were read, how many failed and the time.

**The lines go to the tenant's log store**, to the workload partition, as
`api.calls` does, and to the journal. They are queued, sixty-four deep, and
never waited on; what is still queued when `nowplayd` stops is not sent.

**The dashboard is Covers**, declared in the tenant's Terraform beside
Outside Services.

## Consequences

- **A track that went without a cover is on a dashboard, by name**, with
  which step gave up. Tonight's would have read `name_error: timeout`.
- **`nowplayd` sends more to the store than `api.calls`**, as ADR-0027 said
  it would not: these three events, and still not its log.
- **Track titles and artists are in the tenant's log store.** They are the
  owner's own listening, in the owner's own store, and the stand's events
  already carry the track playing.
- **A line can be lost** to a full queue or a stop. The journal has it.
- **A track completed again with the same outcome is not written again**, so
  the count of tracks is a count of tracks, not of pauses.
- **The times are a step's, not a call's.** Which call in a step was slow is
  for Outside Services and the journal.

## What would settle it

Tested: one line a track and again when it comes out differently; each of
`named_by`'s answers; a step that ran out of time; a store that hangs holds
nobody up; the collection's two events; and every query on the dashboard,
against VictoriaLogs v1.52.0 with lines as `nowplayd` writes them.

Not yet shown:

- [ ] A side of a record read back from the workload partition.
- [ ] The dashboard showing it, applied by the tenant's Terraform.
- [ ] A track without a cover found on the dashboard, and its reason right.
