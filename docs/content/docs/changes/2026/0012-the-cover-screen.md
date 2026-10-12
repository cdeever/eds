---
title: "CHG-0012: The Cover Screen"
weight: 12
---

# CHG-0012: The Cover Screen

| | |
|---|---|
| **Date** | 2026-10-11 |
| **Change type** | Deployment |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | a new device, `cover-screen-01`; the tenant's Terraform (one device, one broker account, grants on two others); `nowplayd` on the workload; the agent on the Pi |
| **How** | `make flash` and `make provision` in `firmware/cover-screen`; `make plan apply` in `infra/deevnet-tenant-eds`; `make -C deploy deploy WHERE=builder`; `make -C deploy/relay push` |
| **Risk** | Medium. It is the first change that lets anything alter what plays. Most likely to go wrong: the agent or `nowplayd` not coming back, which would stop the stand following the music |
| **Decisions** | [ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/), which this is the evidence for |
| **Related** | [CHG-0002](/docs/changes/2026/0002-relay-pi-and-now-playing-accounts/), which made the accounts this adds to |

---

## Summary

A touch screen is added beside the stand. It shows the cover of what is
playing and, when touched, five buttons: play or pause, previous, next,
volume up, volume down.

To get there, `nowplayd` starts publishing each cover a second time, small,
and starts passing commands to the source that is current; the agent's iTunes
driver starts carrying them out; and the broker is told to let each of them.

## Goal

- The board shows the cover of the track playing in iTunes, and a record
  when there is none.
- Each of its five buttons does what it says to the real iTunes.
- The stand, the console and the agent's reporting are as they were.

## Scope

**In scope:** the board's firmware and credentials; the tenant's Terraform;
`nowplayd`; the agent Pi's `npagent`.

**Out of scope:** `lightd`, `palette`, the console, the stand's firmware,
the Pi's image, Spotify.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| The board's own firmware is lost | the board | the whole flash is read to `firmware/cover-screen/backup/` before the first write |
| The plan changes something other than what was written | the tenant | the plan is read before it is applied: one device and one account added, two accounts changed in place, nothing replaced. A replaced account is a new password and a stand or a Pi that stops connecting |
| The substrate refuses a wildcard in a publish grant | the tenant | seen in the plan or the apply; `nowplayd` runs without it and commands simply go nowhere |
| `nowplayd` does not come back | the workload | the deploy reports it; the console shows the current track afterward |
| The agent does not come back, or stops reporting because its new subscription is refused | the Pi | the subscription is optional by construction and a refusal is only logged; `make -C deploy/relay status` and the console afterward |
| A command is carried out that nobody pressed | iTunes | commands are never retained, a retained one is ignored twice over, and the screen never queues one |

## Procedure

Ordered so that each step is safe without the ones after it.

1. **The board, with no network.** `make backup`, `make flash`. Check the
   picture is the right way up and in the right colors, and that touches
   land where the finger is; set the panel and touch settings until they do.
2. **The tenant.** `make plan`, read it, `make apply`.
3. **The workload.** `make -C deploy deploy WHERE=builder`.
4. **The Pi.** `make -C deploy/relay push`.
5. **The board's credentials.** `make provision`.

## Verification

- The board's corner mark goes out, and the cover of the track playing
  appears.
- Each button does what it says in iTunes, and the console shows the result.
- `eds/nowplaying/current/art/small` holds a JPEG of 240 pixels.
- The stand still follows a track change.
- The agent's log shows each command it carried out.

## Undo

- **The board:** write `backup/factory-*.bin` back at offset 0.
- **The Pi and the workload:** deploy from `main` again. Both run without
  the new grants.
- **The tenant:** remove the device and account and the added grants, and
  apply. Nothing else depends on them.

## Outcome

| When (UTC) | What happened |
|---|---|
| 22:52 | The board's whole flash was read to `firmware/cover-screen/backup/factory-20261011-185251.bin`, 4,194,304 bytes. It had been running the maker's LVGL demonstration, built on ESP-IDF 4.3. |
| 22:58 | The tenant was planned: 2 to add (the screen's device and account), 2 to change in place (`nowplayd`, `np-relay`), 0 to destroy. Nothing replaced. The apply was not run from the session that planned it. |
| 22:59 | The firmware (`cfad098`) was flashed. The board started without a fault: the panel and the touch controller both answered, it reported itself not provisioned, and it began trying `DVNTM-IOT` with no key. What is on the panel has not been looked at yet. |
| 23:05 | Its owner looked: the record, the right way up, the mark red. Seven presses on the five buttons each read within about 20 pixels of where the button is drawn, on the default touch settings. Nothing was changed. |
| 23:28 | The tenant was applied: 2 added, 2 changed, 0 destroyed. The substrate granted `eds/nowplaying/source/+/command` to publish, wildcard and all. |
| 23:30 | The services were deployed from the Builder. All four came back active. `nowplayd` connected, named its command topic, and published the track playing: Genesis, *Abacab*, from iTunes. |
| 23:30 | The agent was pushed to the Pi: `installed efcb197; npagent is active`. |
| 23:31 | The screen was provisioned. It joined `DVNTM-IOT` as 10.20.30.205, connected to the broker over TLS 1.4 seconds later, read the current track as playing with a cover, and received a small cover of 8,504 bytes. The decoder reported no fault. |
| 23:56 | Its owner pressed the buttons. **The first eight presses, made over ten seconds, reached `nowplayd` together**, within 100 ms: the screen's connection had stopped carrying anything and delivered them when it recovered. iTunes skipped two tracks at once. The screen's own events show the same ten seconds, each sent several times over. After that, each press arrived as it was made. |
| 23:57 | `play_pause` paused iTunes and resumed it, each within 20 ms of being passed on. `next` and `previous` changed the track. Volume commands were carried out without error. The agent logged every command, and both the screen's and the agent's events reached the log store. |
| 23:57 | Its owner reported the cover perfect, and the buttons turned around: top was next, bottom was previous. |
| 23:58 | Reflashed with the touch axes swapped and both turned around, as the defaults for this board. |
| 00:04 | Reflashed again: a command is now sent only when the broker has answered within three seconds or answers a question first, and WiFi power saving is off. It started, connected, and drew the cover of what was playing. Neither change has been tried by hand yet. |

### Departures from the plan

- The tenant was planned before the board was flashed, because the board's
  serial port was busy with the backup. Nothing was applied out of order.
- The apply was run from the session after all, at its owner's word, as a
  saved plan whose summary was checked against the one already read.
- The board was flashed three times, not once: the touch settings were wrong,
  and the stall found a fault in the design that was fixed the same evening
  (ADR-0020, "What building it found").
- Touch was first checked with no broker, by reading where presses landed. It
  was read wrongly: the presses matched the buttons' places on axes that were
  in fact swapped, because nobody recorded which button each press was. It
  took real commands to show it.

## Follow-ups

- [ ] ADR-0020 moves to `Accepted` when its six questions are answered here.
  Open: the buttons by hand on the corrected firmware, what the volume
  buttons sound like, and a day connected.
- [ ] Why the connection stalled for ten seconds is not known.
- [ ] A stall sends each of the screen's events several times: the client
  repeats an unacknowledged event, and the store keeps every copy. The
  stand's firmware does the same.
- [ ] `next` on an album's last track stops iTunes, and then `previous` and
  `next` do nothing until something is played. That is iTunes; the screen
  shows the record and gives no hint.
