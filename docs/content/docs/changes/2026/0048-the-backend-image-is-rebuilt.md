---
title: "CHG-0048: The Backend Image Is Rebuilt"
weight: 48
---

# CHG-0048: The Backend Image Is Rebuilt

| | |
|---|---|
| **Date** | 2026-11-01 |
| **Change type** | Deployment |
| **Status** | {{< status-badge "complete" "Complete" >}} |
| **Systems** | The backend image (`images/backend`), built on the Builder; nothing running |
| **How** | `make -C deploy/backend image`, then a resumable copy in place of `fetch` |
| **Risk** | Low. Nothing is flashed. Most likely to go wrong: a base without `deevnet-kit`'s `cert` commands, which the downloads still serve |
| **Decisions** | [ADR-0051](/docs/architecture/decisions/0051-spotify-is-switched-on-and-off-from-the-console/), [ADR-0052](/docs/architecture/decisions/0052-each-pis-itunes-pairing-is-kept-encrypted-in-the-repository/) |
| **Related** | [CHG-0043](/docs/changes/2026/0043-the-backend-pi-names-the-record-the-recognizer-heard/) and [CHG-0046](/docs/changes/2026/0046-the-backend-pi-is-on-its-cable-alone/), whose follow-ups this answers |

---

## Summary

The backend image was the one built on 2026-10-27, older than the Pi it
would replace. It is rebuilt from `main`, so the next card has what the Pi
was given by hand since: the resolver's original album (CHG-0043), the
Spotify switch (ADR-0051), the kept iTunes pairing's import (ADR-0052),
and the radio off (CHG-0046). Nothing is flashed.

## Procedure

### Step 1: Build

**Run:** `make -C deploy/backend image`, from `main` at `a33e691`.

**Verify:** the build's own checks pass, among them the radio's starting
state.

### Step 2: Fetch

**Run:** `make -C deploy/backend fetch`.

**Verify:** its checksum, on the Mac.

## Outcome

| When (UTC) | Step | What happened |
|---|---|---|
| 2026-11-01 02:01 | 1 | Built from `a33e691` on the base the Builder keeps, `/srv/deevnet-http/tenant/pi/…pi-backend.img.xz` (2026-10-25, kit with `cert request` and `cert install`), not the 18-Oct one in the downloads. Finished 03:08, its checksum 03:17 |
| 2026-11-01 13:30 | 1 | The Mac's SSH session to the build hung partway, so its log ended at palette's install and never reported; the build itself had finished. Checked on the Builder instead: checksum ok; `/etc/eds-backend-release` names `a33e691`; `NetworkManager.state` has `WirelessEnabled=false`; the kit carries `cert install FULLCHAIN ROOT` |
| 2026-11-01 | 2 | `make fetch` failed with SSH error 255 on the link in use, leaving the old image in place. `rsync --partial` copied it; `shasum -c` ok, 1 040 701 796 bytes |

### Departures from the plan

- The build's check results were not seen in its log, which the hung
  session cut short. The finished image was inspected instead; its checks
  stop the build when they fail.
- `fetch` was replaced by a resumable copy.

## Follow-ups

- [ ] `fetch` with `rsync --partial`, so a dropped link resumes rather than
      leaving the old image.
