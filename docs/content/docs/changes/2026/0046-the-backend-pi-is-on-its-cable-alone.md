---
title: "CHG-0046: The Backend Pi Is on Its Cable Alone"
weight: 46
---

# CHG-0046: The Backend Pi Is on Its Cable Alone

| | |
|---|---|
| **Date** | 2026-10-30 |
| **Change type** | Configuration |
| **Status** | {{< status-badge "complete" "Complete" >}} |
| **Systems** | The home backend Pi, `eds-backend`; the backend image (`images/backend`) and `deploy/backend/card.sh` |
| **How** | `nmcli` on the Pi, over its cable; the image's playbook and `card.sh` changed for the next card |
| **Risk** | Low. Most likely to go wrong: the session that makes the change riding the Wi-Fi it removes, which connecting to the wired address prevents |
| **Decisions** | None of its own. It is ADR-0053's "radio off by default" before that record's window is built, and CHG-0028's "one address at home" |
| **Related** | [CHG-0042](/docs/changes/2026/0042-the-home-pi-becomes-a-deevnet-pi/), whose card asked for Wi-Fi; [CHG-0028](/docs/changes/2026/0028-the-back-end-on-one-pi-on-dvntm/) |

---

## Summary

The backend Pi had two addresses on the home network: `192.168.2.40` on its
cable and `192.168.2.228` on Wi-Fi. Both were on DVNT-CORE. The Wi-Fi was
`deevnet-kit-wifi`, which deevnet-kit made on the first boot from the
network `make card` asked for. A second address on the same network bought
nothing, appeared in the kit's address warning, and could answer for
`eds-backend.local`.

After: the Pi has its cable alone, the Wi-Fi's connection and key are gone
from it, and its radio is off and stays off. The next card is the same: the
image starts with the radio off, and `make card` asks for no network.

## Procedure

### Step 1: The Pi

**Run**, over SSH to the wired address: `nmcli connection delete
deevnet-kit-wifi`, then `nmcli radio wifi off`.

**Verify:** the radio disabled, `wlan0` unavailable, one address, and
`WirelessEnabled=false` in `/var/lib/NetworkManager/NetworkManager.state`.

### Step 2: The next card

**Change:** `images/backend/config.yml` writes `NetworkManager.state` with
the radio off, and checks it at the end of the build; `card.sh` no longer
asks for a network; the README says the Pi is on a cable, and how to switch
Wi-Fi on by hand.

## Verification

`make status` ok, the console answering, and `eds-backend.local` resolving
to `192.168.2.40` alone.

## Undo

On the Pi: `sudo nmcli radio wifi on` and `sudo nmcli --ask device wifi
connect DVNT-CORE`. For a card: revert this change's commit.

## Outcome

| When (UTC) | Step | What happened |
|---|---|---|
| 2026-10-30 02:16 | 1 | Over `192.168.2.40`: `deevnet-kit-wifi` deleted, the radio off. `wlan0` unavailable; one address; `WirelessEnabled=false` saved |
| 2026-10-30 02:16 | 1 | `make status`: every check ok. The console answered 200; `eds-backend.local` resolved to `192.168.2.40` alone |
| 2026-10-30 | 2 | The playbook parsed and passed Ansible's syntax check; `card.sh` parsed. Not built: the Builder needs dvntm |

### Departures from the plan

- None.

## Follow-ups

- [ ] Build the image on a day on dvntm; its check reads the radio's state
      back.
