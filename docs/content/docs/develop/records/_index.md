---
title: "Keeping Records"
weight: 90
bookCollapseSection: true
---

# Keeping Records

EdS writes three kinds of record, each answering a different question. They
are what stops a settled design question being argued again from memory, and
what lets a change made in a hurry be understood a month later.

| Record | Question it answers | Where |
|---|---|---|
| **ADR** | Why is it built this way, and what did we turn down? | [Decisions](/docs/architecture/decisions/) |
| **CHG** | What was changed on something running, and what happened? | [Change Records](/docs/changes/) |
| **INC** | What broke, why, and what was done about it? | [Incident Records](/docs/incidents/) |

## The working rule

1. **Before reopening a design question, read the
   [decision index](/docs/architecture/decisions/).** If a record answers it,
   that answer stands until a new record changes it.
2. **A design fork gets an ADR before the code.** If two approaches were
   seriously considered, write down both and why one won. If the choice rests
   on something unproven, the record is `Proposed` until the evidence is in.
3. **A change to the running tenant, a workload or a device in use gets a
   CHG**, written before it runs and completed after.
4. **A failure someone would notice gets an INC**, started while it is fresh.

Records move in the same commit as the work they describe, like the rest of
this site.

## Numbering

Each kind has its own series — `ADR-0001`, `CHG-0001`, `INC-0001` — numbered
in the order records are opened and never renumbered or reused. Change and
incident records are grouped in a folder per year; the number runs across
years.

EdS runs on the Deevnet substrate, which uses the same three names. A bare
number in this repository is always EdS's own. A substrate record is always
written with its owner — **Deevnet ADR-0012** — and linked to
[deevnet-docs](https://deevnet.github.io/deevnet-docs/).

## Templates

- An ADR's format is described on the
  [decision index](/docs/architecture/decisions/#format); copy the most recent
  record.
- [Change record template](change-record/)
- [Incident record template](incident-record/)
