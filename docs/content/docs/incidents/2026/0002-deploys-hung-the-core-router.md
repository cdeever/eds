---
title: "INC-0002: Deploys Hung the Core Router"
weight: 2
---

# INC-0002: Deploys Hung the Core Router

| | |
|---|---|
| **Date** | 2026-10-24 |
| **Systems** | The mobile site's core router; `deploy/` (`make ship`, `make load`), the Builder, the services workload |
| **Severity** | The mobile site's routed traffic stopped, twice, until the owner rebooted the router; a deploy was left half done, with the services still on the version before |
| **Status** | Open · {{< inc-status "Hardening" >}} |
| **Times** | UTC |

---

## Summary

Every deploy sent every image from the Builder to the services workload,
whether it had changed or not, as fast as it would go: about a gigabyte,
most of it scribe's 767 MB and palette's 256 MB, through the core router
that joins the Builder's segment to the workload's. That evening it was done
eight times. During the eighth, and again when it was retried, the router
hung, and the site's routed traffic with it.

Images are now sent only when their sources changed, as files, at 5 MB/s.

## Impact

The mobile site's routed traffic, twice, for the minutes until the owner
noticed and rebooted the router. The deploy that was running stopped
part-way: four images loaded, none started, so the services ran on as they
were.

## Detection

The owner: "I rebooted the core router -- I think we had the hang condition
again", and then "I think the big file thing you are doing is killing the
core router". The deploy itself only seemed slow; the tool running it gave
up waiting after ten minutes.

## Timeline

| Time | Event |
|---|---|
| 00:00–02:42 | Seven deploys, each sending all six images, about 1 GB, without trouble. |
| 02:58 | `make ship` for ADR-0044: six images built; the load began, palette and the Go images first. |
| ~03:05 | scribe's image streaming to the workload; the deploy stopped making progress. |
| ~03:10 | The owner rebooted the core router: "the hang condition again". |
| 03:14 | The stuck transfers stopped; `make load` alone retried, sending all six again. |
| ~03:20 | The router hung again; rebooted. Claude Code restarted. |
| 03:27 | Transfers still running on the Builder and the workload since 03:16 found and stopped. |
| 03:29 | nowplayd and voiced, 10 MB each, copied by hand in 4 s; the relay pushed; the device flashed by USB. |
| 03:34 | `load.sh`: six images compared by source, none sent, in 6 s; a stale one sent at 5 MB/s in 5 s. |

## Root cause

`deploy/Makefile`'s `load` streamed every image with
`podman save | ssh … podman load`, unthrottled, every time. The router -
that it hangs under a sustained gigabyte between segments - is the
condition; the deploy is what kept meeting it.

## Recovery

The owner rebooted the router twice. The images that mattered were copied
by hand, small ones only; the services were restarted one at a time.

## Contributing factors

- **Retrying the whole load** after the first hang sent the same gigabyte
  again.
- **A transfer outlived the session that started it**: after the restart,
  the Builder and the workload were still mid-copy, eleven minutes in.
- **Nothing measured the transfer**, so "slow" was all anyone could see.

## Corrective actions

| # | Action | Where | Status |
|---|---|---|---|
| 1 | An image is sent only when its sources changed: each carries `eds.source`, a hash of what it was built from, compared with the workload's | `deploy/Makefile`, `deploy/load.sh` | {{< action-status "Done" >}} |
| 2 | What is sent goes as a file, with `scp -l`, at 5 MB/s (`LIMIT_KBIT`) | `deploy/load.sh` | {{< action-status "Done" >}} |

## Preventive actions

| # | Action | Where | Status |
|---|---|---|---|
| 3 | Find why the core router hangs under a sustained transfer between segments | Deevnet IoTaaS | {{< action-status "Open" >}} |

## Follow-ups

| # | Follow-up | Where | Status |
|---|---|---|---|
| 4 | A smaller scribe image: the model and its runtime are most of it | `services/scribe` | {{< action-status "Open" >}} |

## Lessons learned

- **A deploy's size is part of what it changes.** Sending what has not
  changed is never free on a network with a weak link in it.
- **After an interrupted transfer, look for the transfer**, on both ends,
  before starting another.

## Related

- [CHG-0037](/docs/changes/2026/0037-a-request-answered-sooner/), whose deploy it was.
