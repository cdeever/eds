---
title: "CHG-0043: The Backend Pi Names the Record the Recognizer Heard"
weight: 43
---

# CHG-0043: The Backend Pi Names the Record the Recognizer Heard

| | |
|---|---|
| **Date** | 2026-10-29 |
| **Change type** | Deployment |
| **Status** | {{< status-badge "complete" "Complete" >}} |
| **Systems** | The home backend Pi, `eds-backend` (`192.168.2.40`): its five services and the agent |
| **How** | `make -C deploy/backend push`, from the branch of PR #109 |
| **Risk** | Low. Most likely to go wrong: a service not coming back after the push, which `push.sh` checks |
| **Decisions** | [ADR-0038](/docs/architecture/decisions/0038-the-turntable-names-the-original-album-by-what-is-on-it/), as revised by PR #109 |
| **Related** | [CHG-0042](/docs/changes/2026/0042-the-home-pi-becomes-a-deevnet-pi/), during whose last check the fault was found |

---

## Summary

At the end of CHG-0042, Journey's "Wheel in the Sky", heard on *Infinity*,
was named *Alive in America*, a live album, and the stand lit from its
cover. Deezer lists the track on live albums, best-ofs and a 2008
re-recording, never on *Infinity*, and the recognizer's album was asked
about only when the catalog had nothing.

PR #109 has the recognizer's album weighed beside the catalog's, as an
original only, ranked by date. This puts it on the backend Pi, which ran
the image built from `930b191`, and heard the record again.

## Goal

- `/etc/eds-backend-release` records the push.
- Every service and the agent active after it.
- *Infinity* on the turntable named *Infinity*, and the stand lit from its
  sleeve.

## Scope

**In scope:** the backend Pi's services, agent, units and tools, as
`push.sh` installs them.
**Out of scope:** its accounts, settings, pairing and authorizations, which
a push does not touch; the firewall, kernel settings and broker drop-in,
which change only with an image; the tenant's workload and the relay Pi.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| A service does not come back | The backend Pi | `push.sh` reports each one's state |
| The Pi runs code that is not on `main` | The backend Pi | Pushed from the PR's branch, then the PR merged |

## Procedure

### Step 1: Push

**Run:** `make -C deploy/backend push`, in the worktree of
`resolver-hint-original`.

**Verify:** `installed 4e51363`, and each service active.

### Step 2: The record

**Run:** with the stand idle, *Infinity* on the turntable.

**Verify:** nowplayd's `cover.lookup` names *Infinity*, and lightd sends
the stand a scene from its cover.

## Verification

Every check under Goal.

## Undo

`make -C deploy/backend push` from `930b191`, or any later `main`.

## Outcome

| When (UTC) | Step | What happened |
|---|---|---|
| 2026-10-29 03:09 | 1 | `installed 4e51363`, pushed on 2026-10-29T03:09:28Z. `eds-palette`, `eds-lightd`, `eds-nowplayd`, `eds-console` and `npagent` active. The restart emptied the resolver's held answers, so none of the old was reused |
| 2026-10-29 03:10 | 2 | "Wheel In the Sky" heard on *Infinity*: named *Infinity* in 3.4 seconds (`named_by=original`, cover from Deezer), and the stand sent `chase`, four reds from its sleeve, brightness 0.8 |
| 2026-10-29 | — | PR #109 merged as `8f148df`. The Pi's code is `main`'s: the one commit after `4e51363` changed only ADR-0038's text |

### Departures from the plan

- The record was written after the push, not before it.

## Follow-ups

- [ ] Rebuild the backend image (`make -C deploy/backend image fetch`), so
      the next card flashed is not older than this Pi.
