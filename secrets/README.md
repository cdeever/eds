# secrets

The secrets EdS is the only holder of, committed **encrypted** with
[sops](https://github.com/getsops/sops) to the owner's
[age](https://age-encryption.org) key
([ADR-0037](../docs/content/docs/architecture/decisions/0037-hand-kept-secrets-are-encrypted-in-the-repository.md)).
Each file's values are encrypted and its names are readable, so a diff
says what changed and not to what.

| File | What | Written by |
|---|---|---|
| `itunes-<hostname>.sops.json` | a Pi's iTunes pairing ([ADR-0052](../docs/content/docs/architecture/decisions/0052-each-pis-itunes-pairing-is-kept-encrypted-in-the-repository.md)) | `make pair`, in `deploy/relay` or `deploy/backend` |

## The key

One age key, the owner's. Its private half lives in the Mac's Keychain
(service `eds-sops-age`) and on paper, never in a file. The tools hand it
to sops with

```bash
export SOPS_AGE_KEY_CMD="security find-generic-password -s eds-sops-age -a $USER -w"
```

which `deploy/itunes-pairing.sh` sets by itself.

**Made once**, on the Mac, with `sops` and `age` installed
(`brew install sops age`):

```bash
secrets/new-key.sh
```

It makes the key, puts its private half in the Keychain, and writes its
public half into `.sops.yaml`, which is committed. Then copy the private
half to paper:

```bash
security find-generic-password -s eds-sops-age -a "$USER" -w
```

**Lost, the files here are noise**, and each secret is made again at its
source: a Pi pairs with iTunes again. **A second machine** gets a key of its
own, added to `.sops.yaml` as a second recipient, and then, from this
folder, `sops updatekeys *.sops.*`. sops finds `.sops.yaml` by the folder
it is run in, so the tools name it with `--config`.

## Rules

- **Read, not copied out.** The tools decrypt to a pipe or to where the
  secret is used, never to a file in a worktree.
- **A plain secret is never committed.** `*.env` and `secrets.*` stay
  ignored; only `*.sops.*` is admitted here.
