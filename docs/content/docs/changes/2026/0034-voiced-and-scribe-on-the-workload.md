---
title: "CHG-0034: voiced and scribe on the Workload"
weight: 34
---

# CHG-0034: voiced and scribe on the Workload

| | |
|---|---|
| **Date** | 2026-10-24 |
| **Change type** | New service |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | The tenant's Terraform on the mobile site's API: broker accounts for `voiced` and `respeaker-01` and the device `respeaker-01` created; `console` and `nowplayd` given `nowplaying/stand`. The services workload: `eds-scribe` and `eds-voiced` added, the other four rebuilt and restarted |
| **How** | `make -C infra/deevnet-tenant-eds apply`, then `make -C deploy kit-env deploy WHERE=builder`, from the branch `voice-play-album` |
| **Risk** | Medium. The workload has two cores and two gigabytes, and scribe's model takes a few hundred megabytes of it: most likely to go wrong is memory, which would stop a service. `eds-scribe` is held to 900 MB for that reason. Updating `console` and `nowplayd` in place keeps their passwords |
| **Decisions** | [ADR-0042](/docs/architecture/decisions/0042-a-request-is-heard-on-the-workload/); [ADR-0039](/docs/architecture/decisions/0039-the-console-can-light-the-stand-for-every-source/), whose grant this applies to the tenant at last |
| **Related** | [CHG-0033](/docs/changes/2026/0033-an-album-asked-for-on-the-bus/), which put `play_album` on the bus; [CHG-0030](/docs/changes/2026/0030-the-console-switches-the-stand/), which switched the stand on the back-end Pi only |

---

## Summary

A spoken request is written down by `scribe` and made a command by `voiced`
([ADR-0042](/docs/architecture/decisions/0042-a-request-is-heard-on-the-workload/)).
Both have run on a Mac against the development broker. This puts them on the
workload, with `voiced`'s own broker account, and registers the reSpeaker,
whose account is the one that may publish a request.

The plan also brings two accounts up to `main.tf`: `console` and `nowplayd`
were never given `nowplaying/stand` on the tenant (ADR-0039). Both take it as
optional, so nothing changes until the console switches the stand here.

Until the reSpeaker's firmware exists, its account is used from a Mac to
send a recorded request, as the device will.

## Goal

- `terraform apply`: 3 added, 2 changed, 0 destroyed.
- All six services active; `voiced/status` online; scribe answers `/healthz`.
- A recording of "Play Van Halen's first album", published with
  `respeaker-01`'s account on `voice/respeaker-01/utterance`, becomes
  `play_album` on `nowplaying/command`, which nowplayd passes to iTunes, which
  plays *Van Halen*. What was heard is said on `voice/respeaker-01/heard`.
- The workload's memory after scribe has written a request down.

## Procedure

1. `make -C infra/deevnet-tenant-eds plan`, then `apply`.
2. `make -C deploy kit-env`, which writes `voiced-broker.env` beside the
   others.
3. `make -C deploy deploy WHERE=builder`.
4. `make -C deploy status`; `free -m` on the workload.
5. Publish the recording with `respeaker-01`'s account; watch
   `nowplaying/command`, `voice/respeaker-01/heard`, and what plays.

## Undo

`sudo systemctl disable --now eds-scribe eds-voiced` on the workload. The
accounts can stay: nothing uses them but these. To remove them, revert this
branch's Terraform and apply.

## Outcome

| When (UTC) | What happened |
|---|---|

### Departures from the plan

## Follow-ups

- The reSpeaker's firmware (step 4), and its provisioning from
  `respeaker_01_broker`.
- Artists misheard as another name ("Arrowsmith"): corrected against the
  owner's collection, perhaps.
