---
title: "CHG-0014: The Owner's Collection"
weight: 14
---

# CHG-0014: The Owner's Collection

| | |
|---|---|
| **Date** | 2026-10-12 |
| **Change type** | Deployment |
| **Status** | {{< status-badge "in-progress" "In progress" >}} — a record from the collection is shown with its own sleeve; one that is not in it has not been tried |
| **Systems** | the services workload: nowplayd's image, its unit, one settings file changed and one added. The console's image, for one line of its page |
| **How** | `make -C deploy deploy WHERE=builder` |
| **Risk** | Medium. Most likely to go wrong: nowplayd's unit, which names a new settings file, not starting; nothing would then be current and the stand would hold its last scene |
| **Decisions** | [ADR-0022](/docs/architecture/decisions/0022-the-owners-collection-names-the-record/) |
| **Related** | [CHG-0013](/docs/changes/2026/0013-the-turntable-driver/), which put the turntable on the bus |

---

## Summary

nowplayd learns to ask its owner's Discogs collection which record a track
from the turntable is on.

After this change a record on the platter is shown as the record it is, with
its own sleeve, and the stand is lit from that sleeve.

## Goal

- nowplayd starts, lists the collection, and says how many records it found.
- A record that is in the collection is `current` with the record's own
  title, and `art.from` is `discogs`.
- A record that is not is completed from the catalogues, as before.
- iTunes is completed exactly as before.

## Scope

**In scope:** nowplayd's image and unit; `NP_COLLECTION_SOURCES=turntable` in
`nowplayd.env`; a new `discogs.env` on the workload, holding the Discogs
username and token; the console's image, for its note beside a cover.

**Out of scope:** the agent Pi, the tenant's Terraform, lightd, palette, the
stand. No broker account changes: nowplayd publishes where it always has.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| The unit names `discogs.env` and the file is not there | the workload | `make units` refuses to run without it, and installs it with the others |
| The token is wrong, or the collection cannot be read | nowplayd | not fatal: said once in the log, and every track is completed the ordinary way |
| The token leaks | the repository, the logs | the file is gitignored and `0600`; nowplayd logs whether there is a token, never the token |
| A wrong record is named | the room | the catalogues are still there for what the collection lacks; `NP_COLLECTION_SOURCES` emptied is the old behaviour |
| Discogs' limit is met | nowplayd | a tracklist read is waited out; a lookup that does not finish in time falls back to the catalogues |

## Procedure

```bash
# discogs.env is in deploy/, kept by hand. Then everything, as usual.
make -C deploy deploy WHERE=builder
make -C deploy status
make -C deploy logs
```

## Verification

- nowplayd's log has `looking these sources up in the owner's collection
  first` and `the collection was listed records=…`.
- With a record from the collection playing: on the console, the album is the
  record's title, the note says the sleeve is from the owner's collection, and
  the stand changes.
- With iTunes playing: the cover is the player's own, as before.

## Undo

Remove the `NP_COLLECTION_SOURCES` line from `deploy/nowplayd.env`, then
`make -C deploy units restart`. The collection is no longer asked. The image
and the unit can stay.

## Outcome

| When (UTC) | What happened |
|---|---|
| 04:04 | `make -C deploy deploy WHERE=builder`, from `f6020bd` plus the commit that puts an album before a compilation. Four images built on the Builder and loaded; settings and units installed, `discogs.env` among them. |
| 04:05 | All four services active. nowplayd logged `looking these sources up in the owner's collection first sources=[turntable] discogs_user=… token=true`, and three seconds later `the collection was listed records=226`. |
| 04:06 | On the console: iTunes current, paused, with the player's own cover, as before. The turntable online and stopped. The stand online. |
| 04:07 | A record played, by the owner. The turntable reported *Who'll Stop The Rain* by Creedence Clearwater Revival, on what the recogniser called "Chronicle: The 20 Greatest Hits (Remastered 2023 / Hi Res)". `current` named the album *Cosmo's Factory*, with `art.from` of `discogs`; lightd lit from that cover and the stand reported the scene applied. |

Still to do: a record that is not in the collection.

### Departures from the plan

## Follow-ups

- [ ] The evidence [ADR-0022](/docs/architecture/decisions/0022-the-owners-collection-names-the-record/)
  waits on.
- [ ] The token is a file kept by hand. If the substrate comes to offer
  tenants a place for secrets, it belongs there.
- [ ] Compilations of several artists.
