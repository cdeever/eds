---
title: "CHG-0030: The Console Switches the Stand"
weight: 30
---

# CHG-0030: The Console Switches the Stand

| | |
|---|---|
| **Date** | 2026-10-19 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | The home backend Pi, `dv01rpi001p01` (`192.168.2.40`): nowplayd, the console, the `nowplayd` and `console` broker accounts; the other services restarted |
| **How** | `make -C deploy/backend push PI=192.168.2.40` from branch `adr-0039-stand-for-every-source` (PR #60), before it is merged |
| **Risk** | Low. Most likely to go wrong: the two accounts made again with grants the broker refuses, and nowplayd or the console unable to log in until they are put back |
| **Decisions** | [ADR-0039](/docs/architecture/decisions/0039-the-console-can-light-the-stand-for-every-source/), whose evidence this is |
| **Related** | [CHG-0029](/docs/changes/2026/0029-the-turntables-album-by-what-is-on-it/), the push before this one |

---

## Summary

The console gets a switch that lights the stand for every source, for working
on the stand's looks with iTunes and Spotify. Pushed from the PR's branch so
it can be tried before it is merged.

This is the first push that changes an account the Pi already has:
`eds-backend-setup` now makes an account again, with the password it has, when
its grants differ from `accounts.conf`. The `nowplayd` and `console` accounts
each gain `nowplaying/stand`.

## Goal

- The two accounts hold the new grants; every service and npagent active;
  `eds-backend-status` clean.
- The console shows "lit for records" and a button.
- iTunes playing: switched to every source, the stand lights from the cover;
  switched back, it goes dark.

## Procedure

### Step 1: Push

**Run:** `make -C deploy/backend push PI=192.168.2.40`

**Verify:** "account eds-nowplayd granted …" and the same for `console` in
the output, each unit `active`, nowplayd's log says no account warning.

### Step 2: On the console

**Verify:** the Goal's last two checks, on the page and the stand.

## Undo

`make -C deploy/backend push PI=192.168.2.40` from `main`; the setup there
leaves the extra grant in place, which is harmless. Switching to records first
leaves nothing retained that a nowplayd from before reads.

## Outcome

| When | Step | What happened |
|---|---|---|

### Departures from the plan

-

## Follow-ups

- [ ] Merge PR #60 once seen working, and record the result in ADR-0039.
