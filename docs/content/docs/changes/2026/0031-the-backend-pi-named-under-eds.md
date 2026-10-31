---
title: "CHG-0031: The Backend Pi Named Under EdS"
weight: 31
---

# CHG-0031: The Backend Pi Named Under EdS

| | |
|---|---|
| **Date** | 2026-10-20 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "planned" "Planned" >}} |
| **Systems** | `dv01rpi001p01` (the backend Pi, `192.168.2.40`); the home router's DNS. No device, nothing of the tenant's |
| **How** | DNS entries by the owner; `deevnet-kit names` and `regen-certs` on the Pi; `make -C deploy/backend push status` |
| **Risk** | Low. Most likely to go wrong: nginx not starting, which leaves the console unreachable until it is fixed or the console's settings are put back. The lights and the screen are not affected: they use the broker, which is not touched |
| **Decisions** | [ADR-0040](/docs/architecture/decisions/0040-a-backend-pis-services-are-named-under-its-app/), whose evidence this is |
| **Related** | [CHG-0028](/docs/changes/2026/0028-the-back-end-on-one-pi-on-dvntm/) |

---

## Summary

The Pi's services are named under `home.deevnet.net` directly, the console
holds 443, and Grafana is at `:3000`. After this change every service is
under `eds.home.deevnet.net`. nginx holds 80 and 443 and sends `grafana.…`
to Grafana and every other name to the console, both of which listen on
loopback only. The devices are not moved: they keep dialing
`mqtt.home.deevnet.net`. Their move waits for Deevnet ADR-0039, so that each
is provisioned once, with its final name and CA (EdS ADR-0040).

## Goal

- `https://grafana.eds.home.deevnet.net/` signs in, with no port.
- `https://console.eds.home.deevnet.net/` follows the music live.
- `:3000` does not answer from the Mac.
- `eds-backend-status` passes.

## Procedure

### Step 1: The names (owner)

In OPNsense, Unbound, each one pointing at `192.168.2.40`:
`mqtt.eds.home.deevnet.net`, `logs.eds.home.deevnet.net` and
`grafana.eds.home.deevnet.net` (`console.eds` is already there). Leave
`mqtt`, `grafana` and `logs` under `home.deevnet.net` alone: the devices still use them.

**Verify:** `dig +short` on each gives `192.168.2.40`.

### Step 2: The certificate (owner, on the Pi)

```bash
sudo deevnet-kit names    # what it carries now
sudo deevnet-kit names set 192.168.2.40,dv01rpi001p01.home.deevnet.net,mqtt.eds.home.deevnet.net,logs.eds.home.deevnet.net,grafana.eds.home.deevnet.net,console.eds.home.deevnet.net,mqtt.home.deevnet.net,grafana.home.deevnet.net,logs.home.deevnet.net
sudo deevnet-kit regen-certs
```

The old names stay, for the devices. `regen-certs` restarts the broker and the
log store; Step 3 restarts the rest.

### Step 3: Push

```bash
make -C deploy/backend push status
```

nginx is installed from Debian on this first push.

**Verify:** the Goal.

### Not in this change: the devices, and the old names

Moving the devices to `mqtt.eds.…` and then dropping `mqtt`, `grafana` and
`logs` under `home.deevnet.net` waits for Deevnet ADR-0039. Each device is
then provisioned once, by cable (an update over the air leaves `creds`
alone), with its final name and CA bundle. Until then the certificate
carries both sets of names.

## Rollback

nginx first, since the console wants 443 back. On the Pi:

```bash
sudo systemctl disable --now nginx
sudo rm /etc/systemd/system/grafana.service.d/eds.conf
sudo systemctl daemon-reload && sudo systemctl restart grafana
```

Then `make -C deploy/backend push` from the commit before ADR-0040. The new
names in DNS and in the certificate do no harm and stay.

## Log

| When | Step | What happened |
|---|---|---|
