---
title: "ADR-0053: A Home Pi Opens a Maintenance Leg for the Van"
weight: 53
---

# ADR-0053: A Home Pi Opens a Maintenance Leg for the Van, for a While, When Asked From Inside

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}}: until a window has been opened, used and closed on the backend Pi; see [What would settle it](#what-would-settle-it). It takes effect when the Pi's cable leaves DVNT-CORE |
| **Date** | 2026-10-28 |
| **Scope** | How the mobile site reaches an EdS Pi that lives at home: a Wi-Fi leg onto the home LAN, off unless opened, and what it admits. The day-2 jobs that use it are named here and decided elsewhere. |
| **Extends** | [ADR-0010](/docs/architecture/decisions/0010-home-lan-relay/): a second leg, fenced, now off by default and opened for a time. [ADR-0033](/docs/architecture/decisions/0033-the-whole-back-end-on-one-pi/): the backend Pi, now at home. |
| **Related** | [CHG-0042](/docs/changes/2026/0042-the-home-pi-becomes-a-deevnet-pi/), which moves the backend Pi home. [CHG-0028](/docs/changes/2026/0028-the-back-end-on-one-pi-on-dvntm/), which chose one address at home. [ADR-0052](/docs/architecture/decisions/0052-each-pis-itunes-pairing-is-kept-encrypted-in-the-repository/), one of the jobs. Deevnet ADR-0011, ADR-0013, ADR-0020 and ADR-0041, and Deevnet's network segmentation standard. |

---

## Context

The backend Pi has moved home (CHG-0042), and others may follow. At home
it is on the home LAN, DVNT-CORE (`192.168.2.0/23`, behind the home
OPNsense). Its certificate, its tenant's Terraform and the tools that
look after it are on the mobile site, dvntm.

The mobile site is the contractor's van. It rolls in for a job, does the
work and leaves. The first job is the certificate: a Deevnet Pi's lasts a
year, and renewing it takes a request from the Pi, a signature from the
tenant's Terraform (reachable only from dvntm), and an install on the Pi.
Deevnet ADR-0041 (Proposed) has the Pi go back to the factory for this, or
the request carried on a USB stick. A van that can reach the Pi does it in
one visit, with nothing carried by hand. Others will follow: revoking a
certificate, patching, `make push`, a pairing restored (ADR-0052).

**Today the van needs no window.** The backend Pi's cable is on
DVNT-CORE, and since CHG-0046 its radio is off and it has that one address.
A van on DVNT-CORE reaches it directly. That ends when Deevnet's home site
is built and the Pi's cable moves onto one of its segments: then the Pi's
wired leg is on a network the van is not on, and whether the house routes
the one to the other is the house's business, which cannot be counted on.
This record is for then.

**The van, in practice, is the Builder.** Deevnet's Builder is dual-homed by
design: "Upstream, it joins whatever external network is available (home,
hotel, office)", and downstream it sits on the site's management segment
(Deevnet's *Builder & Core Services*). It is site `00`, the roaming
appliance, named in whichever site zone it is attached to (Deevnet
ADR-0008). Away from dvntm it is out of reach today: `builder` names its
management address, `10.20.99.95`, which a Mac at home cannot reach, so an
EdS image cannot be built from home.

**DVNT-CORE is not a Deevnet network.** Deevnet's home site (`dv01`, its
trusted VLAN 10 and SSID `DVNT`) is designed and not built, and DVNT-CORE
appears in no Deevnet repository. Deevnet's rules do not govern it. Its
reasoning still applies, and it argues against what this record does:

- A trusted segment holds "known, managed devices" (Deevnet's standard,
  §2), and a tenant device attaches by its trust class (Deevnet ADR-0011
  §3).
- A host with a leg on two segments carries traffic between them without
  the router's policy. Deevnet rejected it for VMs (ADR-0013 §2) and for a
  backend's second NIC (ADR-0020), because "`ip_forward=0` is a default,
  not a boundary."
- Deevnet's home site is deliberately stricter than its mobile one: it has
  no trusted-to-management exception.

EdS has been here before. The relay Pi has a leg on the home LAN
([ADR-0010](/docs/architecture/decisions/0010-home-lan-relay/)), accepted
as "the operator's call", with SSH closed on that leg. CHG-0028 went the
other way at home and kept the backend Pi to one address.

## Options considered

**Rely on the house's routing.** The van joins DVNT-CORE and reaches the
Pi's wired leg through the home router. Nothing new on the Pi, but the
van's access depends on a policy EdS does not own, written for other
reasons, and gone the day the Pi's wired leg moves.

**Return to the factory.** Deevnet ADR-0041's way: the Pi, or its request
on a USB stick, goes to dvntm. A visit a year, by hand, and nothing for
the other jobs.

**A Wi-Fi leg on DVNT-CORE, always on.** ADR-0010's shape. The van reaches
the Pi whenever it is there, and the Pi is on two networks all the time,
which is the exposure Deevnet's records describe, for a need that arises a
few times a year.

**The van brings its own network**, and the Pi's Wi-Fi joins it. The house's
LAN never holds a tenant device. But a Pi that dials a network that may or
may not be there needs to know when to look, which is the same question
as when to open a window, and the van's network is one more thing to
build.

**A Wi-Fi leg on DVNT-CORE, off unless opened, opened from inside, closed
by itself.** The exposure of the always-on leg, for the length of a job.

## Decision

**The Pi's Wi-Fi is a maintenance port onto DVNT-CORE. It is off. Someone
already inside opens it for a set time; it closes itself; it admits SSH and
nothing else; and every opening is logged.** The house lets the contractor
in.

- **The radio is off by default**, and turned off on every boot. A reboot
  always closes the window. The first half is already so: CHG-0046 turned
  the backend Pi's radio off, and its image starts with it off.
- **It is opened from inside, over SSH on the wired side**:
  `eds-maint open [minutes]` on the Pi. A window that is closed cannot be
  opened from the van, and that is the point: the owner lets the van in.
- **A window lasts 60 minutes unless asked for longer, and never more than
  8 hours.** A timer on the Pi enforces it. `eds-maint close` ends it early;
  `eds-maint status` says whether it is open and for how long.
- **Open, the Wi-Fi joins DVNT-CORE as a leg, not a route**, with the
  settings the relay's leg has (`images/agent/files/eds-agent-import`):
  `never-default`, `ignore-auto-dns`, `ignore-auto-routes`, IPv6 off. The
  Pi's own default route and DNS stay on the wired side.
- **The firewall on `wlan0` admits SSH and nothing else.** No broker, no
  console, no log store, no Grafana; forwarding is dropped, as on every
  EdS Pi.
- **Each window is an event**: `maint.opened`, with its length and the user
  who opened it, and `maint.closed`, saying whether it timed out, was
  closed or was ended by a reboot. In the journal, and in the log store.
- **The DVNT-CORE key is the card's**, written by `make card` and imported
  on the first boot, as the relay's is.
- **This is for DVNT-CORE.** Once Deevnet's home site is built, a leg onto
  its trusted VLAN is Deevnet's network and Deevnet's decision, and needs a
  record there first.

### The van's side: the Builder's upstream leg on DVNT-CORE

During a maintenance window, **the Builder's upstream leg joins DVNT-CORE**:
the use Deevnet already gives that leg, at the house. The Builder seems to
have a Wi-Fi radio it does not use, which would let it join without a
cable; that is to be confirmed. Then:

- **The Mac at home reaches the Builder**, on its DVNT-CORE address, so an
  image is built and fetched from home.
- **The Builder reaches the home Pis**: over their cable while it is on
  DVNT-CORE, and through the Pi's window once it is not. The day-2 jobs
  below run from it.
- **Its management leg is not exposed by this.** The upstream leg is the
  one Deevnet already treats as outside the site, and nothing inside the
  site depends on it.

How the Builder's upstream leg is set up, which radio, and what the Mac
calls it on DVNT-CORE are Deevnet's to decide, in a record of its own.

### What the van does through it

The window is a way in. What runs through it is day-2 work, as Ansible
over SSH from the van, and each job is its own decision:

- **Renewing the certificate** first: `deevnet-kit cert request` on the
  Pi, the tenant's Terraform signs it on the van, `cert install` on the
  Pi. Then **revoking or replacing** one.
- Patching, `make push`, and a pairing restored (ADR-0052).

Deevnet's own day-2 certificate roles are its precedent
(`opnsense_cert`, `proxmox_node_cert`). None covers a Pi that has gone
home. Where each playbook belongs is open: a certificate is the kit's and
Deevnet's, the services are EdS's. Most likely the certificate's
playbook belongs in Deevnet's collection, and EdS's services stay with
EdS's `make`.

## Consequences

- **The van reaches a home Pi only when invited**, and only for SSH.
- **For the length of a window, the Pi is on two networks.** That is the
  exposure Deevnet's records describe. It is accepted for a short window
  that closes itself and admits one port, as ADR-0010 accepted it for the
  relay, and it is visible in the log store each time.
- **Opening needs the wired side.** A Pi whose wired leg is down cannot be
  opened; this is not out-of-band access. A button on the Pi is the answer
  if that is ever wanted.
- **The relay's always-on leg is not changed** by this record.
- **The Pi needs its DVNT-CORE key on the card.** CHG-0046 took the Wi-Fi
  question out of `make card`; the window brings it back, written as a
  connection that never connects by itself, so the radio stays off until a
  window opens.
- **A certificate renewed by the van needs no visit to the factory**, which
  answers Deevnet ADR-0041's "a visit a year" for a Pi the van can reach.

## What would settle it

- [ ] The backend Pi boots with its radio off.
- [ ] `eds-maint open 15` over the wired side: the Pi joins DVNT-CORE, a
      laptop on DVNT-CORE reaches it by SSH, and nothing else on `wlan0`
      answers.
- [ ] The window closes by itself at fifteen minutes. One opened and the Pi
      rebooted is closed after the reboot.
- [ ] `maint.opened` and `maint.closed` in the log store, with who and how
      long.
- [ ] A certificate renewed by the van through a window.
- [ ] The Builder's upstream leg on DVNT-CORE: the Mac at home reaches it,
      and `make -C deploy/backend image fetch` runs from home.

## Follow-ups

- [ ] The day-2 playbooks: renewing and revoking a certificate first, then
      patching, push and a pairing restored. Decide which live in Deevnet's
      collection and which in EdS.
- [ ] Opening a window from the console or the broker: a topic, a grant,
      and who may send it.
- [ ] A Deevnet record, before Deevnet's home site is built, for a tenant
      Pi's maintenance leg onto its trusted VLAN.
- [ ] A Deevnet record for the Builder's upstream leg at the house: whether
      its unused Wi-Fi joins DVNT-CORE during a maintenance window, and
      what the Mac calls it there. First, confirm the Builder has the
      radio.
