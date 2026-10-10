---
title: "This Site"
weight: 4
---

# This Site

Hugo with the [hugo-book](https://github.com/alex-shpak/hugo-book) theme,
following the `deevnet-docs` pattern. It lives at `docs/` inside the monorepo
so documentation and the thing it documents move in the same commit.

```bash
cd docs
make server      # live reload on :1313
make html        # build to public/
```

The theme is a git submodule. After a fresh clone:

```bash
git submodule update --init --recursive
```

## Living in a monorepo

Two consequences, both handled:

**A repository serves exactly one Pages site.** The site is therefore published
at `https://cdeever.github.io/eds/` — the repository name, not the site name —
and `baseURL` reflects that. `BookEditPath` carries the `docs/` prefix so "edit
this page" links resolve.

**Builds are path-filtered.** `.github/workflows/docs.yml` triggers only on
changes under `docs/**` or to the workflow itself, so firmware, service and
Terraform commits do not deploy the site.

> [!NOTE]
> If branch protection ever requires this workflow as a status check, a pull
> request that touches no docs will wait forever for a check that is skipped
> rather than run. Path-filtered workflows and required status checks do not
> combine well; use a separate always-runs job if you need a required check.
