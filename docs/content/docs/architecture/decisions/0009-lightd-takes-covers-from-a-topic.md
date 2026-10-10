---
title: "ADR-0009: lightd Takes Covers From a Topic"
weight: 9
---

# ADR-0009: lightd Takes Covers From a Topic as a Second Input

| | |
|---|---|
| **Status** | {{< adr-status "Accepted" >}} |
| **Date** | 2026-10-10 |
| **Scope** | How the current track's cover reaches `lightd`. |
| **Related** | [ADR-0002](/docs/architecture/decisions/0002-gamma-in-firmware/), [ADR-0008](/docs/architecture/decisions/0008-nowplayd-arbitrates-and-enriches/) |

---

## Context

`nowplayd` publishes the current cover as image bytes on
`eds/nowplaying/current/art`. `lightd` turns image bytes into a scene, and
today takes them only over HTTP, at `POST /v1/cover`. Something has to carry
the bytes from the topic to `lightd`.

`lightd`'s boundary is that it knows about colour and hardware and nothing
about music ([ADR-0002](/docs/architecture/decisions/0002-gamma-in-firmware/)).

## Options considered

**A third service** that subscribes to the topic and POSTs to `lightd`. The
boundary is untouched, but the service's whole job is to move bytes from one
transport to another, and it is one more thing to deploy and to fail.

**`nowplayd` calls `lightd`.** One fewer process, but `nowplayd` would then
know about `lightd`, and the bus would stop being the thing subscribers
share.

**`lightd` subscribes.** `lightd` already holds a broker connection and an
account. It gains a second input that delivers exactly what the first does.

## Decision

`lightd` gains `LIGHTD_COVER_TOPIC`, unset by default. When set, `lightd`
subscribes to it and feeds each message through the same path as
`POST /v1/cover`.

## Consequences

- The boundary holds: `lightd` still receives image bytes and nothing else.
  It does not learn what a track is, and the topic is a setting, not
  knowledge.
- `POST /v1/cover` stays, and remains the way to drive a stand by hand.
- `lightd` subscribes for the first time. Subscriptions are made in the
  on-connect handler so they survive a reconnect, and its broker account
  gains one subscribe grant.
- Two inputs can now set the scene, and the last one wins. That is the
  existing rule, and it is acceptable while there is one stand.
- A retained cover is delivered again whenever `lightd` reconnects, so it
  republishes a scene the broker already holds. Harmless, and it means a
  restarted `lightd` puts the room right by itself.
- A cover that `palette` cannot use is logged and ignored, as a bad upload
  is.
