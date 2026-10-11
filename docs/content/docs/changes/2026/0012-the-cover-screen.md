---
title: "CHG-0012: The Cover Screen"
weight: 12
---

# CHG-0012: The Cover Screen

| | |
|---|---|
| **Date** | 2026-10-11 |
| **Change type** | Deployment |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | a new device, `cover-screen-01`; the tenant's Terraform (one device, one broker account, grants on two others); `nowplayd` on the workload; the agent on the Pi |
| **How** | `make flash` and `make provision` in `firmware/cover-screen`; `make plan apply` in `infra/deevnet-tenant-eds`; `make -C deploy deploy WHERE=builder`; `make -C deploy/relay push` |
| **Risk** | Medium. It is the first change that lets anything alter what plays. Most likely to go wrong: the agent or `nowplayd` not coming back, which would stop the stand following the music |
| **Decisions** | [ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/), which this is the evidence for |
| **Related** | [CHG-0002](/docs/changes/2026/0002-relay-pi-and-now-playing-accounts/), which made the accounts this adds to |

---

## Summary

A touch screen is added beside the stand. It shows the cover of what is
playing and, when touched, five buttons: play or pause, previous, next,
volume up, volume down.

To get there, `nowplayd` starts publishing each cover a second time, small,
and starts passing commands to the source that is current; the agent's iTunes
driver starts carrying them out; and the broker is told to let each of them.

## Goal

- The board shows the cover of the track playing in iTunes, and a record
  when there is none.
- Each of its five buttons does what it says to the real iTunes.
- The stand, the console and the agent's reporting are as they were.

## Scope

**In scope:** the board's firmware and credentials; the tenant's Terraform;
`nowplayd`; the agent Pi's `npagent`.

**Out of scope:** `lightd`, `palette`, the console, the stand's firmware,
the Pi's image, Spotify.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| The board's own firmware is lost | the board | the whole flash is read to `firmware/cover-screen/backup/` before the first write |
| The plan changes something other than what was written | the tenant | the plan is read before it is applied: one device and one account added, two accounts changed in place, nothing replaced. A replaced account is a new password and a stand or a Pi that stops connecting |
| The substrate refuses a wildcard in a publish grant | the tenant | seen in the plan or the apply; `nowplayd` runs without it and commands simply go nowhere |
| `nowplayd` does not come back | the workload | the deploy reports it; the console shows the current track afterwards |
| The agent does not come back, or stops reporting because its new subscription is refused | the Pi | the subscription is optional by construction and a refusal is only logged; `make -C deploy/relay status` and the console afterwards |
| A command is carried out that nobody pressed | iTunes | commands are never retained, a retained one is ignored twice over, and the screen never queues one |

## Procedure

Ordered so that each step is safe without the ones after it.

1. **The board, with no network.** `make backup`, `make flash`. Check the
   picture is the right way up and in the right colours, and that touches
   land where the finger is; set the panel and touch settings until they do.
2. **The tenant.** `make plan`, read it, `make apply`.
3. **The workload.** `make -C deploy deploy WHERE=builder`.
4. **The Pi.** `make -C deploy/relay push`.
5. **The board's credentials.** `make provision`.

## Verification

- The board's corner mark goes out, and the cover of the track playing
  appears.
- Each button does what it says in iTunes, and the console shows the result.
- `eds/nowplaying/current/art/small` holds a JPEG of 240 pixels.
- The stand still follows a track change.
- The agent's log shows each command it carried out.

## Undo

- **The board:** write `backup/factory-*.bin` back at offset 0.
- **The Pi and the workload:** deploy from `main` again. Both run without
  the new grants.
- **The tenant:** remove the device and account and the added grants, and
  apply. Nothing else depends on them.

## Outcome

| When (UTC) | What happened |
|---|---|

### Departures from the plan

## Follow-ups

- [ ] ADR-0020 moves to `Accepted` when its six questions are answered here.
