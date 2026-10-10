---
title: "Documentation"
weight: 1
bookFlatSection: true
---

# EdS Documentation

This site is both the **developer guide** and the **user guide** for EdS. It
grows alongside the services rather than after them.

The split it tries to hold:

- **Architecture** says *why* the system is shaped the way it is, and which
  layer is permitted to make which decision. When a change feels like it
  belongs in two places at once, this is the section that settles it.
- **User Guide** is for operating what exists: setting up a stand, provisioning
  it, running the services.
- **Developer Guide** is for changing what exists: building, testing, and the
  bring-up steps that are easy to get wrong once and painful to rediscover.
- **Reference** is the contracts. These are the pieces other software depends
  on, so they are described precisely and versioned explicitly.

Those four are **maintained**: kept current as the system changes. Three kinds
of record are **retained** instead, written once and left as evidence:

- **[Decisions](/docs/architecture/decisions/)** (ADRs) say why a design fork
  went the way it did, and what was turned down. Read the index before
  reopening a question.
- **[Change Records](/docs/changes/)** say what was changed on something
  running, and what actually happened.
- **[Incident Records](/docs/incidents/)** say what broke, why, and what was
  done about it.

[Keeping Records](/docs/develop/records/) has the rules and the templates.

Implementation lives in the repository. This site defines intent, contracts and
procedure.
