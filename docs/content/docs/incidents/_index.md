---
title: "Incident Records"
weight: 60
bookCollapseSection: true
---

# Incident Records

One record per incident: what broke, how it was found, why it happened, how
it came back, and what was done so it does not happen again. Records are
numbered `INC-NNNN` in the order they are opened, like
[ADRs](/docs/architecture/decisions/), and grouped by year.

An incident here is something EdS did wrong, or stopped doing, that someone
would notice: a stand that went dark or showed the wrong record, a service
that stopped publishing, a credential that leaked, a deployment that broke
what was working. A failed test or a bug caught before it shipped is not one.

Each record is written so the next person meets the failure in a document
rather than in the dark. Like [change records](/docs/changes/), these are
**retained**: written once, updated only as their actions close. Start one
while it is fresh, even with most sections empty; the timeline and the wrong
turns are what memory loses first.

New records start from the
[incident record template](/docs/develop/records/incident-record/), which
also defines the statuses used below.

A bare **INC-0001** means an EdS record. A substrate incident is always
written **Deevnet INC-0004**. An EdS failure caused by the substrate gets an
EdS record that links to the substrate's.

## Records

| ID | Date | Incident | Root cause | Status | Substatus |
|---|---|---|---|---|---|
| | | *None recorded yet.* | | | |
