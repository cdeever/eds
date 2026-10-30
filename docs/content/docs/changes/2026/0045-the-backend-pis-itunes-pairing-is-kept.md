---
title: "CHG-0045: The Backend Pi's iTunes Pairing Is Kept"
weight: 45
---

# CHG-0045: The Backend Pi's iTunes Pairing Is Kept

| | |
|---|---|
| **Date** | 2026-10-29 |
| **Change type** | Deployment |
| **Status** | {{< status-badge "complete" "Complete" >}} |
| **Systems** | The home backend Pi, `eds-backend` (`192.168.2.40`): its tools and its iTunes pairing; iTunes on MEDIA-SERVER, which gained a remote; the owner's Mac, which gained an age key |
| **How** | `secrets/new-key.sh`; `make -C deploy/backend push`, `make pair`, `make pairing`, from the branch of PR #107 |
| **Risk** | Low. Most likely to go wrong: the Pi left unpaired by a test, which a copy set aside guarded |
| **Decisions** | [ADR-0052](/docs/architecture/decisions/0052-each-pis-itunes-pairing-is-kept-encrypted-in-the-repository/), and ADR-0037's first step |
| **Related** | [CHG-0044](/docs/changes/2026/0044-spotify-is-switched-from-the-backend-pis-console/), the push before |

---

## Summary

The backend Pi's iTunes pairing lived on its card alone, under the name
"EdS". After: it pairs as "EdS home", and the pairing is kept in the
repository, encrypted to the owner's age key, so a new card for this Pi
starts paired.

## Goal

- The age key made, its private half in the Keychain.
- The Pi paired as "EdS home", and its pairing committed encrypted.
- A card's first boot installs the kept pairing, and `make pairing` restores
  it on a running Pi; iTunes followed after each.

## Procedure

### Step 1: Push

**Run:** `make -C deploy/backend push`, in the worktree of
`adr-0052-itunes-pairing-kept` with `main` merged in.

**Verify:** `eds-agent-pair` takes a name, and `eds-backend-setup` takes a
pairing from the boot partition.

### Step 2: The key, and the pairing

**Run:** `secrets/new-key.sh`, then `make pair`, both by the owner.

### Step 3: Restoring it, twice

**Run:** the Pi's pairing moved aside; the kept one written to
`/boot/firmware/eds/` and `eds-backend-setup` run. Then the pairing removed
and `make pairing`.

## Verification

Every check under Goal.

## Undo

The pairing stays on the Pi whatever happens to `secrets/`. To stop keeping
it, delete `secrets/itunes-eds-backend.sops.json`. "EdS home" is forgotten
in iTunes only with every other remote.

## Outcome

| When (UTC) | Step | What happened |
|---|---|---|
| 2026-10-29 04:02 | 1 | `installed 682e185`; every service active |
| 2026-10-29 04:07 | 2 | `new-key.sh`: the key made, its public half `age123ajx5…` in `.sops.yaml` |
| 2026-10-29 04:09 | 2 | `make pair`: the code typed into iTunes, the Pi listed as "EdS home", the iTunes driver online at 04:09:29, and `secrets/itunes-eds-backend.sops.json` written with its four values encrypted |
| 2026-10-29 04:11 | 3 | Pairing moved aside: the iTunes driver offline. The kept one on the boot partition, `eds-backend-setup`: "iTunes pairing installed", the folder emptied, `npagent:npagent 600`, iTunes online at 04:11:24 |
| 2026-10-29 04:11 | 3 | Pairing removed, `make pairing`: restored byte for byte, iTunes followed. The copy set aside deleted |

### Departures from the plan

- The card was not reflashed. Its two halves were tried apart: `make
  card`'s writing with a throwaway key, and the first boot's taking on the
  running Pi, as above.
- The pairing iTunes knew as "EdS" stays listed there: iTunes forgets
  remotes only all at once.

## Follow-ups

- [ ] The relay paired as "EdS relay" and kept, on a day on dvntm.
- [ ] Rebuild the backend image, so a new card has `eds-backend-setup`'s
      import.
- [ ] The private key on paper, by the owner.
