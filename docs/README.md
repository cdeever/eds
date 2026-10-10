# eds-docs

The EdS developer and user guide. Hugo with the
[hugo-book](https://github.com/alex-shpak/hugo-book) theme, following the
pattern established by [`deevnet-docs`](https://github.com/deevnet/deevnet-docs).

Published to <https://cdeever.github.io/eds/>.

## Running it

```bash
git submodule update --init --recursive   # once, after a fresh clone
make server                               # live reload on :1313
make html                                 # build to public/
```

## Living in the monorepo

The site sits at `docs/` inside the EdS monorepo rather than in a repository of
its own, so documentation and the thing it documents move in the same commit.
Three consequences, all handled:

**One Pages site per repository.** The site is served at
`https://cdeever.github.io/eds/` — the *repository* name, not the site name —
so `baseURL` ends in `/eds/` and `make server` mirrors that subpath locally.
There is no second Pages site available from this repository.

**Edit links need the subdirectory.** `.Path` is relative to Hugo's working
directory, so `BookEditLink` prepends `docs/` or every "edit this page" link
would point at a file that does not exist.

**Builds are path-filtered.** `.github/workflows/docs.yml` triggers only on
changes under `docs/**` or to the workflow itself. A firmware or Terraform
commit does not rebuild or redeploy the site.

> [!NOTE]
> Path-filtered workflows and *required* status checks do not combine. If this
> workflow is ever made a required check in branch protection, a pull request
> touching no documentation will wait forever for a check that is skipped
> rather than run. Add a separate always-runs job if a required check is
> wanted.

## Conventions

- `content/docs/architecture/` — why the system is shaped as it is, and which
  layer decides what.
- `content/docs/guide/` — operating what exists.
- `content/docs/develop/` — changing what exists.
- `content/docs/reference/` — the contracts other software depends on.

Those are maintained. Three kinds of record are retained: written once, then
left alone.

- `content/docs/architecture/decisions/NNNN-slug.md` — **ADRs**. A design
  fork, the options, the choice, the consequences. Add a row to the index.
- `content/docs/changes/<year>/NNNN-slug.md` — **change records**. A change to
  the running tenant, a workload or a device in use.
- `content/docs/incidents/<year>/NNNN-slug.md` — **incident records**.

Each has its own number series, in the order opened, never reused. A bare
`ADR-0006` in this repository is EdS's own; a substrate record is always
written `Deevnet ADR-0012`. The rules and templates are under
`content/docs/develop/records/`.

Status badges come from four shortcodes in `layouts/shortcodes/`
(`adr-status`, `status-badge`, `inc-status`, `action-status`). An unknown
status fails the build, on purpose.

Placeholder sections are fine; establishing structure early is worth more than
filling every page at once.
