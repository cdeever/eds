---
title: "ADR-0037: Hand-Kept Secrets Are Encrypted in the Repository"
weight: 37
---

# ADR-0037: The Secrets Nothing Else Can Issue Are Kept Encrypted in the Repository, with sops and age

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until the first secrets have moved and a second machine has read them; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-18 |
| **Scope** | Where EdS keeps the secrets it is the only holder of, and how its tools read them. Not the secrets Deevnet IoTaaS issues, and not those made on a device or a card. |
| **Extends** | [ADR-0004](/docs/architecture/decisions/0004-firmware-secrets-nvs-only/), whose rule (no secret compiled in) stands; [ADR-0022](/docs/architecture/decisions/0022-the-owners-collection-names-the-record/), whose token "is never committed" in plain text, and now is committed encrypted |
| **Related** | [ADR-0033](/docs/architecture/decisions/0033-the-whole-back-end-on-one-pi/), whose first card went out without the Discogs token |

---

## Context

On 2026-10-18 the first backend card was written from a worktree, and went
out with no Discogs settings. `make card` looks for `deploy/discogs.env` in
the checkout it runs in; the file is gitignored, so a worktree never has it;
and the script says "no Discogs settings" and carries on, as it should for an
owner who has none. Nothing was wrong except where the secret lived.

EdS's secrets are of three kinds, and only one of them has nowhere to live:

| Kind | Which | Where the truth is |
|---|---|---|
| **Issued by Deevnet IoTaaS** | broker passwords, log tokens, the devices' Wi-Fi key, the console's TLS key | the tenant's Terraform state, in Deevnet's store. `kit.env`, `*-broker.env` and `deploy/tls/` on the Mac are copies, written again by `make -C deploy kit-env tls-files` |
| **Made where they are used** | a backend card's CA, accounts and tokens (ADR-0033); Spotify's refresh token on the agent Pi | the card, or the Pi. One holder, by design |
| **Kept by hand** | `deploy/discogs.env` (a personal Discogs token), `deploy/relay/spotify.conf` (an app's client id and a household's device names), the tenant's `.backend.env` (the state store's credentials), `TF_VAR_tsig_key_secret` | **one file on one Mac**, or an environment variable typed in. Copied "from a machine that already has it" |

The third kind is the problem. Each is in one place, outside version
control, found by a path that differs by checkout; a new Mac starts without
them, and a lost Mac loses them. Deevnet IoTaaS keeps its own the way this
ADR proposes: its inventory's `vault.yml` files are committed, encrypted.

## Options considered

**Leave them as files, and teach the tools where the main checkout is.** The
narrow fix for the card. Nothing is versioned or backed up, and every tool
that wants one learns a path.

**The Mac's Keychain.** `security find-generic-password` in each tool. No
file at all, and nothing to install. Not versioned, tied to one Mac, and
`.backend.env` and `spotify.conf` are files their programs read, so each
would be written out again for every use.

**Deevnet IoTaaS's OpenBao.** Deevnet runs one, for its own platform; no
record says a tenant is offered it. And a backend Pi at home has no Deevnet
behind it.

**Encrypted in the repository, with sops and age.** Each secret is a file
beside what uses it, its values encrypted to an age key and its names left
readable. Versioned, reviewable as a diff of names, present in every
worktree, and the same on any machine with the key. One key to keep safe.

**Encrypted in the repository, with ansible-vault.** What Deevnet uses. One
password for whole files, so a diff says only that something changed; and
EdS has no Ansible outside its image builds.

## Decision

**A secret that EdS is the only holder of is committed to the repository,
encrypted with sops to an age key, and read by the tools with
`sops --decrypt`. Secrets that are issued, or made where they are used, are
not moved.**

- **Where:** `secrets/`, at the root, one file a secret, dotenv where the
  program reads dotenv: `secrets/discogs.sops.env`, `secrets/spotify.sops.conf`,
  `secrets/tenant-backend.sops.env`. `.sops.yaml` beside them names the
  recipients.
- **The key:** one age identity, the owner's. Its private half is kept in
  the Mac's Keychain, and on paper, nowhere else; `sops` is given it through
  `SOPS_AGE_KEY_CMD`, so it is never a file on disk. A second recipient is
  added for a second machine, not the key copied to it.
- **Read, not copied out.** A tool decrypts to a pipe or to the place the
  secret is used (`/etc/eds/discogs.env` on a Pi, over SSH), never to a file
  in the worktree. `make card`, a new `make discogs`, `make -C deploy
  discogs-env` and the relay's Spotify step read from `secrets/`.
- **Missing is an error, not a default.** A tool that wants a secret and
  cannot decrypt it stops and says why, unless the secret is optional
  (Discogs is) and the owner has said so (`DISCOGS=none`).
- **`.gitignore` keeps ignoring the plain files.** `*.env` and `secrets.*`
  stay; `secrets/*.sops.*` is admitted by name. A plain secret committed by
  mistake is still the first thing caught.
- **Nothing that is issued moves.** The tenant's state is already a store,
  with Deevnet's access control on it. `kit.env` and the rest stay
  re-derivable copies.

## Consequences

- **The card's gap closes for good**: every worktree has `secrets/`, and the
  tools read it the same way.
- **A new Mac needs one thing**: the age key, from paper or from another
  machine that is a recipient. Then `make init` in the tenant, and every
  hand-kept secret, are there.
- **The key is the one thing to lose.** Lost, the encrypted files are
  noise, and each secret is made again at its source: a new Discogs token,
  the state store's credentials from Deevnet. That is today's position for
  each of them, one at a time.
- **A leaked key leaks history.** Every version of every file in `secrets/`
  is readable with it. Rotating means a new key, re-encrypting, and
  replacing each secret at its source; removing old versions from git is
  not enough.
- **Two tools to install**: `sops` and `age`, from Homebrew, on any machine
  that deploys. Not on a Pi or the workload: they are handed decrypted
  values.
- **CLAUDE.md's rule changes in words, not in spirit**: "never committed"
  becomes "never committed in plain text". The rule about logging them is
  unchanged.
- **EdS gains a secret scan.** It has none today: its one workflow builds
  the docs, where Deevnet IoTaaS's repositories run gitleaks on every push.
  Committing ciphertext on purpose is the moment to add one, told that
  `secrets/*.sops.*` is expected and still failing on a plain secret.

## What would settle it

- [ ] `sops` and `age` installed; an age key made, its private half in the
      Keychain and on paper.
- [ ] `discogs.env` and `spotify.conf` moved to `secrets/`, the plain files
      deleted, and the services deployed from them unchanged.
- [ ] `make card` from a worktree puts Discogs on a card; `make discogs`
      puts it on a running backend Pi.
- [ ] `.backend.env` moved, and `make init` in the tenant works from it.
- [ ] A second machine, added as a recipient, decrypts all of them.
- [ ] A gitleaks workflow on pushes and pull requests, passing on
      `secrets/`, and failing on a plain secret committed to try it.
