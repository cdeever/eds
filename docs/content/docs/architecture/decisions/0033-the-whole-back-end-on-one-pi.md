---
title: "ADR-0033: The Whole Back End on One Pi"
weight: 33
---

# ADR-0033: The Whole Back End Can Run on One Pi, as an Image Built on Deevnet's Take-Home One

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until a card has been built, booted and run; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-13 |
| **Scope** | Whether EdS can run with nothing of Deevnet IoTaaS behind it, on one Raspberry Pi; what that Pi's image is built from; how its services run, where its secrets come from, and what its console presents. |
| **Extends** | [ADR-0015](/docs/architecture/decisions/0015-eds-builds-its-own-pi-images/): a second image, on a second base. [ADR-0003](/docs/architecture/decisions/0003-broker-is-the-substrates/): on Deevnet the broker is still Deevnet's; on this Pi it is the Pi's. [ADR-0010](/docs/architecture/decisions/0010-home-lan-relay/): this Pi has one leg and is no relay. [ADR-0018](/docs/architecture/decisions/0018-a-console-of-eds-own/): the console's certificate, where there is no Tenant Service CA. |
| **Related** | [ADR-0017](/docs/architecture/decisions/0017-an-agent-pi-is-a-registered-device/), [ADR-0027](/docs/architecture/decisions/0027-what-is-asked-of-outside-services-is-counted/), Deevnet's runbook "Convert a Tenant to a Pi Image" |

---

## Context

EdS runs in two places. Four services (`palette`, `lightd`, `nowplayd`, the
console) are containers on a workload in the EdS tenant. The agent, `npagent`,
is on a Pi with a leg on the home network
([ADR-0010](/docs/architecture/decisions/0010-home-lan-relay/)). The broker,
the log store and Grafana are Deevnet IoTaaS's.

That is right for the site, which travels. It is a lot for a house. The
owner wants to know whether the whole of it can be one Pi on the home
network: broker, logs, Grafana, the four services and the agent.

Deevnet IoTaaS supports the half of that which is its own. Its `pi-backend`
image is a Raspberry Pi that keeps the tenant's contract with nothing of
Deevnet behind it: Mosquitto on 8883, a log store on 8427, the bridge that
carries `<tenant>/log/<device>` into it, and Grafana on 3000 with the same
data source UIDs. A program on it, `deevnet-kit`, does what the tenant's
Terraform did: it makes the card's own CA and tokens on first boot, and adds
broker accounts under the same rules.

What Deevnet documents for the tenant's own programs is a container, loaded
onto the running Pi by hand. EdS has four services and an agent, seven
broker accounts, and an image build of its own.

Nothing here changes the tenant, its workload or the agent Pi. They keep
running. The devices stay with them until this Pi is proven.

## Options considered

**What the image is built on.**

- *Deevnet's plain base, as the agent image is, with EdS installing a broker
  and a log store itself.* Then EdS owns a broker's access rules, a log
  bridge and Grafana's provisioning, all of which Deevnet has already written
  and keeps in step with the site.
- *Deevnet's `pi-backend` image, built from the factory's checkout as the
  base is today.* That is ADR-0015's seam again, wider: the factory's
  Makefile, its staged inputs and its playbook, reached by path.
- *Deevnet's `pi-backend` image as Deevnet publishes it.* The factory puts
  the finished image and its checksum in the tenant downloads. Taking that
  file is a dependency on something Deevnet offers, not on how it is laid
  out.

**How the four services run on the Pi.**

- *Containers, as on the tenant and as Deevnet's runbook has it.* The same
  units as today. But `podman` cannot load an image into a card that is not
  running, so the four would be loaded on the first boot or by hand, and
  each costs memory on a machine that also runs Grafana.
- *Natively, as `npagent` already does.* The three Go services are static
  binaries and cross-compile as the agent does. `palette` is Python and gets
  a virtual environment, as the turntable's recognizer does. Everything is in
  the image when it is built. The cost is that a service is then run two
  ways, in a container on the tenant and as a unit on the Pi.

**Where the Pi's secrets come from.**

- *The tenant's*, carried over, so that a device needs only a new address and
  CA. Deevnet's runbook does this. It puts the tenant's passwords on a card,
  and makes the Pi depend on the tenant's state to be built.
- *Made on the card.* The devices have to be provisioned over a cable in any
  case, for the Wi-Fi network and the CA, and the provisioning writes the
  password in the same pass.

**What the console presents.** On the tenant its certificate is from the
site's Tenant Service CA
([ADR-0018](/docs/architecture/decisions/0018-a-console-of-eds-own/)). The
kit issues one server certificate, for the broker, the log store and Grafana,
and has no command to issue another.

- *Plain HTTP.* Nothing to trust. The console's page is then the one thing on
  the Pi that is not encrypted.
- *A certificate EdS signs with the card's CA key.* The kit says that key
  signs the server's certificate and nothing else, and EdS would be reaching
  past the kit to use it.
- *The kit's server certificate.* It names `<hostname>.local`, which is what
  a browser on the home network would type, and its key is readable by a
  group made for the services that present it.

## Decision

- **A second image, `images/backend`, whose base is Deevnet's published
  `pi-backend` image.** `images/build.sh` takes the file from the tenant
  downloads, checks it against the checksum beside it, and applies EdS's
  playbook offline. It builds nothing of Deevnet's and reaches into no
  checkout for this image.
- **The build refuses a base that cannot do what the card needs.** It asks
  the image's own `deevnet-kit` whether it can keep a password from a file
  and take a hostname from the card. An older one fails the build with the
  reason.
- **The four services and the agent run natively**, each as its own user
  with no login, in units held to what `npagent`'s is held to.
  `palette` and `lightd`'s HTTP ports are for the Pi itself and are not
  opened.
- **Every secret is made on the card.** A unit, `eds-backend-setup`, runs
  after the kit's first boot and adds the seven accounts with passwords it
  makes there. Nothing from the tenant is on the card, and the two share no
  secret. The image carries none, and its build checks.
- **The seven accounts are a list in the image**, `accounts.conf`, with the
  same names and patterns as the tenant's Terraform. That is one list in two
  places, and a check that reads both fails when they differ.
- **The tenant keeps its name and index** (`eds`, 2), so every topic, every
  account's username and the device log partition are what they are on
  Deevnet. No firmware is rebuilt.
- **The console serves the kit's server certificate**, on 443, with 80
  sending people there. A browser trusts it once the card's CA is trusted on
  that computer.
- **One leg, on the home network, and tried first on the kit's.** The Pi
  is no relay and routes nothing. Before it goes home it is proven on
  DVNTM with two legs, as the relay Pi has
  ([ADR-0010](/docs/architecture/decisions/0010-home-lan-relay/)): a cable
  on the IoT segment beside the devices, with the site's reservation for it
  (`dv02rpi003p01`, `10.20.30.13`), and Wi-Fi on DVNT-CORE, where iTunes is.
  It still forwards nothing between them. The tenant keeps running beside it
  throughout. Going home is pulling the cable, a reservation on the home
  router, and `deevnet-kit regen-certs` if the certificate lacks the home
  address; then each device is built and provisioned again for DVNT-CORE and
  the Pi.
  Its firewall is its own: SSH, the broker, the console, Grafana, the log
  store, and what iTunes needs to pair.
- **The Pi is its owner's, not the site's.** `pi-backend` has no automation
  user. The owner's login is made on first boot from the card, and the Pi is
  reached directly, not through the Builder.
- **Nothing of the agent image is changed.** The backend's playbook has its
  own copy of the few tasks it shares with the agent's, and installs the
  agent's tools from where they already are. Folding the two together waits
  until this Pi is proven.

## Consequences

- EdS can be built for a house with no Deevnet in it, and for the site, from
  one repository.
- **Two ways to run a service.** A change to how `lightd` is started is made
  in `deploy/eds-lightd.service` and in `images/backend/files`. The settings
  that are not secret are not copied: the image takes `deploy/*.env` as they
  are.
- **Eight accounts**: the seven services' and devices', and `ota`, which
  sends a build to a device (ADR-0035).
- **Three places hold the account list** when the console's subscriptions
  are counted: `view.Topics`, the tenant's Terraform, and `accounts.conf`.
  The check covers the last two.
- **The certificate carries both sites' names.** `deevnet-kit` 0.12.0
  takes extra names and addresses (`names=` on a new card, `deevnet-kit
  names` after), so a Pi on DVNTM can already be verified as
  `mqtt.home.deevnet.net` and at its home address, `192.168.2.40`.
- **The devices do not find the Pi by name.** The firmware has no mDNS, so a
  device is given the Pi's address, and the address has to stay put: a
  reservation on the router. The kit's certificate names the addresses the Pi
  had when it was issued; `deevnet-kit regen-certs` reissues it, and the
  console has to be restarted to present the new one.
- **Mosquitto is not VerneMQ.** A subscription outside an account's grant is
  accepted and delivers nothing, where Deevnet's broker refuses it. Either
  way nothing crosses, and the programs already treat a quiet subscription as
  a refused one.
- **The retained scene now rests on a card.** Deevnet's broker keeps it
  through a restart. Mosquitto writes what it retains to disk every half
  hour by default, so the image shortens that to a minute.
- **A card's CA has to be trusted by hand**, on each computer that opens the
  console, and it is a different CA on every card.
- **Dashboards are not carried over yet.** They are in the tenant's
  Terraform root and cannot be applied to a second Grafana from one state.
  Deevnet's own first test on a real Pi left Grafana's log plugin
  unregistered, so there may be nothing to apply them to.
- **EdS's build now depends on Deevnet publishing a current image.** The one
  published when this was written is too old (see below).
- The agent's unit, its tools and the recognizer are installed by two
  playbooks until they are folded together.

## What would settle it

On a Pi 4 or 5 with 4 GB or more, on DVNTM first and then on the home
network:

1. The image builds, and its own checks pass.
2. `deevnet-kit selftest` and `eds-backend-status` pass on the card.
3. Everything fits in memory, with Grafana running.
4. The agent's drivers report to the Pi's own broker, and `nowplayd`
   publishes the current track and its cover there.
5. A scene sent by hand to `lightd` is on the broker, retained, and is still
   there after the Pi's plug is pulled and put back.
6. The console's page opens at `https://<hostname>.local/` with the card's
   CA trusted.
7. A device connects. **Not known:** whether the firmware's TLS accepts a
   certificate matched by an IP address. If it does not, the device is given
   the name the home router has for the Pi, which the certificate also
   carries.
8. A device's events reach the Pi's log store.

Steps 7 and 8 move a device off the tenant, and are done last, when the
owner chooses.

## Evidence

**2026-10-13.** Written, and the image defined. Nothing has been built.

**2026-10-18, evening.** On DVNTM, then home. A card from the image on kit
0.12.0, with `names=`, booted with a certificate for both sites; the
self-test passed; iTunes was paired and played. The cable moved to DVNT-CORE
(`192.168.2.40`) and the Pi worked unchanged. **Each device moved by
re-provisioning alone, no build**: the firmware reads its network and broker
from `creds`. **A device verifies the card's certificate by name**
(`mqtt.home.deevnet.net`), which settles item 7; the beacon's events reached
the card's log store (item 8). Still open: a record end to end, and
`deevnet-kit` issuing certificates before the clock is set (the first card's
Discogs lookup failed for it until restarted). Details in CHG-0028.

**2026-10-18.** Picked up again, to be tried on DVNTM on `dv02rpi003p01`.
The definition was carried onto current `main`; `accounts.conf` gained the
update topics and the `ota` account (ADR-0035), which the check caught. Deevnet's
`pi-backend` is being rebuilt with `deevnet-kit` v0.11.0.

**2026-10-13.**

- **Deevnet's published `pi-backend` image is too old to build on.** It was
  built on 2026-09-24 and carries `deevnet-kit` v0.8.1, which stops on a
  `hostname=` or `wifi_*` line in the card's settings and cannot keep a
  password from a file. The kit's source is at v0.11.0. Restaging the kit
  and rebuilding the image is Deevnet's to do, and is an open follow-up of
  Deevnet CHG-0031.
