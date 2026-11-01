---
title: "CHG-0047: The Tenant Switches Spotify, and Names Its Console Without a Site"
weight: 47
---

# CHG-0047: The Tenant Switches Spotify, and Names Its Console Without a Site

| | |
|---|---|
| **Date** | 2026-11-01 |
| **Change type** | Deployment |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | The tenant's Terraform (two broker accounts, the console's certificate); the services workload's console; the relay Pi `dv02rpi002p01` |
| **How** | `make apply` in `infra/deevnet-tenant-eds`; `make -C deploy deploy WHERE=builder` and `make -C deploy units`; `make -C deploy/relay push` and `drivers` |
| **Risk** | Low. Most likely to go wrong: the relay's agent, pushed from `main`, losing the album demo someone was using (see below) |
| **Decisions** | [ADR-0051](/docs/architecture/decisions/0051-spotify-is-switched-on-and-off-from-the-console/); EdS's part of Deevnet CHG-0055 (Deevnet ADR-0040) |
| **Related** | [CHG-0044](/docs/changes/2026/0044-spotify-is-switched-from-the-backend-pis-console/), the same switch on the backend Pi |

---

## Summary

ADR-0051 is proven on the backend Pi (CHG-0044). The tenant's half was
waiting for dvntm: the broker grants for the switch, the console that draws
it, and the relay's agent that obeys it.

Two more things go with it:

- **The console's certificate gains its site-less name.** It named only
  `console.eds.mobile.deevnet.net`. With `sites = ["mobile"]` it names
  `console.eds.deevnet.net` too. That is EdS's part of Deevnet CHG-0055,
  and what Deevnet's check V10 asks a browser to open, both names with one
  certificate.
- **The relay runs the Spotify album demo.** `a05b68c`, `00e6e3a` and
  `de44c28`, from the branch `demo-spotify-album`, were pushed to it on
  2026-10-27 and 10-28 with no record. Pushing `main` replaces them. The
  demo stays on GitHub, not yet a decision.

## Goal

- The two accounts granted the switch's topic; the console's certificate
  naming both names.
- The tenant's console showing the Spotify switch, serving the new
  certificate.
- The relay running `main`, Spotify allowed and switched off, asking it
  nothing.

## Procedure

### Step 1: The tenant's Terraform

**Run:** `make plan`, then `make apply`, from the branch of this change.

**Verify:** 0 to add, 3 to change, 0 to destroy; afterward no changes.

### Step 2: The console

**Run:** `make -C deploy deploy WHERE=builder`, then `make -C deploy units`.

**Verify:** `https://console.eds.mobile.deevnet.net/` presents the new
certificate and draws the Spotify row.

### Step 3: The relay

**Run:** `make -C deploy/relay push`, then
`make -C deploy/relay drivers DRIVERS=itunes,spotify,turntable`.

**Verify:** the relay's release names `main`; Spotify offline and no calls
to it for a full minute; iTunes and the turntable followed.

## Undo

Step 1: revert this change's `main.tf` and apply; the old certificate stays
valid until 2027-10-11. Step 3: `make -C deploy/relay drivers
DRIVERS=itunes,turntable`.

## Outcome

| When (UTC) | Step | What happened |
|---|---|---|
| 2026-11-01 01:33 | 1 | Provider upgraded to 0.8.0 in the worktree. The plan: 0 to add, 3 to change, 0 to destroy, all in place: the console's certificate given `sites`, and `console` and `np-relay` granted `nowplaying/source/spotify/switch`. The owner applied it. The new certificate: serial `24:d7:34:39:…:36:1b`, valid to 2027-11-01, for `console.eds.deevnet.net` and `console.eds.mobile.deevnet.net`. A second plan: no changes |

### Departures from the plan

- The tenant had no TSIG variable any more; the plan needed only the API
  token.

## Follow-ups

- [ ] Tell Deevnet that eds is reissued for CHG-0055, and that V10's two
      names are in one certificate.
