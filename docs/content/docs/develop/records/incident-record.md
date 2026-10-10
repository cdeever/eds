---
title: "Incident Record Template (INC)"
weight: 2
---

# Incident Record Template (INC)

The shape every [incident record](/docs/incidents/) takes. Copy the skeleton
into `docs/content/docs/incidents/<YYYY>/<NNNN>-<slug>.md`, numbered with the
next unused `INC-NNNN`, and add a row to the index.

Start the record while the incident is fresh, even with most sections empty.
The timeline and the wrong turns are the parts memory loses first.

## Status

An incident carries a **status** and a **substatus**. The status says whether
anything is left to do; the substatus says how far along it is, and only moves
forward.

| Status | Substatus | Means |
|---|---|---|
| **Open** | {{< inc-status "Triage" >}} | Just recorded. Nobody is yet sure what broke. |
| **Open** | {{< inc-status "Investigating" >}} | The fault is being looked for. It may still be broken. |
| **Open** | {{< inc-status "Mitigated" >}} | Working again, but the cause is not fixed: a restart, a revert, a workaround. It can happen again. |
| **Open** | {{< inc-status "Remediated" >}} | The cause is fixed. Preventive actions are not yet chosen or started. |
| **Open** | {{< inc-status "Hardening" >}} | Every corrective action is done. Only preventive actions or follow-ups remain. |
| **Closed** | {{< inc-status "Completed" >}} | Every action is Done or Declined, and every follow-up is Done, Declined or Scheduled with a link. Give the date. |

A record may skip a substatus but never moves back. If a closed incident's
fix turns out not to have worked, open a new incident and link the old one.

Actions are {{< action-status "Open" >}}, {{< action-status "In Progress" >}},
{{< action-status "Done" >}} or {{< action-status "Declined" >}}; follow-ups
use {{< action-status "Scheduled" >}} in place of In Progress. An unknown
status fails the site build, on purpose.

## Skeleton

````markdown
---
title: "INC-NNNN: <What broke>"
weight: NNNN
---

# INC-NNNN: <What broke>

| | |
|---|---|
| **Date** | YYYY-MM-DD — the day it began |
| **Systems** | Services, devices and tenant resources involved, by name |
| **Severity** | What was lost, and what recovery required |
| **Status** | Open · {{</* inc-status "Investigating" */>}} — or Closed · {{</* inc-status "Completed" */>}} YYYY-MM-DD |
| **Times** | Timezone used in this record |

---

## Summary

What happened and why, in two short paragraphs.

## Impact

What stopped working, for whom, and for how long.

## Detection

How it was noticed, by what or by whom, and how long that took. If nothing
detected it, say why not.

## Timeline

| Time | Event |
|---|---|
| | |

## Symptoms

What was observable, in the order it appeared.

## Investigation

How the cause was found, including the wrong conclusions and what overturned
them.

## Root cause

The fault, or the faults that combined. Cite the file and line.

## Recovery

How it came back, and the state things were left in.

## Contributing factors

What made it possible, worse or harder to see, without being the cause.

## Corrective actions

Fix the faults behind this incident.

| # | Action | Where | Status |
|---|---|---|---|
| 1 | | | {{</* action-status "Open" */>}} |

## Preventive actions

Stop this class of failure recurring, or make surviving it unnecessary.

| # | Action | Where | Status |
|---|---|---|---|
| | | | {{</* action-status "Open" */>}} |

## Follow-ups

Worth doing because of this incident, but fixing nothing in it.

| # | Follow-up | Where | Status |
|---|---|---|---|
| | | | {{</* action-status "Open" */>}} |

## Lessons learned

What to carry into the next change, stated so it applies beyond this one.

## Related

The change that caused it, the changes that fixed it, any ADR it led to, and
the substrate's record if the cause was there.
````

## Filling it in

- **Detection and symptoms are different things.** Detection is how the
  incident came to light, and how long that took. Symptoms are what could be
  seen, whether or not anyone read it correctly at the time.
- **Corrective and preventive actions are different things.** Corrective
  actions fix what caused this incident. Preventive actions stop the same
  class of failure elsewhere. Number actions across all three tables, so that
  "action 4" means one thing wherever it is cited.
- **EdS fails soft by design**, which makes its incidents quiet: a stand that
  keeps showing the last scene looks like nothing was sent. Say under
  Detection how long the quiet lasted; that is usually the real finding.
- **Keep the record, even when it is embarrassing.** A wrong conclusion
  reached during the incident belongs under Investigation, with what
  overturned it.
