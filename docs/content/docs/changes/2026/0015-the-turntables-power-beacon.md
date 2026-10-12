---
title: "CHG-0015: The Turntable's Power Beacon"
weight: 15
---

# CHG-0015: The Turntable's Power Beacon

| | |
|---|---|
| **Date** | 2026-10-12 |
| **Change type** | Deployment |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | a new device, `turntable-power-01`; the tenant's Terraform (one device, one broker account, a grant on two others); `nowplayd` and the console on the workload |
| **How** | `make flash` and `make provision` in `firmware/power-beacon`; `make plan apply` in `infra/deevnet-tenant-eds`; `make -C deploy deploy WHERE=builder` |
| **Risk** | Low to medium. `nowplayd`'s rule for what is current changes, for the turntable only and only once its beacon has been heard. Most likely to go wrong: `nowplayd` or the console not coming back, which would stop the stand following the music or blank the page |
| **Decisions** | [ADR-0023](/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand/), which this is the evidence for |
| **Related** | [CHG-0013](/docs/changes/2026/0013-the-turntable-driver/), whose follow-up about a lifted needle this answers; [CHG-0012](/docs/changes/2026/0012-the-cover-screen/), the last device added, and the shape this follows |

---

## Summary

A bare ESP32-S2 goes on the outlet that is switched with the turntable. It
is connected to the broker while the turntable is on and says nothing else.

`nowplayd` is told the beacon speaks for the turntable: while it is
connected the turntable is current, through its silences, and while it is
not the turntable's driver is not heard. The console shows the beacon beside
the other parts.

## Goal

- Switching the turntable on puts `online` on
  `eds/beacon/turntable-power-01/status` within a few seconds, and switching
  it off puts `offline` there within about fifteen.
- With the turntable on and iTunes left paused, lifting the needle leaves
  the stand lit from the record's sleeve.
- With the turntable off, what is current is what it would be with no
  turntable.
- The console shows the beacon, lit when the turntable is on.
- The stand, the cover screen and the agent's reporting are as they were.

## Scope

**In scope:** the beacon's firmware and credentials; the tenant's Terraform;
`nowplayd`; the console.

**Out of scope:** `lightd`, `palette`, the stand's and the screen's
firmware, the agent Pi and its drivers, the audio path, and where the Pi and
its sound card are moved to.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| The wrong board is written | the Mac's serial port, which is the one the cover screen takes | `make flash` and `make provision` ask the chip what it is first and stop unless it is an ESP32-S2 |
| The plan changes something other than what was written | the tenant | the plan is read before it is applied: one device and one account added, two accounts changed in place, nothing replaced. A replaced account is a new password and a program that stops connecting |
| `nowplayd` is refused its new subscription | the broker | the subscription is optional by construction: a refusal is a warning in its log, and the turntable is arbitrated as before |
| The console is refused its new subscription | the broker | the page shows no beacon; nothing else on it depends on that topic. The grant and `view.Topics` are written the same, to the character |
| `nowplayd` or the console does not come back | the workload | the deploy reports it; the console shows the current track afterward |
| The beacon's connection comes and goes | upstairs, on WiFi | each loss is fifteen seconds before it is believed; its events count them, and the setting can be removed |

## Procedure

Ordered so that each step is safe without the ones after it.

1. **The board, with no network.** `make build flash`. It starts, says it is
   not provisioned, and reaches nothing.
2. **The tenant.** `make plan`, read it, `make apply`.
3. **The board's credentials.** `make provision`. Its presence appears on
   the bus; nothing reads it yet.
4. **The workload.** `make -C deploy deploy WHERE=builder`.
5. **The outlet.** The board is moved to the strip's switched outlet.

## Verification

- The board's serial log says `provisioned from creds` and that it
  connected; the `uptime_ms` of its first `mqtt.connected` event is how long
  the room waits.
- `eds/beacon/turntable-power-01/status` reads `online` with the turntable
  on, and `offline` about fifteen seconds after it is switched off.
- `nowplayd`'s log names the switch each time it changes.
- The console shows `turntable-power-01`, lit and unlit with the turntable.
- A needle lifted with iTunes paused: the stand keeps the sleeve.

## Undo

- **The workload:** remove `NP_SOURCE_POWER` from `deploy/nowplayd.env` and
  `make -C deploy units`. The turntable is arbitrated as before, with the
  beacon still on the bus. Or deploy from `main` again.
- **The board:** unplug it. Its will says `offline`, which with the setting
  in place means the turntable is never heard; remove the setting first.
- **The tenant:** remove the device, the account and the two added grants,
  and apply. Nothing else depends on them.

## Outcome

| When (UTC) | What happened |
|---|---|
| 10:50 | The board its owner plugged in was asked what it is: an ESP32-S2, revision v0.0, single core, WiFi only, 4 MB of flash behind a CP2102N bridge, secure boot and flash encryption off. Suitable. |
| 11:20 | The firmware was built for it with ESP-IDF 6.1 and flashed. It started without a fault, reported itself not provisioned, and began trying `DVNTM-IOT` with no key. |
| 11:38 | The tenant was planned: 2 to add (the beacon's device and account), 2 to change in place (`nowplayd`, `console`), 0 to destroy. Nothing replaced. The apply was left to its owner. |
| 11:48 | Its owner had applied the tenant. The beacon was provisioned from the tenant's outputs, as account `eds-turntable-power-01`, and restarted three times from its reset pin, on the bench. Each time it had an address about 1.7 s after starting and had said `online` at 3.03, 3.07 and 3.06 s. |
| 11:50 | The work was committed (`73b6e94`) and the services deployed from the Builder. All four came back active. `nowplayd` named the switch as it started, `source=turntable on=true`, and published what was current: iTunes, paused. The console's page listed `turntable-power-01`, on. |
| 11:52 | The board was held in reset from the Mac, twice, to stand in for its power going, and the console watched. It read `offline` after 12.3 s and after 18.6 s. Let go, it read `online` again after 7.6 s and after 3.1 s. |
| 11:54 | Its owner had reconnected the sound card and the turntable's driver was online again, silent. With the beacon on, `nowplaying/current` was the turntable, stopped, over the track still paused in iTunes: the hold. The board held in reset, current went to iTunes, paused, after 15.5 s; let go, back to the turntable after 3.2 s. |

## Departures

- The turntable's driver on the Pi was offline when the services were
  deployed, and with the switch on and no driver iTunes stayed current,
  which is the rule. The hold was seen once the driver was back. A record
  actually played and its needle lifted has still not been.
- The will took up to 18.6 s, not "about fifteen". One and a half keepalives
  is when a broker may send it, and this one looks every few seconds.

## Follow-ups

- [ ] The evidence [ADR-0023](/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand/#what-would-settle-it)
  asks for.
- [ ] The cover screen is not on the console either; it could be shown the
  way the beacon now is.
- [ ] ADR-0021 and the relay's README still describe the Pi beside the
  amplifier. They are to be brought up to date when the Pi has moved.
