---
title: "CHG-0001: The LP Stand Joins Its Tenant"
weight: 1
---

# CHG-0001: The LP Stand Joins Its Tenant

| | |
|---|---|
| **Date** | 2026-10-10 |
| **Change type** | Deployment |
| **Status** | {{< status-badge "complete" "Complete" >}} 2026-10-10 |
| **Systems** | `lp-stand-01` (ESP32-D0WD-V3); the `eds` tenant's Grafana organization |
| **How** | `make build flash provision` in `firmware/lp-stand`; `terraform apply` in `infra/deevnet-tenant-eds` |
| **Risk** | Low. Nothing issued is replaced. Most likely to go wrong: a broker account missing a topic the firmware uses, which fails silently |
| **Decisions** | [ADR-0004](/docs/architecture/decisions/0004-firmware-secrets-nvs-only/), [ADR-0005](/docs/architecture/decisions/0005-device-events-over-mqtt/) |
| **Related** | None |
| **Backfilled** | Written after the change ran, from the session that ran it. |

---

## Summary

The stand had been flashed once, on 2026-09-09, built for plaintext MQTT on
port 1883 and never provisioned. The substrate's broker is TLS only, so it had
never reached the real broker; it sat lit in its default warm white, retrying
WiFi with no key.

After this change it joins the tenant's device network, verifies the broker,
renders the retained scene, and reports its events to the tenant's log store,
where a dashboard reads them.

## Goal

- The stand boots `provisioned from creds`, with a WiFi key, a broker
  password and a provisioned CA.
- It holds an address on the IoT network and a TLS session to the broker as
  `eds-lp-stand-01`.
- `make logs` shows its events from the tenant's device partition.
- A dashboard **LP Stand** exists in the tenant's Grafana organization and
  its panels return the stand's data.

## Scope

**In scope:** the firmware on `lp-stand-01`; its `creds` partition; a Grafana
folder and dashboard in the tenant's organization; two new Terraform outputs.

**Out of scope:** the services on the workload; any change to what the
tenant was issued. The stand's account already had `log/lp-stand-01` granted.

## Procedure

### Step 1: Bring the tenant's state to the development machine

The state had only been used from the builder. `.backend.env`,
`deevnet-root-ca.pem` and `ssh.auto.tfvars` were copied into
`infra/deevnet-tenant-eds`, all gitignored, and `make init` run.

**Verify:** `terraform state list` shows the tenant's eight resources and a
plan proposes no resource change.

### Step 2: Build and flash

```bash
. ~/.espressif/tools/activate_idf_v6.1.sh
make build flash PORT=/dev/cu.usbserial-0001
```

### Step 3: Provision

```bash
make provision PORT=/dev/cu.usbserial-0001
```

Writes the WiFi key, the broker URI, the stand's account and the Deevnet Root
CA into the `creds` partition from the tenant's outputs.

**Verify:** the console shows
`provisioned from creds; … wifi key set, … mqtt key set, broker CA provisioned`,
then an address and `connected`.

### Step 4: Declare the dashboard

`dashboards.tf` and `dashboards/lp-stand.json`, then `terraform apply`.

**Verify:** the plan is two additions, a folder and a dashboard.

## Verification

- `make logs` printed `system.boot`, `wifi.connected`, `mqtt.connected` and
  `scene.applied` for `lp-stand-01`.
- Through Grafana, as the tenant's own login, the Restarts, WiFi signal and
  Stand events queries returned the stand's data.
- A `stand.health` event arrived five minutes after boot.

## Undo

- The dashboard: remove the two resources and apply.
- The stand: reflash the earlier image. Its `creds` partition can be erased
  or left; an image that does not read it ignores it.
- Nothing issued was replaced, so there is no credential to restore.

## Outcome

| When (UTC) | Step | What happened |
|---|---|---|
| 2026-10-10 15:1x | 1 | `make init` failed: `Failed to load state: unexpected end of JSON input`. The local `.terraform/` held a 0-byte backend record and a stale lock from an interrupted init on 2026-10-07. Both removed; init then succeeded. The store itself was never at fault. |
| 15:2x | 2 | Built on ESP-IDF 6.1; 40% of the app slot free. |
| 15:25 | 3 | Provisioned. The stand took `10.20.30.202`, connected over TLS and applied a retained scene: breathe, 4 colours. |
| 15:2x | 4 | Applied: 2 added, 0 changed, 0 destroyed. |
| 15:30 | — | First `stand.health`: 168 KB heap free, −34 dBm. |
| 15:32 | 2 | Reflashed with the password options removed from Kconfig ([ADR-0004](/docs/architecture/decisions/0004-firmware-secrets-nvs-only/)); the stand reconnected from its existing `creds` partition. |

### Departures from the plan

- The WiFi handler now formats an event on the system event task, whose
  default 2304-byte stack was sized for handlers that set a flag. Raised to
  4096 in `sdkconfig.defaults` before the first flash.
- The Kconfig default broker URI still said `mqtt://…:1883`. Changed to
  `mqtts://…:8883`.
- A reset by the reset pin reads as `reason=power` on this chip.

## Follow-ups

- [ ] `make wifi-config` uses a multi-line recipe that needs `.ONESHELL`,
  which macOS's make 3.81 lacks.
- [ ] `backend.tf` names the state store on port 9000; the tenant now reports
  the endpoint without a port. It still works.
- [ ] Several pages still call the broker `mqtt01`, a name that does not
  resolve.
