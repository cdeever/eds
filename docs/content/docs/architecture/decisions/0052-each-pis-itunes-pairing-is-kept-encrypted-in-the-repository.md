---
title: "ADR-0052: Each Pi's iTunes Pairing Is Kept, Encrypted, in the Repository"
weight: 52
---

# ADR-0052: Each Pi's iTunes Pairing Is Kept, Encrypted, in the Repository, and a New Card Starts Paired

| | |
|---|---|
| **Status** | {{< adr-status "Accepted" >}}: a pairing kept, put on a card's boot partition and taken by the first boot's setup, with nobody at iTunes, 2026-10-29 ([CHG-0045](/docs/changes/2026/0045-the-backend-pis-itunes-pairing-is-kept/)); see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-28 |
| **Scope** | Where an agent's iTunes pairing is kept between cards, how it reaches a new card, and one pairing per Pi. Not Spotify's authorization, and not the other hand-kept secrets. |
| **Extends** | [ADR-0037](/docs/architecture/decisions/0037-hand-kept-secrets-are-encrypted-in-the-repository/): its `secrets/`, its age key and `sops` are built here, for this first secret. A pairing is "made where it is used", which that record leaves where it is made; this record says why a pairing is kept anyway. [ADR-0011](/docs/architecture/decisions/0011-itunes-by-remote-pairing/), whose consequence "lose it and pairing is repeated" this ends. |
| **Related** | [ADR-0010](/docs/architecture/decisions/0010-home-lan-relay/), the relay Pi. [ADR-0033](/docs/architecture/decisions/0033-the-whole-back-end-on-one-pi/), the backend Pi. |

---

## Context

The iTunes driver controls iTunes as a paired remote (ADR-0011). Pairing
takes a person at the iTunes PC, typing a four-digit code that the Pi
prints. What it leaves behind is one small file on the Pi,
`/var/lib/eds-npagent/itunes-pairing.json`:

```json
{ "guid": "<16 hex digits>", "host": "<the iTunes PC>", "port": 3689, "name": "<the library>" }
```

The GUID is the whole credential. iTunes remembers the GUID, not the Pi or
the card that showed it: whoever logs in with it is that remote. So a card
that starts with the same file is already paired.

Today no card does. An image carries no secret (its build fails if a
pairing is in it), `make card` writes none, and a card written again starts
unpaired. Two Pis now need one: the relay on dvntm and the backend Pi at
home. Every new card for either is a trip to the iTunes PC. Rebuilding a
card is also how most things are fixed, and that is exactly when nobody is
at the PC.

ADR-0037 sorts EdS's secrets into three kinds. A pairing is in the second,
"made where it is used", which ADR-0037 leaves where it is made, with one
holder by design. That fits a card's CA, which a new card simply makes
again. It does not fit a pairing, which cannot be made again without a
person, and which iTunes goes on honoring after the card is gone.

## Options considered

**Pair again each time.** Today's position. It costs a person at the PC
for every card, and leaves another forgotten remote in iTunes each time.

**A gitignored file on the Mac, per Pi.** `make pair` copies the pairing
back; `make card` writes it to the card. Simple, and the pattern
`discogs.env` follows. It is the same single file on one Mac that
ADR-0037 was written to end: not in a worktree, and lost with the Mac.

**The tenant's Terraform state.** Terraform does not issue a pairing, the
backend Pi deliberately sits outside the tenant (ADR-0033), and the tenant
has no route to iTunes.

**On the Pi, somewhere a reflash does not reach** (a USB stick, a second
partition). A new card is exactly when it is wanted, and this is the copy
that would not travel with it.

**Encrypted in the repository, with sops, as ADR-0037 proposes.** The
pairing goes where hand-kept secrets go: versioned, in every worktree, and
the same on any machine with the key.

## Decision

**Each Pi's pairing is committed to the repository encrypted with sops to
the owner's age key. `make pair` keeps it there, and `make card` writes it
onto the next card for that Pi.**

- **One pairing per Pi**, never shared. Each pairs under its own name:
  `NP_REMOTE_NAME` is `EdS relay` on the relay and `EdS home` on the
  backend Pi, so iTunes lists them apart. iTunes holds several remotes.
  Sharing one GUID would make a lost card a reason to forget both.
- **The file is `secrets/itunes-<hostname>.sops.json`**, named by the
  card's hostname (`PI_HOSTNAME`). It is the pairing as the agent wrote it,
  with every value encrypted and its four keys readable.
- **`make pair` keeps it.** After the code is typed, it reads the new
  pairing from the Pi over SSH and pipes it into `sops --encrypt`. The
  plain file is never written on the Mac. Committing it is the owner's
  step, as any other change is. A pairing kept already is replaced, and
  the old one is said to be still remembered by iTunes until forgotten
  there.
- **`make card` writes it.** If the hostname has a pairing, it is
  decrypted straight onto the card's boot partition beside the other
  settings. On the first boot the import installs it as
  `/var/lib/eds-npagent/itunes-pairing.json` (owned by `npagent`, `0600`)
  and deletes it from the boot partition, as it does the Wi-Fi key. A
  hostname with no pairing is not an error: the card is written, and says
  to run `make pair` after it boots.
- **`make pairing` puts it on a running Pi**, over SSH, for a Pi whose
  pairing was lost without its card being rewritten.
- **The image stays secret-free.** Its check for a baked-in pairing does
  not change.
- **The first step of ADR-0037 is built with it**: `secrets/`, `.sops.yaml`
  naming the owner's age public key, the private key in the Mac's Keychain
  and on paper, and `SOPS_AGE_KEY_CMD` to read it. The other secrets
  ADR-0037 names (Discogs, Spotify's client ID, the state store's
  credentials) move under that record, not this one.
- **No sops, no key, or a file that will not decrypt** stops `make pair`
  with the reason. The pairing is still on the Pi, so nothing is lost; it
  just is not kept yet. For `make card` the pairing is optional, so a
  failure to decrypt is said and the card is written without it.

## Consequences

- **A card written again starts paired.** iTunes is followed from its
  first boot, and nobody goes to the PC.
- **The iTunes PC's address travels with the pairing.** It is where iTunes
  answered at pairing time, so a reserved address for the PC matters as
  much as one for each Pi.
- **Forgetting a remote is done in iTunes.** iTunes can forget remotes only
  all at once ("Forget All Remotes"), so after that every Pi pairs again,
  and each `make pair` replaces its file.
- **The key now guards control of the music.** Anyone with the age key and
  this repository can log in to iTunes as a remote, from the home network.
  They could play, pause and change the volume, and nothing more. That is
  less than the other secrets the key will hold under ADR-0037.
- **`sops` and `age` are needed on the Mac** to pair or write a card, and
  only there. The Pis are given plain files.
- **ADR-0037's sorting gains a line**: a secret made where it is used, that
  cannot be made again without a person, is kept as a hand-kept one is.
- Spotify's authorization stays where it is made. Spotify may replace its
  refresh token as it is used, so a kept copy can go stale; keeping it
  would need the Pi to write it back after every refresh. That is a
  decision of its own.

## What would settle it

Before the key was made, the script was tried against the backend Pi
with a throwaway key: keep, restore (byte for byte, iTunes followed after),
card, a hostname with nothing kept, and a Mac without the key. macOS's bash
3.2 read an apostrophe inside `${VAR:?message}` as a quote, twice; the
messages were reworded. sops looks for `.sops.yaml` from the folder it is
run in, so the script names it with `--config`.

On 2026-10-29 (CHG-0045):

- [x] `sops` 3.13.3 and `age` 1.3.2 installed on the Mac; `new-key.sh` made
      the key, its private half in the Keychain and its public half,
      `age123ajx5…`, in `.sops.yaml`. The copy on paper is the owner's.
- [x] `make pair` on the backend Pi at home: the code typed once, the Pi
      listed in iTunes as "EdS home", and `secrets/itunes-eds-backend.sops.json`
      written and committed, its four values encrypted.
- [x] A card's first boot, without a reflash: the Pi's pairing moved aside,
      the kept one decrypted to `/boot/firmware/eds/` as `make card` writes
      it, and `eds-backend-setup` run. It installed it (`npagent`, `0600`),
      emptied the boot partition's folder, and iTunes was followed within
      five seconds. `make card`'s half, the file written to a card's folder,
      was tried with the throwaway key.
- [ ] The relay paired as `EdS relay` and kept the same way, iTunes listing
      both remotes. On a day on dvntm.
- [x] `make pairing` restored a deleted pairing on the running Pi, byte for
      byte, and iTunes was followed again.
