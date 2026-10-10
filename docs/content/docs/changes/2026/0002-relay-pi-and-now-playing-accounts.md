---
title: "CHG-0002: The Relay Pi, and Accounts for Now Playing"
weight: 2
---

# CHG-0002: The Relay Pi, and Accounts for Now Playing

| | |
|---|---|
| **Date** | 2026-10-10 |
| **Change type** | Deployment |
| **Status** | {{< status-badge "complete" "Complete" >}} 2026-10-10 |
| **Systems** | the `eds` tenant's broker accounts; `dv02rpi002p01` (a Pi lab host in the site inventory); iTunes on a PC on the home LAN |
| **How** | `terraform apply` in `infra/deevnet-tenant-eds`; `make -C deploy/relay image fetch card status pair` |
| **Risk** | Medium. A host is put on two networks the site otherwise keeps apart. Most likely to go wrong: the Pi's Wi-Fi leg picking up a default route or DNS, which would send site traffic through the house |
| **Decisions** | [ADR-0006](/docs/architecture/decisions/0006-now-playing-bus-of-drivers/), [ADR-0010](/docs/architecture/decisions/0010-home-lan-relay/), [ADR-0011](/docs/architecture/decisions/0011-itunes-by-remote-pairing/), and, made during it, [ADR-0015](/docs/architecture/decisions/0015-eds-builds-its-own-pi-images/) |
| **Related** | [CHG-0001](/docs/changes/2026/0001-lp-stand-joins-its-tenant/) |

---

## Summary

iTunes runs on a PC on the home LAN. The broker is inside the Deevnet Mobile
site. Neither can reach the other, so nothing that happens in iTunes can reach
the stand.

After this change one of the site's Raspberry Pis, `dv02rpi002p01`, has its
Ethernet on the site's IoT segment and its Wi-Fi on the home LAN, and runs the
now-playing agent: it is paired with iTunes and publishes what iTunes is
playing to the broker.

This change stops at the broker. It does **not** deploy `nowplayd` to the
workload or switch `lightd` to the cover topic; until that is done, the
relay's reports arrive and nothing acts on them. That is the next change.

## Goal

- The tenant has broker accounts `eds-nowplayd` and `eds-np-relay`, and
  `eds-lightd` may subscribe to `eds/nowplaying/current/art`.
- `dv02rpi002p01` answers on its reserved wired address, `10.20.30.12`.
- `eds-agent-status` on the Pi passes every check: both legs up, the default
  route on `eth0`, none on `wlan0`, forwarding off, firewall loaded, broker
  and player reachable, agent running.
- From a machine on the site, `eds/nowplaying/source/itunes/state` shows what
  iTunes is playing, and changes within a second or two of a track change.
- The phone remote still works.

## Scope

**In scope:** three changes to the tenant's broker accounts; an image; one
card, in one Pi; one pairing in iTunes.

**Out of scope:** `nowplayd` and `lightd` on the workload; MusicBee; the other
three Pis; anything on the PC beyond what pairing needed.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| The Pi routes between the house and the site | `dv02rpi002p01` | `ip_forward` 0 and a firewall forward chain that drops everything; the image build fails without both, and `eds-agent-status` checks both on the running Pi |
| Site traffic leaves through the house | the Wi-Fi leg | the connection is written with `never-default`, `ignore-auto-dns` and `ignore-auto-routes`; `eds-agent-status` fails if `wlan0` carries a default route |
| The home LAN reaches into the site through the Pi | the Wi-Fi leg | inbound on `wlan0` is Bonjour and one pairing port; no SSH; nothing forwarded |
| A secret ends up in an image file | the Builder | none is baked in: the Wi-Fi key and broker account are written to the card after flashing, and the build fails if it finds either |
| `lightd`'s account is disturbed | the broker | the plan shows an in-place update adding one subscribe grant; no replacement, so no new password |
| A relay account that cannot publish | the broker | `card.sh` refuses to write a card whose account was not granted the agent's topics |

## Procedure

### Step 1: Declare the accounts

`deevnet_iot_broker_account.nowplayd` and `.np_relay`, and one more subscribe
grant on `.lightd`. Plan, read the plan, apply.

**Verify:** the plan is 2 to add, 1 to change, 0 to destroy, and the change is
an in-place update.

### Step 2: Build the image

The substrate's base image with EdS's agent layer applied offline
(`images/agent`).

```bash
make -C deploy/relay image    # cross-compile the agent; build on the Builder
make -C deploy/relay fetch    # the image and its checksum, here
```

*As planned this was a new variant in the substrate's image factory. See the
outcome.*

**Verify:** the build's own checks pass: no secret in the image, forwarding off
in both places, the firewall rules parse, `a_autoprov` present.

### Step 3: Flash and finish the card

Flash the image to a microSD with no customization. With the card still
mounted:

```bash
make -C deploy/relay card     # asks for the home Wi-Fi key
```

### Step 4: Boot

Card into `dv02rpi002p01`, Ethernet to the IoT segment, power.

```bash
make -C deploy/relay status
```

### Step 5: Pair

```bash
make -C deploy/relay pair     # type the code it prints into iTunes
```

## Verification

- `make -C deploy/relay status` exits 0.
- `mosquitto_sub` on `eds/nowplaying/source/itunes/#` from the site shows the
  state changing as tracks change.
- From a machine on the home LAN, the Pi's Wi-Fi address answers nothing on
  port 22.
- On the Pi, `ip route get 10.20.99.1` names `eth0`.
- The phone remote connects to iTunes.

## Undo

- The Pi: power it off, or reflash the card with another image. Nothing else
  depends on it.
- The pairing: iTunes offers only *Forget All Remotes* (Preferences →
  Devices), which also forgets the phone, so the phone would be paired again.
- The accounts: remove the two resources and the one grant, and apply. Nothing
  uses them until the next change.
- The image: it is a file. Nothing depends on it once a card is written.

## Outcome

| When (UTC) | Step | What happened |
|---|---|---|
| 17:3x | 2 | Built as a variant in the substrate's image factory: 37 tasks, none failed, and the build's checks passed. 528 MB compressed. |
| 17:4x | 1 | Applied: 2 added, 1 changed in place, 0 destroyed. Both new accounts then checked against the real broker: the relay's publish delivered, `nowplayd`'s one wildcard subscription admitted, and a publish outside the relay's grant not delivered. |
| ~18:2x | 3, 4 | Card flashed, settings written, Pi booted. It took its reserved address at once: its switch port was already on the IoT segment. Every check passed on first boot except the two that wait on pairing. |
| 18:2x | 5 | **No Remote button in iTunes.** Bonjour was not running on the Pi; see below. Fixed on the Pi. |
| 18:3x | 5 | A second attempt ran out its five minutes unanswered. The third paired. The login straight after was answered 503, which stopped the pairing script before it started the agent; started by hand, it logged in and published. |
| 18:38 | — | First report on the real broker: AC/DC, paused, with its cover. Then live changes within a second or two of each track change at home. |
| 18:4x | 2 | The image moved to the EdS repository ([ADR-0015](/docs/architecture/decisions/0015-eds-builds-its-own-pi-images/)). The factory was restored to `main` with nothing committed. The Pi was updated in place to match and kept its pairing. |

All of the goal's checks hold. The phone remote still works, with three
remotes now paired: the phone, the development Mac and the Pi.

### Departures from the plan

- **Bonjour would not start on the first card.** The playbook restricted it to
  `wlan0` with a line written `allow-interfaces = wlan0`, and avahi reads the
  key up to the `=`, spaces included. The daemon exited, the pairing
  advertisement never went out, and the pairing tool waited as if it had. The
  build's checks had not caught it, and neither had a read-only look inside
  the finished image, which noticed the spacing and wrongly took it to be
  tolerated. The playbook now writes the line without spaces and reads it
  back; the pairing tool now fails at once if its advertisement stops.
- **iTunes answers 503 for a few seconds after a pairing.** The pairing was
  good and saved; only the confirming login failed. The tool now retries it,
  and the Pi's pairing script starts the agent whenever a pairing exists.
- **The image is not where it was planned.** It was built as a factory
  variant, worked, and was moved to `images/agent` the same day. A relay is
  now an agent Pi given a second leg, and the card's name is a setting rather
  than part of the image.
- **The tools were renamed** from `eds-relay-*` to `eds-agent-*` with that
  move, on the running Pi as well.
- **The Pi's clock was in British time.** The base image's default. The image
  now sets UTC.
- Killing the stalled pairing from an SSH command line that itself contained
  the pattern killed the command instead, twice. Nothing was harmed; the
  second time it cost a rebuild that was being abandoned anyway.

## Follow-ups

- [ ] Deploy `nowplayd` to the workload and switch `lightd` to the cover
  topic. Until then nothing acts on what the relay reports.
- [ ] The agent Pi cannot report its own events to the log store: its account
  has no device, and only a device may publish under `log/`. Whether an EdS
  Pi should be a registered device is left open by ADR-0015.
- [ ] EdS's image build reaches into the factory's checkout for the base
  image. A base-image target in the factory would make that a contract.
- [ ] What the agent does when iTunes quits or the PC sleeps has not been
  watched on the Pi.
