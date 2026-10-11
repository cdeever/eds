---
title: "CHG-0006: The Console"
weight: 6
---

# CHG-0006: The Console

| | |
|---|---|
| **Date** | 2026-10-11 |
| **Change type** | Deployment |
| **Status** | {{< status-badge "active" "In progress" >}} |
| **Systems** | the `eds` tenant: a new broker account and a DNS name; the `services` workload: a new `eds-console`, and its firewall |
| **How** | `terraform apply`; `make -C deploy deploy WHERE=builder` |
| **Risk** | Low. Nothing existing is changed except by being restarted. Most likely to go wrong: the console unable to bind port 80 as a user who is not root |
| **Decisions** | [ADR-0018](/docs/architecture/decisions/0018-a-console-of-eds-own/) |
| **Related** | [CHG-0003](/docs/changes/2026/0003-the-stand-follows-now-playing/) |

---

## Summary

EdS gains a page of its own: what is playing and on which player, its cover,
each stand drawn in the colours and motion it was sent, and whether each part
is up. It is served from the workload at `http://console.eds.mobile.deevnet.net/`
and only looks.

## Goal

- A fourth unit, `eds-console`, is active on the workload.
- The page opens from a browser on the site's trusted network and shows the
  track playing, its cover, and the stand.
- A track change appears without a reload.
- The other three services run as before, and the stand still follows iTunes.

## Scope

**In scope:** a broker account that reads and does not write; a DNS record;
a new image, unit and two settings files; **port 80 opened on the workload's
own firewall**, the first port there other than SSH.

**Out of scope:** controls of any kind; a login; TLS; any change to the
other services or the stand.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| A subscription is refused, silently | the broker | each is granted as the exact filter the console uses, and a test holds the two lists together; the page showing a part as "unknown" is how a miss would look |
| The console cannot bind port 80 | the workload | it is given that one capability by name; if the unit fails, it falls back to a high port and the record says so |
| The page is reachable by more than intended | the site | checked beforehand: the workload answers only from the trusted and management networks, and only on ports its own firewall admits. The page changes nothing and holds no secret |
| The services restart together | the workload | as in CHG-0003: the stand keeps its scene, and everything is retained |
| A cover or a title carries markup | a viewer's browser | titles are put on the page as text; only JPEG, PNG, WebP and GIF are kept as a cover; a content policy forbids anything but the page's own files |

## Procedure

### Step 1: The account and the name

```bash
make -C infra/deevnet-tenant-eds plan
```

Expected: one broker account and one DNS record created, nothing changed.

### Step 2: Build, load, start

```bash
make -C deploy deploy WHERE=builder
```

Builds four images on the Builder, loads them, writes the settings, opens
`http` in the workload's firewall, and starts the units.

## Verification

- `make -C deploy status` shows four active units.
- `consoled` logs `connected to broker topics=9`, then `listening`.
- `curl http://console.eds.mobile.deevnet.net/api/state` from the desk names
  the track iTunes has loaded, both sources, the stand and its scene.
- The page, in a browser, with a track change while it is open.

## Undo

- `sudo systemctl disable --now eds-console` on the workload, and
  `sudo firewall-cmd --permanent --remove-service=http && sudo firewall-cmd --reload`.
- The account and the name can stay; nothing uses them without the service.

## Outcome

**Not complete**: it is running and has been checked from the desk. It has
not yet been looked at by a person in a browser, with a track changing.

| When (UTC) | Step | What happened |
|---|---|---|
| before | — | Checked whether a browser on the trusted network can reach a workload at all, which the platform's guide does not say: one port was opened on the workload's firewall for a single request and closed again. It can. |
| 16:5x | 1 | The plan showed one account and one DNS record created. Applied. The nine subscriptions came back granted exactly as the console asks for them. |
| 16:56 | 2 | Four images built on the Builder and loaded; `http` opened in the workload's firewall; four units active. `consoled` logged `connected to broker topics=9` and `listening addr=:80`: binding port 80 as a user who is not root, with that one capability, works. |
| 16:57 | — | From the desk, `/api/state` named the track paused in iTunes with its cover, both sources online, the stand **offline** with the scene waiting for it, and both services online. The stand had been unplugged, so that is right. |
| 16:57 | — | The page rendered in a headless browser on the desk: the track, the cover's top edge still loading, the stand drawn dimmed and marked offline, its four colours and shares. |

### Departures from the plan

- None in the procedure. The fall-back to a high port was not needed.
- A cover on the bus was a 600 KB PNG. The page shows it; it is larger than a
  page this size needs, and nothing here resizes it.

## Follow-ups

- [ ] A person looks at it, and changes the track while it is open
  ([ADR-0018](/docs/architecture/decisions/0018-a-console-of-eds-own/),
  *What would settle it*).
- [ ] The stand plugged back in, to see it go from offline to drawn.
- [ ] Covers are sent to the browser at the size the player gave them.
- [ ] Recently played starts empty at each restart. It could be filled from
  the log store, which has the history.
- [ ] The stand's firmware reports its palette in `scene.applied` as of
  [CHG-0005](/docs/changes/2026/0005-the-agent-reports-its-events/)'s day, and
  has not been flashed with it.
