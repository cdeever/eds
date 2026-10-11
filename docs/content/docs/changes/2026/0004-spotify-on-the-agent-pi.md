---
title: "CHG-0004: Spotify on the Agent Pi"
weight: 4
---

# CHG-0004: Spotify on the Agent Pi

| | |
|---|---|
| **Date** | 2026-10-11 |
| **Change type** | Deployment |
| **Status** | {{< status-badge "complete" "Complete" >}} 2026-10-11 |
| **Systems** | the agent Pi `dv02rpi002p01`; the `np-relay` broker account of the `eds` tenant; `eds-nowplayd` on the `services` workload |
| **How** | `terraform apply`; `make -C deploy units`; `make -C deploy/relay push spotify` |
| **Risk** | Low to the stand, which this change is designed not to affect. Most likely to go wrong: the account change reissuing the relay's password, which would silence iTunes too |
| **Decisions** | [ADR-0012](/docs/architecture/decisions/0012-spotify-by-web-api/), [ADR-0016](/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover/) |
| **Related** | [CHG-0002](/docs/changes/2026/0002-relay-pi-and-now-playing-accounts/), [CHG-0003](/docs/changes/2026/0003-the-stand-follows-now-playing/) |

---

## Summary

The agent Pi follows iTunes. After this change it also follows a Spotify
account, and reports what that account plays on the devices named as being in
the room.

The stand does **not** follow Spotify. `nowplayd` reports a Spotify track as
current and publishes no cover for it, so the stand holds what it was showing.

## Goal

- The agent on the Pi runs two drivers. Either can be missing what it needs
  without stopping the other.
- `eds/nowplaying/source/spotify/state` names a track played on a named
  device within one poll, and `eds/nowplaying/current` names it with no `art`.
- Playback on a device that is not named is reported as `stopped`.
- **Playing something on Spotify does not change the stand**, and `lightd`
  logs no new scene.
- iTunes still lights the stand, before and after.

## Scope

**In scope:** three more topics on the relay's broker account; one more line
in `nowplayd.env`; a new agent, unit and tools on the Pi; an authorization
for the Spotify account, moved to the Pi.

**Out of scope:** `lightd` and the stand, which are unchanged; reflashing the
Pi; controlling playback, which is a further permission not asked for.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| The account change replaces the account and reissues its password | the broker; the Pi goes silent for iTunes as well | the plan is read first, and the change stops if it shows a replacement and not an update in place |
| The broker ties a login to one client id, and the agent now makes two connections | the Pi | found out on the first start; both connections log `connected to broker` or one is refused |
| The stand follows Spotify after all | `lp-stand-01` | `NP_COVERLESS_SOURCES=spotify` is set before the driver is started, and the goal's check is made with a track playing |
| The Pi cannot reach Spotify from the IoT segment | the Pi | `eds-agent-status` checks `api.spotify.com:443` |
| Restarting the agent drops iTunes for a moment | the Pi | the agent logs in again within seconds, and the broker holds the retained state meanwhile |
| The Pi is ahead of its image | the next card flashed | the image is rebuilt at the end of this change |

## Procedure

### Step 1: The account

```bash
make -C infra/deevnet-tenant-eds plan
```

Expected: `deevnet_iot_broker_account.np_relay` **updated in place**, three
topics added, nothing else. Then `apply`.

### Step 2: nowplayd

```bash
make -C deploy units WHERE=builder
```

Ships `nowplayd.env` with `NP_COVERLESS_SOURCES=spotify`. This is done before
any Spotify report exists, so there is no moment in which one could light the
stand.

### Step 3: The agent

```bash
make -C deploy/relay push
```

The new agent, its unit and its tools, onto the Pi as it runs.

### Step 4: The authorization

```bash
make -C deploy/relay spotify
```

A browser opens on the desk for **Agree**. The authorization is moved to the
Pi. The devices that count are named in `deploy/relay/spotify.conf`, which is
not committed, from the list the Pi prints.

### Step 5: The image

```bash
make -C deploy/relay image fetch
```

## Verification

- `make -C deploy/relay status` passes and shows both drivers.
- The agent logs `connected to broker` twice, once per driver.
- With a track playing on a named device: the source's state and `current`
  name it; `current` has no `art`; `lightd` logs nothing.
- With the same track on the phone: `stopped`.
- An album in iTunes, then Spotify, then a different album in iTunes: the
  stand shows the first cover, holds it, and changes for the second.

## Undo

- `eds-agent-spotify forget` on the Pi removes the authorization and the
  device names; the driver reports itself not set up and iTunes carries on.
- Removing the app at spotify.com/account/apps withdraws the permission at
  Spotify's end.
- The three topics can stay granted; nothing publishes to them without the
  driver.

## Outcome

| When (UTC) | Step | What happened |
|---|---|---|
| 15:43 | 4, early | Authorized from the desk, on the first attempt: the browser came back to the loopback address and a token was issued. **Every request with it was then refused**: `403 Active premium subscription required for the owner of the app`. Neither the device list nor the player could be read. |
| 15:4x | 1 | The plan showed the account updated in place, three topics added. Applied. |
| 15:49 | 2 | `nowplayd` restarted with the setting and logged `reporting these sources without a cover sources=[spotify]`. All three units active; `lightd` repainted from the retained cover. |
| 15:49 | 3 | Pushed. The agent made two connections, `eds-np-relay-itunes` and `eds-np-relay-spotify`, and the broker accepted both: a login is not tied to one client id. The iTunes driver logged in and published at once, which also shows the account's password was not reissued. The Spotify driver reported `not authorized yet` and did not run. From the Pi, `api.spotify.com` and `accounts.spotify.com` were reachable on 443 over the wired leg. |
| 15:54 | 4 | The app had been created under the wrong Spotify account. Created again under the one with Premium; `make spotify` with its Client ID. Authorized, moved to the Pi, and the Pi listed two devices: a computer and a phone. The computer was named. |
| 15:55 | 4 | The driver started, `following the account devices=[…] every=10s authorized_until=2027-04-09`, and reported `stopped`. `eds-agent-status` passed for both drivers. |
| 15:56–15:57 | — | Four tracks on the named computer, one paused and three playing, each reported by the driver and published as `current` by `nowplayd` 3–5 ms later, with no cover. **`lightd` logged nothing**: the stand kept the iTunes cover it had. |
| 15:59 | — | Playback moved to the phone. The driver reported `stopped`. |
| 16:00 | — | A different album in iTunes: `current` named it, and the stand applied a new scene 1.4 s after the driver reported the track. The library had no cover for it, and the one used was looked up (`art_from=deezer`), which is also the first lookup made from the workload in service. |
| 16:02 | — | `nowplayd` redeployed with the correction below. On its restart a stopped Spotify no longer displaced the track paused in iTunes. |
| 16:0x | 5 | Image rebuilt on the Builder and fetched. |

Every check under Verification holds. As in CHG-0003, the stand has no LEDs
attached: "does not change" and "changes" are its own `scene.applied`
reports and `lightd`'s log, not seen by eye.

### Departures from the plan

- **The authorization worked and the account was refused.** The refusal is
  about the *owner of the app*, not the account that clicked Agree: the app
  had been created while signed in to a Spotify account without Premium. A
  new app under the right account, and so a new Client ID and a new
  authorization, cured it.
- **A bare `403` said nothing.** The driver first reported only the status.
  Spotify's own sentence is the only thing that says what to do, so the
  driver now passes it on, goes offline at once, and asks again every five
  minutes instead of every ten seconds.
- **One authorization, two readers.** Listing the account's devices from the
  desk after the token had moved to the Pi would have needed a second copy,
  and Spotify replaces the token on use. The Pi now lists them itself
  (`eds-agent-spotify list`), and the agent re-reads its token file before
  each refresh so that a listing cannot strand the running driver.
- **A source with nothing to say became current.** When the Spotify driver
  started and reported `stopped`, `nowplayd` made that the current state in
  place of the track paused in iTunes: with nothing playing it chose the
  most recent report, and a driver starting up is always the most recent.
  It did not touch the stand, and it was wrong. Arbitration now puts playing
  before paused and paused before stopped, and only then asks which is more
  recent. This is the first time two sources have been live.
- **Steps ran out of order.** The authorization was tried first, from the
  desk, before the account and the agent were changed. Nothing depended on
  the order.

## Follow-ups

- [ ] What Spotify returns during a private session, and whether ten-second
  polling draws a `429` over a day
  ([ADR-0012](/docs/architecture/decisions/0012-spotify-by-web-api/)).
- [ ] Authorize again before 2027-04-09: `make -C deploy/relay spotify AGAIN=1`.
  The driver warns from two weeks before.
- [ ] After a restart of all three services `lightd` lit the same cover
  twice, 0.4 s apart: once from the retained cover and once when `nowplayd`
  published it again. Harmless, and one more than needed.
