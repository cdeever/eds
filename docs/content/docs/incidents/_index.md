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

A bare **INC-0001** means an EdS record. A Deevnet IoTaaS incident is always
written **Deevnet INC-0004**. An EdS failure caused by Deevnet IoTaaS gets an
EdS record that links to Deevnet IoTaaS's.

## Records

| ID | Date | Incident | Root cause | Status | Substatus |
|---|---|---|---|---|---|
| [INC-0004](2026/0004-the-stand-went-dark-on-one-channel/) | 2026-10-26 | The stand went dark on one channel | the receiver's Tape 1 output lost its left channel, and the turntable driver listens to the left channel alone | Open | {{< inc-status "Mitigated" >}} |
| [INC-0002](2026/0002-deploys-hung-the-core-router/) | 2026-10-24 | Deploys hung the core router | every image sent on every deploy, a gigabyte unthrottled between segments, which the router does not survive | Open | {{< inc-status "Hardening" >}} |
| [INC-0003](2026/0003-the-library-index-was-unreadable-to-voiced/) | 2026-10-24 | The library index was unreadable to voiced | a file copied into a `:Z` volume keeps its own SELinux label, and an unreadable index was taken silently for none | Open | {{< inc-status "Hardening" >}} |
| [INC-0001](2026/0001-the-beatles-played-for-a-request-nobody-made/) | 2026-10-24 | The Beatles played for a request nobody made | scribe's prompt was a request, which it wrote back for audio it could not make out, and voiced acted on any words | Open | {{< inc-status "Hardening" >}} |
