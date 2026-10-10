---
title: "Change Record Template (CHG)"
weight: 1
---

# Change Record Template (CHG)

The shape every [change record](/docs/changes/) takes. Copy the skeleton into
a new file, fill it in **before** the change runs, and complete **Outcome**
after. [CHG-0001](/docs/changes/2026/0001-lp-stand-joins-its-tenant/) is a
worked example.

It follows the substrate's template, trimmed to what an application needs: no
site or maintenance window, because EdS changes one tenant and its own
devices.

## Where it goes

| | |
|---|---|
| **Number** | The next unused `CHG-NNNN`, in the order records are opened. Runs across years, never reused, and kept even if the change is abandoned. |
| **File** | `docs/content/docs/changes/<YYYY>/<NNNN>-<slug>.md` |
| **Title** | `CHG-NNNN: <What changed>`, from the point of view of what is running |
| **Date** | The day execution starts. While it is still planned, the planned date; with none yet, **Unscheduled** |
| **Weight** | `NNNN`, so records sort by number |
| **Index** | Add a row to [Change Records](/docs/changes/) and to that year's page |

## Status

`Planned` → `In progress` → `Complete`, or `Rolled back` or `Abandoned`. A
record is never deleted: a change that was abandoned or rolled back is exactly
the one worth keeping.

## Skeleton

````markdown
---
title: "CHG-NNNN: <What changed>"
weight: NNNN
---

# CHG-NNNN: <What changed>

| | |
|---|---|
| **Date** | YYYY-MM-DD |
| **Change type** | Deployment · Configuration · Upgrade · Migration · Decommission |
| **Status** | Planned · In progress · Complete · Rolled back · Abandoned |
| **Systems** | The tenant, workloads and devices touched, by name |
| **How** | The make targets, Terraform or tools that carry it out |
| **Risk** | Low · Medium · High — and the one thing most likely to go wrong |
| **Decisions** | The ADRs this change carries out |
| **Related** | Other changes and incidents, or None |

---

## Summary

What is changing and why. What is running now, and what will be after.

## Goal

The end state that counts as done, as facts a command or a look can confirm:

-

## Scope

**In scope:** …
**Out of scope:** …

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| | | |

## Procedure

### Step 1: <name>

What this step does, and whether anything stops working while it runs.

**Run:**

```bash
<command>
```

**Verify:**

1. <observable result>

## Verification

The acceptance checks for the change as a whole.

## Undo

Steps are backed out in reverse order. Name the point where undo stops being
practical: a reissued credential, for instance, cannot be un-issued.

## Outcome

*Completed after the change has run.*

| When | Step | What happened |
|---|---|---|
| | | |

### Departures from the plan

-

## Follow-ups

- [ ]
````

## Filling it in

- **Write Goal as checks, not intentions.** "The stand reports its events" is
  an intention; "`make logs` shows a `system.boot` from `lp-stand-01`" is a
  check.
- **Say what replaces a credential.** Replacing a Wi-Fi key or a broker
  account issues a new secret and strands every device holding the old one.
  If a step does that, the record says so in Risk.
- **Record the departures.** What actually happened is the valuable part, and
  the part that differs from the plan is the most valuable of all.
