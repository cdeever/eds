---
title: "ADR-0049: A Device Holds Two Sites, and Is Told Which"
weight: 49
---

# ADR-0049: A Device Holds Two Sites' Settings, and Is Told Which to Use by a Message

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}}, until a device has been switched both ways and has undone a switch it could not complete; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-26 |
| **Scope** | What a device's `creds` partition holds for more than one site, the keys it is held under, the message that switches a device from one site to the other, and what a device does when the switch fails. Not what is in a site's settings, which is unchanged. |
| **Extends** | [ADR-0004](/docs/architecture/decisions/0004-firmware-secrets-nvs-only/), whose partition now holds a second site's secrets beside the first's, and [ADR-0035](/docs/architecture/decisions/0035-firmware-is-updated-through-the-broker/), whose topics, sender and undo-it-yourself rule a switch follows |
| **Related** | [ADR-0033](/docs/architecture/decisions/0033-the-whole-back-end-on-one-pi/), the backend Pi that is the second site; [ADR-0034](/docs/architecture/decisions/0034-presence-is-heard-not-remembered/), whose status says where a device is |

---

## Context

**EdS's devices now live at two sites.** The tenant's broker at Deevnet
IoTaaS's mobile site, on its IoT network, and the backend Pi at home, on the
home network. Each site has its own Wi-Fi, its own broker accounts and, until
the home Pi took a certificate from Deevnet's Tenant Service CA, its own CA.

**Moving a device between them is a walk to it with a cable.** Its six keys
in `creds` (ADR-0004) hold one site, and only `provision_tenant.py` over USB
writes them. A build sent over the air (ADR-0035) writes an app slot, never
`creds`. Four devices, moved twice, is eight visits.

**Since the home Pi's Deevnet certificate, both sites' brokers chain to
the Deevnet Root.** Everything else differs: the Wi-Fi network and its key;
the broker, which at home is the Pi's own (`mqtt.eds.deevnet.net`, answered
by the home network's DNS) and at the mobile site is Deevnet IoTaaS's
(`mqtt.mobile.deevnet.net`, the tenant's `broker_host`); and the broker
passwords, which the mobile site's API issues and the home Pi makes for
itself.

**Both networks can be in range at once**, while the mobile site is set up at
home, so which one a device uses cannot be decided by which it sees.

## Options considered

### Choosing the site

**By what is in range.** Nothing to send, but wrong when both are, which is
exactly when a device is moved.

**By a message.** A person says which, when it is time. The device has to be
able to undo it, or a switch to a site it cannot reach leaves it reachable by
nothing.

**By provisioning again.** What there is today.

### What a switch writes

**The site's secrets, sent in the message.** The device writes them over its
own. A message on the bus would then carry a Wi-Fi key and a broker password,
and the firmware would write its own secrets, which ADR-0004 keeps it from.

**Only the site's name.** Both sites' secrets are written by provisioning, as
now; the message names one, and the device writes which.

## Decision

**A device's `creds` partition holds two sites. A message names the site to
use; the device writes that name, reboots onto it, and goes back to the site
it came from if it has not reached that site's broker within three minutes.**

### The keys

Under the device's own namespace (`lp-stand`, `cover-screen`,
`power-beacon`, `ears`), beside the six it has today:

| Key | Holds | Written by |
|---|---|---|
| `site` | the name of the slot in use, `s1_name` or `s2_name` | provisioning; then the device |
| `site_prev` | the name in use before the last switch | the device |
| `s1_name`, `s2_name` | a site's name: `home`, `mobile` | provisioning |
| `s1_ssid`, `s2_ssid` | its Wi-Fi network | provisioning |
| `s1_wpass`, `s2_wpass` | its Wi-Fi key | provisioning |
| `s1_uri`, `s2_uri` | its broker, `mqtts://host:port` | provisioning |
| `s1_user`, `s2_user` | the device's account there | provisioning |
| `s1_mpass`, `s2_mpass` | that account's password | provisioning |
| `s1_ca`, `s2_ca` | the CA its broker is checked against | provisioning |

- **The six keys of today stay**, holding a copy of the site in use when the
  device was provisioned. A build from before this decision reads them and
  nothing else, so a device provisioned with two sites runs today's firmware
  unchanged, and a build that is undone (ADR-0035) still has a site to join.
- **A build that knows `site` reads the slot it names**, and the six only
  when `site` is missing.
- **The firmware writes `site` and `site_prev`, and nothing else.** No
  secret is ever written by the device: ADR-0004 stands.
- **Every value has the limit the six have** (ADR-0004 and the firmware's
  buffers). **The CA is held three times**, once in each slot and once in the
  copy, and three fit the 16 KB partition while each is under about 3.4 KB.
  The Deevnet Root is 1.9 KB. ESP-IDF's generator refuses an image that does
  not fit, and so does provisioning.

### The switch

- **Two topics per device**, beside its status topic, as ADR-0035's are
  (`eds/lightstand/lp-stand-01/…`, `eds/screen/…`, `eds/beacon/…`,
  `eds/voice/…`):
  - `…/site`: the switch, JSON: `{"v":1,"to":"home"}`.
  - `…/site/state`: the device's answers, JSON: on every connection
    `{"v":1,"state":"running","site":"home","sites":["home","mobile"]}`;
    `switching` with `to` before it reboots; `refused` with a `why` (a
    site it does not hold, or the one it is on); and `reverted` with
    `from` and a `why` once it is back on a site it switched away from.

  Both at QoS 1 and **neither retained**. A switch left retained on one
  site's broker would send the device away again every time it came back.
- **The sender is the `ota` account**, which may already run code on every
  device; one more topic adds nothing it could not do. `make site-switch
  DEVICE=lp-stand-01 TO=home` sends it, on the broker the device is on.
- **On a switch the device writes `site_prev`, then `site`, says
  `switching`, disconnects cleanly and reboots.** A reboot, not a Wi-Fi
  restart in place: the network, the broker client and its CA are all set up
  once, at start, and are set up again the way they are on every boot.
- **A site not reached in three minutes is undone, by the device.** After
  a switch it has three minutes from boot to reach the new broker. If it does
  not, it writes `site_prev` back as `site` and reboots onto the site it
  came from, and says `reverted` there. Three minutes is longer than a Wi-Fi
  join and a TLS handshake on a slow network, and longer than ADR-0035's two,
  so a build and a switch never undo each other.
- **The status says the site.** The console shows each device's `site`
  from `…/site/state`.

### Provisioning

`provision_tenant.py` takes a second site beside the first:
`--site home --also mobile=terraform`. The first is what it reads today (the
tenant's outputs, or `--outputs-json` and `--ca`), named; the second names
its own source: `terraform` for the tenant's Terraform, or a file of outputs,
with its CA after a comma. The first is the site in use. Without `--site`,
it writes the six keys alone, as it always has.

## Consequences

- **Moving a device is a message, not a visit**, once it holds both sites.
  The visit that writes them is the last one a site's settings need, until a
  site's Wi-Fi key or a broker password changes.
- **A device holds two sites' secrets.** Both are plaintext in flash, as one
  was. A device lost carries both networks' keys.
- **A second home Pi, or a new mobile password, is a visit.** Nothing here
  writes a secret over the air. That is the price of keeping ADR-0004.
- **The `ota` account can move a device off its site.** It could already
  replace its firmware.
- **Each device's account gains a subscription and a publish at both
  sites**, `…/site` and `…/site/state`, and the `ota` account a publish: in
  the tenant's Terraform and in the backend card's `accounts.conf`.
- **Today's firmware ignores the slots**, so provisioning with two sites
  can start before the firmware that switches is built, and that firmware
  arrives over the air on the stand, the screen and the beacon.
- **The reSpeaker takes no build over the air** (ADR-0035 is the other
  three's). Its switching build is flashed by cable once, and is best the
  one that also brings it ADR-0035's updates.

## What would settle it

- [ ] A device provisioned with both sites, on today's firmware, runs on the
      six keys as before.
- [ ] The switching build, sent over the air, reads the slot `site` names.
- [ ] `make site-switch` from home to mobile and back: each time `switching`,
      then `running` on the other broker with the right `site`.
- [ ] A switch to a site out of range is undone within three minutes, and
      `reverted` is said on the site it came back to.
- [ ] A switch to a site the device does not hold, or the one it is on, is
      `refused` with a `why`, and nothing is written.
