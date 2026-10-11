---
title: "CHG-0005: The Agent Reports Its Events"
weight: 5
---

# CHG-0005: The Agent Reports Its Events

| | |
|---|---|
| **Date** | 2026-10-11 |
| **Change type** | Deployment |
| **Status** | {{< status-badge "complete" "Complete" >}} 2026-10-11 |
| **Systems** | the `eds` tenant's device registry and the `np-relay` broker account; the agent Pi `dv02rpi002p01` |
| **How** | `terraform apply -target`; `make -C deploy/relay push`; one line added to the Pi's settings |
| **Risk** | Low. Most likely to go wrong: giving the account a device replaces it and reissues its password, which would silence both drivers |
| **Decisions** | [ADR-0017](/docs/architecture/decisions/0017-an-agent-pi-is-a-registered-device/) |
| **Related** | [CHG-0004](/docs/changes/2026/0004-spotify-on-the-agent-pi/) |

---

## Summary

The agent Pi follows iTunes and Spotify and says so only in its own journal.
After this change it is a registered device of the tenant and reports its key
events to the log store: each track that plays, and each time a driver comes
or goes.

## Goal

- `np-relay` is in the tenant's device registry, and its account may publish
  `log/np-relay`.
- Playing a track in iTunes or Spotify puts a `track.playing` event in the
  log store within seconds, naming the source, artist, album and title.
- The events appear on the LP Stand dashboard's event panel, beside the
  stand's.
- Both drivers keep reporting to the bus as before.

## Scope

**In scope:** one device entry; one more topic and a device on the relay's
account; a new agent on the Pi; one setting on the Pi.

**Out of scope:** the dashboard itself, which already shows every device's
events; `nowplayd` and `lightd`, which are workloads and report another way.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| Giving the account a device replaces it | the broker; both drivers go silent | the plan is read first and must show an update in place. It did in a dry run |
| The plan also carries an unfinished dashboard change | Grafana | the apply is targeted at the device and the account only |
| The API accepts the plan and refuses the device | the apply | nothing has changed if it does; the agent runs as before without the setting |
| Events hold up a driver | the Pi | they are queued and dropped when the queue is full; tested |
| A third connection on one login is refused | the Pi | two already are accepted ([CHG-0004](/docs/changes/2026/0004-spotify-on-the-agent-pi/)); the agent stops at start-up and says so if not |

## Procedure

### Step 1: The device and the account

```bash
terraform plan -target=deevnet_iot_device.np_relay -target=deevnet_iot_broker_account.np_relay
```

Expected: one device created, the account updated in place. Then apply.

### Step 2: The agent

```bash
make -C deploy/relay push
```

### Step 3: The setting

`NP_LOG_DEVICE=np-relay` is added to `/etc/eds/npagent.env` on the Pi, and
the agent restarted. A card written from now on carries it already.

## Verification

- The agent logs `reporting events topic=eds/log/np-relay`.
- `make -C firmware/lp-stand logs` shows `agent.started` and a `track.*`
  event from `np-relay`.
- A track played in iTunes appears as `track.playing` within seconds.
- `make -C deploy/relay status` still passes.

## Undo

- Remove `NP_LOG_DEVICE` from the Pi's settings and restart the agent: it
  reports to its journal only.
- The device entry and the topic can stay; nothing publishes without the
  setting.

## Outcome

| When (UTC) | Step | What happened |
|---|---|---|
| 16:1x | 1 | The plan showed one device created and the account updated in place. Applied. The account's password was unchanged. |
| 16:19 | 2, 3 | Pushed, the setting added, the agent restarted. It logged `reporting events topic=eds/log/np-relay`, then made its three connections; the broker accepted all of them. Both drivers came online. |
| 16:20 | — | Read back from the log store, from the desk: `agent.started`, `driver.online` for each driver, `track.paused` for the track loaded in iTunes and `track.stopped` for Spotify, each under device `np-relay`. |
| 16:20 | 2 | Pushed again: a track event's message was `published`, which says nothing on a dashboard. It is now the sentence a person would read, `paused: Artist - Title [Album]`. |
| 16:24–16:26 | — | Four tracks played. Each arrived in the log store as `track.playing` with its sentence, and is listed under *What played*. |
| 16:2x | — | The image rebuilt and fetched, its checksum verified. |
| 16:3x | 2 | Pushed once more: the sentence now says where a track played, `playing on Spotify: Artist - Title [Album]`, and the event carries the player's name as `player`. The source was already a field, and not in the line a person reads. |
| 16:2x | — | A *What played* panel added to the dashboard, above the rest, and the event panel renamed *Device events* now that it holds more than the stand's. |

### Departures from the plan

- **The dashboard was changed after all.** The plan put it out of scope
  because the event panel already shows every device. A list of what played
  is the reason for the change, and deserved its own panel.
- **The apply was targeted**, as planned, to leave an unfinished dashboard
  change out. The dashboard was then applied on its own once that change had
  been set aside.
- The counters at the top of the dashboard count every device's events. With
  a second device, *Warnings and errors* now includes a driver going offline.

## Follow-ups

- [ ] What the agent reports when iTunes quits or the PC sleeps is now
  visible without SSH, and still has not been watched.
