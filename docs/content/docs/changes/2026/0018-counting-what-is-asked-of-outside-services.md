---
title: "CHG-0018: Counting What Is Asked of Outside Services"
weight: 18
---

# CHG-0018: Counting What Is Asked of Outside Services

| | |
|---|---|
| **Date** | 2026-10-12 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | `npagent` and its recognizer on the agent Pi (`np-relay`); `nowplayd` on the workload; the tenant's Terraform (one dashboard added) |
| **How** | `make plan apply` in `infra/deevnet-tenant-eds`; `make -C deploy/relay push`; `make -C deploy deploy WHERE=builder` |
| **Risk** | Low. Nothing that lights the room changes. Most likely to go wrong: the log store refuses `nowplayd`, and its counts stay in the journal |
| **Decisions** | [ADR-0027](/docs/architecture/decisions/0027-what-is-asked-of-outside-services-is-counted/), which this is the evidence for |
| **Related** | [CHG-0017](/docs/changes/2026/0017-the-gap-between-tracks-and-a-silent-turntable/), which is on the Pi and the workload from a branch of its own and is not in this one: see the procedure |

---

## Summary

`npagent` and `nowplayd` count what they ask of services that are not
EdS's, and say so once a minute. The agent's counts arrive as events.
`nowplayd` writes its own to the tenant's log store, which no EdS workload
has done before. A dashboard, **Outside Services**, shows both.

## Goal

- With a record playing, the dashboard shows the recognizer's calls and
  Discogs', by the minute.
- "Refused for asking too often" says none.

## Scope

**In scope:** the agent, the recognizer program, `nowplayd`, one dashboard.

**Out of scope:** `lightd`, `palette`, the console, every firmware, any
broker account. Nothing is limited or slowed; calls are only counted.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| This branch is deployed without CHG-0017's, and takes the gap and the short wait back off the Pi and the workload | the Pi, the workload | deploy from a tree that has both: merge CHG-0017's branch first, or this into it |
| The plan changes something other than one dashboard | the tenant | read before it is applied: one `grafana_dashboard` to add, nothing changed or destroyed |
| The log store refuses `nowplayd`, or its certificate is not the site CA's | the workload | said once in the journal; the counts stay there and nothing else is affected |
| The recognizer's new answer is not read by an agent that has not been updated | the Pi | the push installs both together; an old agent ignores a field it does not know |
| A panel that reads two partitions shows nothing | Grafana | each partition also has a panel of its own |

## Procedure

1. **The tenant.** `make plan`, read it, `make apply`.
2. **The Pi.** `make -C deploy/relay push`, then `status`.
3. **The workload.** `make -C deploy deploy WHERE=builder`.

Any order. The dashboard is empty until the other two are done.

## Verification

- `nowplayd`'s journal says it is counting for the log store, and names the
  endpoint. It does not say the counts could not be written.
- After a track has changed: a line `N calls to discogs` in the journal, and
  the same in the dashboard's "from nowplayd" list.
- With a record playing: `N calls to shazam` in the agent's journal each
  minute, and in the "from the agent Pi" list.

## Undo

- **The counts:** deploy and push the commit before this one. Lines already
  in the store age out with its retention.
- **The dashboard:** remove `grafana_dashboard.outside_services` and apply.

## Outcome

| When (UTC) | What happened |
|---|---|
| 21:53 | The agent and the recognizer were pushed to the Pi by its owner (`23e4dfb`, this branch on top of CHG-0017's, which had been merged). The agent came back active with both drivers online. The turntable was silent and Spotify's driver is off, so nothing was asked of anybody and there was no count to see. |
| 21:54 | The workload was deployed by its owner (`3de0662`). All four services came back active. `nowplayd` said it was counting for the log store at `https://logs.mobile.deevnet.net`, and listed the collection, 226 records. |
| 21:55 | The first summary: `3 calls to discogs`, none refused, none failed, the slowest 993 ms, 58 left of Discogs' sixty. Nothing was said about the store refusing it, and read back from the workload partition the line was there, with its numbers as fields and `app` as `nowplayd`. |
| 21:58 | This branch was merged, and the tenant planned from the main checkout: 1 to add, `grafana_dashboard.outside_services`, 0 to change, 0 to destroy. The apply was left to its owner. |
| 22:00 | Its owner applied it: 1 added, 0 changed, 0 destroyed. The dashboard is `eds-outside-services`, in the folder EdS. It has not yet been looked at. |

## Departures

- The Pi and the workload were done before the tenant, and by the owner: the
  push and the merge were refused to the assistant. The dashboard waits on
  this branch being merged, because the tenant's state settings are in the
  main checkout and the dashboard is not.

## Follow-ups

- [ ] The evidence ADR-0027 asks for.
- [ ] The dashboard opened: whether the panels that read two partitions
  draw anything.
- [ ] A count from the agent Pi, which waits on a record being played.
