---
title: "ADR-0051: Spotify Is Switched On and Off From the Console"
weight: 51
---

# ADR-0051: Spotify Is Switched On and Off From the Console, and Off Asks Spotify Nothing

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}}: until it has been switched both ways from the console on the relay Pi; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-28 |
| **Scope** | A switch for the Spotify driver: where its state is kept, what "off" stops, who reads it, and the console's second control. |
| **Extends** | [ADR-0039](/docs/architecture/decisions/0039-the-console-can-light-the-stand-for-every-source/): a second retained setting, and the console's second control, which that record said would bring back the question of a login. |
| **Related** | [CHG-0024](/docs/changes/2026/0024-spotify-turned-off-again/), whose follow-up this is. [ADR-0027](/docs/architecture/decisions/0027-what-is-asked-of-outside-services-is-counted/), whose count shows the switch working. [ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/), whose commands are never retained; this is not one of them. [ADR-0034](/docs/architecture/decisions/0034-presence-is-heard-not-remembered/). |

---

## Context

The Spotify driver asks Spotify what is playing every ten seconds, all day,
whether or not anything plays. Its owner does not want that all the time,
and does want Spotify followed sometimes.

Today the only switch is the agent's settings: `make -C deploy/relay drivers`
rewrites `NP_DRIVERS` on the Pi and restarts `npagent`. Spotify has been
turned off that way three times (CHG-0011, CHG-0024, and again on
2026-10-27) and on in between, once with no record. Each time needs the
Mac, a shell through the Builder, and someone who remembers the command.
The other drivers restart with it.

## Options considered

**Leave it a setting.** `make drivers` works and needs nothing new. It is
also how Spotify was left on without anyone writing it down.

**nowplayd ignores Spotify while switched off.** One setting in a service
that already reads one (ADR-0039). The driver would still ask Spotify every
ten seconds, which is the thing being switched off. Rejected for that.

**Ask less often when nothing plays.** CHG-0024's other follow-up, and
worth doing on its own. It makes "on" cheaper. It is not "off".

**A command** on the driver's command topic. Not retained (ADR-0020), so a
restarted agent forgets it and Spotify comes back on by itself.

**A retained setting on the broker, read by the agent.** The agent runs
the driver, so the agent is the one place that can stop it asking. The
broker keeps the setting across restarts of both the agent and the
console, and the console reads it back, as with the stand (ADR-0039).

## Decision

**Whether the Spotify driver runs is a retained setting on the broker,
which the console can change and the agent obeys. Off, the driver asks
Spotify nothing.**

- **The topic is `eds/nowplaying/source/spotify/switch`, retained**, holding
  `{"v":1,"driver":"on"}` or `{"v":1,"driver":"off"}`. Words, as with the
  stand's setting. The topic belongs to the source, beside its `state`,
  `status`, `art` and `command`.
- **`NP_DRIVERS` still says which drivers may run.** The switch decides
  whether Spotify does. A Pi without `spotify` in `NP_DRIVERS` ignores the
  switch, and the console's switch then does nothing, which the console
  shows (below).
- **No message, an empty one, a wrong version or a word the agent does not
  know is `off`**, and the agent logs the last three. Off is the default
  because cost is why this exists: a broker that loses its store (a new
  card) goes quiet, not chatty.
- **Off stops the driver.** Its run ends: no polling, no token refresh,
  no request of any kind to Spotify. It says `offline` on its status topic,
  repeated as ADR-0034 asks, so nowplayd's arbiter treats it as every
  other offline source. Its broker connection and its subscriptions stay,
  so it can be switched on again. A command that reaches it while off is
  a failed command, logged, and never queued for later.
- **On starts it**, as the agent would at startup. No restart of `npagent`,
  and the iTunes and turntable drivers are not touched either way.
- **It applies at once**, as the stand's setting does: the agent acts on
  the message when it arrives, and the next `api.calls` minute shows it.
- **The console shows it and switches it.** It draws Spotify's switch
  beside the source: "Spotify: off" is plain, not an error. Off and offline
  are told apart: switched off and offline is as expected; switched on and
  offline for longer than ninety seconds is a driver that could not start,
  and is shown as such.
- **The page's route is `POST /api/spotify`**, with the same rules as
  `POST /api/stand`: JSON only, its own `Origin` only, answered with what
  was published, and the page showing the setting when the broker echoes
  it back.
- **Still no login.** ADR-0039 said a control that does more brings the
  question back. This one decides whether the owner's own Spotify account
  is asked what it is playing. It changes no playback, no volume and
  nothing a person hears, and the page is reachable only from the site's
  trusted networks and the home network. Accepted for this control. One
  that changes what plays is a new decision, and needs a login.
- **Grants.** The console's account may publish this topic and read it;
  the agent's account (`np-relay`, and the backend Pi's) may read it.
  nowplayd's needs nothing. Changed in the tenant's `main.tf` and in
  `images/backend/files/accounts.conf`, kept as one list.
- **Spotify alone, for now.** The shape fits any source, but iTunes costs
  nothing to ask and the turntable asks only when it hears music. Another
  driver with a switch is a line in this record's follow-ups, not a new
  decision.

## Consequences

- **Spotify can be followed for an evening and turned off after**, from the
  page that already shows it. `make drivers` stays for which drivers may
  run.
- **Deploying it turns Spotify off.** The first agent with this reads no
  setting, which is `off`. To keep it on, switch it on from the console
  after the deploy.
- **Turning it on is a decision someone can see.** The console shows it,
  and the Outside Services dashboard shows the calls begin.
- **CHG-0024's way of turning it off stops being needed.** A driver switch
  is no longer a change to record: a setting changed from the console is
  not a change to something running, any more than the stand's is.
- **Each broker has its own switch.** The relay Pi's agent reads the
  tenant's broker, the backend Pi's reads its own. Switching one does not
  switch the other.
- **The console is a little less of a reader**: two controls now, both
  settings, neither a command. CLAUDE.md and the console's README say so.
- nowplayd, lightd and the firmware do not change.

## What would settle it

Tests in `npagent`: the setting's words, and anything else read as off; off
ending the driver's run with no further request to Spotify (a fake API that
counts); on starting it again; the other drivers untouched across both; a
command while off failed and not kept. Tests in the console: the setting
read and defaulting to `off`; the route refusing as `/api/stand` does.

Then, on the relay Pi, with `NP_DRIVERS=itunes,spotify,turntable`:

- [ ] Deployed with no setting: Spotify offline, and no `api.calls` line
      for `spotify` after the first minute.
- [ ] Switched on from the console: Spotify online within seconds, and
      followed while something plays on it.
- [ ] Switched off: offline, and no `api.calls` for `spotify` in the next
      full minute.
- [ ] `npagent` restarted while on: on again without being switched.
- [ ] iTunes and the turntable followed throughout.

## Follow-ups

- [ ] Ask Spotify less often when nothing plays (CHG-0024). It makes "on"
      cheaper and is independent of this.
- [ ] Voice: "Hey Jarvis, turn Spotify off" would publish the same
      setting. A decision of its own, since voiced would publish a setting.
