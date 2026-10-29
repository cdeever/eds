# The EdS tenant.
#
# Hosts the services behind the LP jacket stand: the palette extractor and
# lightd. It does NOT host the MQTT broker - see the README for why that is a
# forced choice rather than a preference.
#
# Everything here goes through the Deevnet API (Deevnet ADR-0015): the tenant's
# network, its workload and its names. EdS holds one credential, its Deevnet
# token, and no Deevnet IoTaaS credential at all.
#
# Unlike the pattern in Deevnet ADR-0006, where the repository *is* the tenant, this
# tenant lives inside the EdS monorepo so the application and the
# infrastructure it runs on stay in one place. Nothing else about the pattern
# changes.

terraform {
  required_version = ">= 1.5"

  required_providers {
    deevnet = {
      source = "deevnet/deevnet"
      # 0.8 is the first with a certificate's sites.
      version = "~> 0.8"
    }
    # The console's key and its signing request: see the certificate below.
    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.0"
    }
    # The stands' dashboard: see dashboards.tf.
    grafana = {
      source  = "grafana/grafana"
      version = "~> 4.46"
    }
  }

  # No backend here. The first apply keeps state on this machine; after it,
  # `make state-backend` writes backend.tf for the state store Deevnet IoTaaS
  # issued this tenant (Deevnet ADR-0007), from this tenant's own outputs.
}

# DEEVNET_API_ENDPOINT, DEEVNET_API_TOKEN, DEEVNET_API_CACERT.
provider "deevnet" {}

resource "deevnet_tenant" "eds" {
  name = "eds"
}

# One VM runs both services. palette is a small FastAPI process and lightd is a
# static Go binary holding one MQTT connection; neither justifies its own
# machine, and splitting them would only add a network hop between two things
# that always talk to each other.
resource "deevnet_workload" "services" {
  tenant    = deevnet_tenant.eds.name
  name      = "services"
  cores     = var.vm_cores
  memory_mb = var.vm_memory_mb
  ssh_keys  = var.ssh_keys
}

# Service names, so firmware and operators address the service rather than the
# machine. The API publishes the workload's own name; these are the two beside
# it (Deevnet ADR-0015 §13).
resource "deevnet_dns_record" "palette" {
  tenant  = deevnet_tenant.eds.name
  name    = "palette"
  address = deevnet_workload.services.address
}

resource "deevnet_dns_record" "lightd" {
  tenant  = deevnet_tenant.eds.name
  name    = "lightd"
  address = deevnet_workload.services.address
}

# The one a person types: https://console.eds.<site zone>/ (ADR-0018).
resource "deevnet_dns_record" "console" {
  tenant  = deevnet_tenant.eds.name
  name    = "console"
  address = deevnet_workload.services.address
}

# The console's server certificate, from the site's Tenant Service CA
# (Deevnet ADR-0037). It chains to the Deevnet Root CA, which the browsers
# that open the console already trust.
#
# The key is made here and never leaves this tenant: it is in this tenant's
# state and on its workload, and only a signing request goes to the Deevnet
# API. The request is read for its key alone, so its subject is a label for
# whoever reads this, not what the certificate will say.
#
# CAREFUL: replacing the key replaces the certificate, and the old one is not
# revoked - it stays valid until it expires, wherever the old key still is.
#
# It lasts a year. From sixty days before it expires `make plan` shows this
# resource changing; apply, then `make -C deploy units` puts the new one on
# the workload. Nothing does that for you: plan at least every ten months.
resource "tls_private_key" "console" {
  algorithm   = "ECDSA"
  ecdsa_curve = "P256"
}

resource "tls_cert_request" "console" {
  private_key_pem = tls_private_key.console.private_key_pem

  subject {
    common_name = "console"
  }
}

resource "deevnet_certificate" "console" {
  tenant  = deevnet_tenant.eds.name
  name    = deevnet_dns_record.console.name
  csr_pem = tls_cert_request.console.cert_request_pem
}

# The home backend Pi's certificate (CHG-0042, Deevnet ADR-0041 §2). Unlike
# the console's, its key is made on the Pi and never leaves it: this takes the
# request `deevnet-kit cert request` wrote there, which is public and kept
# beside this file. `cert install` refuses a certificate that does not name
# mqtt, logs and grafana; console is the page.
#
# It lasts a year, and renewing is an apply for the same request. Then
# `terraform output -json backend_certificate` and `cert install` on the Pi.
# A new request (a card flashed again) is a new certificate.
resource "deevnet_certificate" "backend" {
  tenant  = deevnet_tenant.eds.name
  name    = "mqtt"
  names   = ["logs", "grafana", "console"]
  sites   = ["home", "mobile"]
  csr_pem = file("${path.module}/backend-pi.csr")
}

# The Wi-Fi credential EdS's devices are flashed with (Deevnet ADR-0012 §3, Deevnet CHG-0013).
#
# One key serves every device: the LP stand, and any later one. Deevnet IoTaaS
# does not know those devices individually, and a MAC binding would buy no
# enforcement while making a first flash wait on a Deevnet IoTaaS registration.
#
# EdS chooses the trust class and nothing else. `iot` is for devices whose
# firmware its owner controls, which is what the stand is. The SSID and the VLAN
# come back from the API - a tenant does not pick a VLAN, which is what keeps
# this tenant's network off the air.
#
# CAREFUL: replacing this resource issues a NEW key, and every device already
# flashed with the old one stops associating until it is reflashed. A lost API
# database does NOT do that - it restores this key from state.
#
# Beside it the tenant holds a `tenant_dev` key named `admission`, for the
# developer's computer on DVNTM-TD (Deevnet ADR-0029). It came with the admission and
# the API adopts it when the tenant is created, so it is not declared here.
resource "deevnet_iot_wifi_key" "devices" {
  tenant      = deevnet_tenant.eds.name
  name        = "devices"
  trust_class = "iot"
}

# The LP jacket stand, in the tenant's device registry (Deevnet ADR-0012 §3, Deevnet CHG-0014).
#
# The entry is an identity and nothing else: it carries no credential and
# grants no access. What it is FOR is the pairing rule below - a broker account
# for a device may only be issued against a device in trust class `iot`, which
# is the class for devices whose firmware its owner controls. That is the stand.
#
# The MAC is deliberately not set. It is a label for the owner's own inventory
# and Deevnet IoTaaS enforces nothing with it, so recording one here would only
# make a first flash wait on a registration.
resource "deevnet_iot_device" "lp_stand_01" {
  tenant      = deevnet_tenant.eds.name
  name        = "lp-stand-01"
  trust_class = "iot"
}

# lightd's own broker account (Deevnet ADR-0012 §3, §10; Deevnet CHG-0016).
#
# A WORKLOAD account: no device, because lightd runs on the services VM and
# reaches the broker over tenant_transit -> iot_backend, not from VLAN 30.
#
# Topic patterns are RELATIVE to the tenant. The API writes the "eds/" prefix
# itself, which is what confines this tenant to its own topics - so write
# "lightstand/+/scene", never "eds/lightstand/+/scene". They come back absolute
# in granted_publish / granted_subscribe, which is what the broker enforces.
#
# These are the permissions that lived in Deevnet IoTaaS's inventory as
# `mqtt_acls` until this resource existed. lightd publishes scenes to any stand
# and reads their presence; it never writes a stand's own status.
resource "deevnet_iot_broker_account" "lightd" {
  tenant = deevnet_tenant.eds.name
  name   = "lightd"

  publish = [
    "lightstand/+/scene", # a scene to any stand
    "lightd/status",      # its own liveness
  ]
  subscribe = [
    "lightstand/+/status",     # a stand's presence
    "lightstand/+/state",      # and what it is currently showing
    "nowplaying/current/art",  # the current cover, which it lights the stand from (ADR-0009)
    "nowplaying/current/look", # whether that cover has a look of its own (ADR-0019)
  ]
}

# The stand's own account, scoped to ITS OWN topics.
#
# Deliberately narrower than lightd's. A device credential that could write any
# stand's scene could repaint every stand in the house, and the IoT segment is
# rated Medium trust precisely because its devices are assumed reachable by
# things EdS did not write. The stand reads the scene meant for it and reports
# only on itself.
#
# CAREFUL: replacing this resource issues a NEW password, and the stand stops
# connecting until it is reflashed. A lost API database does NOT do that - it
# restores this account from state, the same way the Wi-Fi key above is
# restored (Deevnet ADR-0012 §5).
resource "deevnet_iot_broker_account" "lp_stand_01" {
  tenant = deevnet_tenant.eds.name
  name   = "lp-stand-01"
  device = deevnet_iot_device.lp_stand_01.name

  publish = [
    "lightstand/lp-stand-01/status",
    "lightstand/lp-stand-01/state",
    "log/lp-stand-01",                  # its own log lines, into the tenant's log store (Deevnet CHG-0021)
    "lightstand/lp-stand-01/ota/state", # its answers to a build being sent (ADR-0035)
  ]
  subscribe = [
    "lightstand/lp-stand-01/scene",
    "lightstand/lp-stand-01/ota",       # a build offered to it (ADR-0035)
    "lightstand/lp-stand-01/ota/chunk", # and the build, in pieces
  ]
}

# --- Now playing (ADR-0006) --------------------------------------------------
#
# Drivers each report one player; nowplayd picks the current track. Three kinds
# of account, each granted only its own side of the contract:
#
#   a driver   publishes its own source's three topics, and reads only the
#              commands meant for its own player (ADR-0020)
#   nowplayd   reads every source, and publishes the current track; passes a
#              controller's command to the source that is current
#   lightd     reads the current cover, above
#
# The broker does not report a publish or a subscription it refuses, so what
# is granted here has to match what the programs use, to the character.
# deploy/relay/card.sh checks the relay's side before it writes a card.

# nowplayd, on the services workload.
#
# The subscription is one filter covering every source, and it is granted as
# that filter: a subscription is matched against what was granted as a whole,
# so granting the three topics separately would not admit it.
resource "deevnet_iot_broker_account" "nowplayd" {
  tenant = deevnet_tenant.eds.name
  name   = "nowplayd"

  publish = [
    "nowplaying/current",      # the one current track
    "nowplaying/current/art",  # its cover
    "nowplaying/current/look", # whether that cover's album has a look of its own (ADR-0019)
    "nowplaying/status",       # its own liveness

    # ADR-0020. The cover again, small, for a subscriber that cannot take it
    # as it comes; and a controller's command, passed on to whichever source
    # is current. One pattern, because which source that is changes with the
    # music and is not known here.
    "nowplaying/current/art/small",
    "nowplaying/source/+/command",
  ]
  subscribe = [
    "nowplaying/source/+/+", # every driver's state, presence and cover
    "nowplaying/command",    # what a controller wants of whatever is current (ADR-0020)

    # ADR-0023. Whether the turntable is switched on, from the beacon on its
    # switch. Named, and not a pattern: nowplayd subscribes to the beacons its
    # settings name (NP_SOURCE_POWER) and to no others.
    "beacon/turntable-power-01/status",

    # ADR-0039. Which sources the stand is lit for: a retained setting the
    # console switches.
    "nowplaying/stand",
  ]
}

# voiced, on the services workload (ADR-0042). It hears every device's spoken
# requests, as one filter granted as that filter, and asks nowplayd for the
# album one names: play_album on the command topic, as a controller does
# (ADR-0040). What it made of a request is said back to the device. It reads
# nothing else, and cannot address a player directly.
resource "deevnet_iot_broker_account" "voiced" {
  tenant = deevnet_tenant.eds.name
  name   = "voiced"

  publish = [
    "voiced/status",      # its own presence, and its last will
    "nowplaying/command", # play_album, for an album asked for aloud
    "voice/+/heard",      # what was made of a request, to the device that sent it
  ]
  subscribe = [
    "voice/+/utterance", # what each device heard after its wake word, as a WAV
    "voice/+/woken",     # that one just woke: the music is turned down (ADR-0044)
  ]
}

# The console, on the services workload (ADR-0018).
#
# It looks: every topic it reads, and two it writes - its own presence, and
# which sources the stand is lit for (ADR-0039). Another control is a change
# to this list and a decision first.
#
# Each subscription is granted as the filter the console uses, to the
# character (services/console/internal/view, Topics).
resource "deevnet_iot_broker_account" "console" {
  tenant = deevnet_tenant.eds.name
  name   = "console"

  publish = [
    "console/status",
    "nowplaying/stand", # the stand's setting, retained (ADR-0039)
  ]
  subscribe = [
    "nowplaying/current",
    "nowplaying/current/art",
    "nowplaying/status",
    "nowplaying/source/+/state",
    "nowplaying/source/+/status",
    "lightstand/+/scene",
    "lightstand/+/state",
    "lightstand/+/status",
    "lightd/status",
    "beacon/+/status",  # whether each switch with a beacon on it is on (ADR-0023)
    "screen/+/status",  # whether each cover screen is there (ADR-0020)
    "nowplaying/stand", # the setting in force, heard back (ADR-0039)
  ]
}

# The agent Pi, as a device of this tenant (ADR-0017).
#
# The Pi is hardware Deevnet IoTaaS knows, but what runs on it is EdS's own
# image (ADR-0015): firmware its owner controls, which is what trust class
# `iot` is for. Registering it is what lets its account publish under log/,
# and so lets the agent report what it does beside the stand.
#
# As with the stand, the entry is an identity and carries no credential.
resource "deevnet_iot_device" "np_relay" {
  tenant      = deevnet_tenant.eds.name
  name        = "np-relay"
  trust_class = "iot"
}

# The agent's account, on the Pi with a leg on the home LAN (ADR-0010).
#
# One account for every driver on the Pi: each driver's three topics, and the
# Pi's own log topic. A further driver - MusicBee - adds its three here.
#
# CAREFUL: replacing this resource issues a NEW password, and the relay stops
# reporting until its card is rewritten (deploy/relay, `make card`).
resource "deevnet_iot_broker_account" "np_relay" {
  tenant = deevnet_tenant.eds.name
  name   = "np-relay"
  device = deevnet_iot_device.np_relay.name

  publish = [
    "nowplaying/source/itunes/state",
    "nowplaying/source/itunes/status",
    "nowplaying/source/itunes/art",
    # Spotify was reported without a cover (ADR-0016) and now has one
    # (ADR-0031). The art topic was granted from the start, so that changing
    # that decision was not also a change to this account.
    "nowplaying/source/spotify/state",
    "nowplaying/source/spotify/status",
    "nowplaying/source/spotify/art",
    # The turntable names no cover either (ADR-0021): what it hears has no
    # picture. Granted for the same reason as Spotify's.
    "nowplaying/source/turntable/state",
    "nowplaying/source/turntable/status",
    "nowplaying/source/turntable/art",
    "log/np-relay", # its own events, into the tenant's log store, as the stand's go
  ]
  # The one thing the agent is told rather than tells (ADR-0020): play, pause,
  # next, previous, louder, quieter, for iTunes and, since ADR-0032, for
  # Spotify. It arrives from nowplayd and from nowhere else, since no other
  # account may publish here.
  #
  # The turntable has none because there is nothing on it for one to reach.
  subscribe = [
    "nowplaying/source/itunes/command",
    "nowplaying/source/spotify/command",
  ]
}

# --- The reSpeaker (ADR-0042) -----------------------------------------------
#
# A microphone array and an ESP32-S3 that listens for its wake word and sends
# what followed it. Registered as the screen is: firmware its owner controls,
# and an entry that lets its account publish under log/.
resource "deevnet_iot_device" "respeaker_01" {
  tenant      = deevnet_tenant.eds.name
  name        = "respeaker-01"
  trust_class = "iot"
}

# Its account. What it says is audio, which voiced writes down and reads as a
# request; it cannot command anything itself. The worst this credential can
# do is ask for an album by speaking, which a person in the room can do too.
#
# CAREFUL: replacing this resource issues a NEW password, and the device stops
# connecting until it is reprovisioned.
resource "deevnet_iot_broker_account" "respeaker_01" {
  tenant = deevnet_tenant.eds.name
  name   = "respeaker-01"
  device = deevnet_iot_device.respeaker_01.name

  publish = [
    "voice/respeaker-01/status",    # its own presence, and its last will
    "voice/respeaker-01/utterance", # what followed the wake word, as a WAV
    "voice/respeaker-01/woken",     # that the wake word was just heard (ADR-0044)
    "log/respeaker-01",             # its own events, into the tenant's log store
  ]
  subscribe = [
    "voice/respeaker-01/heard", # what voiced made of it, for a light or a sound
  ]
}

# --- The cover screen (ADR-0020) ---------------------------------------------
#
# A small touch screen that shows the sleeve of what is playing, and has five
# buttons: play/pause, next, previous, louder, quieter.
#
# Registered as the stand is, and for the same reason: firmware its owner
# controls is what trust class `iot` is for, and the entry is what lets its
# account publish under log/. It is an identity and carries no credential.
resource "deevnet_iot_device" "cover_screen_01" {
  tenant      = deevnet_tenant.eds.name
  name        = "cover-screen-01"
  trust_class = "iot"
}

# The screen's own account.
#
# THIS IS THE FIRST ACCOUNT THAT CAN CHANGE WHAT PLAYS. Every other account in
# this file says what is so, or draws it; this one may say "next". It is a
# device credential on the IoT segment, which is rated Medium trust because
# its devices are assumed reachable by things EdS did not write - so what it
# may ask for is kept to one topic and five words, and the words are checked
# by nowplayd and again by the driver. The worst this credential can do is
# skip a track, pause it, or change the volume - up as well as down.
#
# It names no source. It publishes what it wants of whatever is current, and
# nowplayd, which knows what that is, passes it on. So it cannot address a
# player directly, and it reads only the current track and its small cover:
# not the sources, not the stand, not the full-size cover it could not hold.
#
# CAREFUL: replacing this resource issues a NEW password, and the screen stops
# connecting until it is reprovisioned.
resource "deevnet_iot_broker_account" "cover_screen_01" {
  tenant = deevnet_tenant.eds.name
  name   = "cover-screen-01"
  device = deevnet_iot_device.cover_screen_01.name

  publish = [
    "screen/cover-screen-01/status",    # its own presence, and its last will
    "nowplaying/command",               # what a hand on the screen asked for
    "log/cover-screen-01",              # its own events, into the tenant's log store
    "screen/cover-screen-01/ota/state", # its answers to a build being sent (ADR-0035)
  ]
  subscribe = [
    "nowplaying/current",               # what is playing, and whether it is
    "nowplaying/current/art/small",     # its sleeve, at a size the screen can hold
    "screen/cover-screen-01/ota",       # a build offered to it (ADR-0035)
    "screen/cover-screen-01/ota/chunk", # and the build, in pieces
  ]
}

# --- The turntable's power beacon (ADR-0023) ---------------------------------
#
# A bare board on the outlet that is switched with the turntable. It is
# connected while the turntable is on, and that is all it says: "online",
# retained, and the broker's "offline" for it when the switch goes off.
#
# Registered as the stand is, and for the same reason: firmware its owner
# controls is what trust class `iot` is for, and the entry is what lets its
# account publish under log/. It is an identity and carries no credential.
resource "deevnet_iot_device" "turntable_power_01" {
  tenant      = deevnet_tenant.eds.name
  name        = "turntable-power-01"
  trust_class = "iot"
}

# The beacon's own account. It reads nothing but a build sent to it.
#
# What this credential can do is say the turntable is on when it is not, or
# off when it is: nowplayd would hold the stand for a silent turntable, or
# stop hearing a record. Neither reaches a player.
#
# CAREFUL: replacing this resource issues a NEW password, and the beacon stops
# connecting until it is reprovisioned - which reads as the turntable never
# being switched on.
resource "deevnet_iot_broker_account" "turntable_power_01" {
  tenant = deevnet_tenant.eds.name
  name   = "turntable-power-01"
  device = deevnet_iot_device.turntable_power_01.name

  publish = [
    "beacon/turntable-power-01/status",    # its own presence, and its last will
    "log/turntable-power-01",              # its own events, into the tenant's log store
    "beacon/turntable-power-01/ota/state", # its answers to a build being sent (ADR-0035)
  ]
  subscribe = [
    "beacon/turntable-power-01/ota",       # a build offered to it (ADR-0035)
    "beacon/turntable-power-01/ota/chunk", # and the build, in pieces
  ]
}

# --- Updates through the broker (ADR-0035) -----------------------------------
#
# eds-ota, run on the services workload by `make ota` in a firmware directory.
# It offers a build to one device and reads that device's answers, and does
# nothing else. Each device is named, not matched by a pattern: a device added
# later is added here when it can take an update, and not before.
#
# What this credential can do is run code on every device it names. Its
# password lives where lightd's does, on the workload, readable by root.
resource "deevnet_iot_broker_account" "ota" {
  tenant = deevnet_tenant.eds.name
  name   = "ota"

  publish = [
    "lightstand/lp-stand-01/ota",
    "lightstand/lp-stand-01/ota/chunk",
    "screen/cover-screen-01/ota",
    "screen/cover-screen-01/ota/chunk",
    "beacon/turntable-power-01/ota",
    "beacon/turntable-power-01/ota/chunk",
  ]
  subscribe = [
    "lightstand/lp-stand-01/ota/state",
    "screen/cover-screen-01/ota/state",
    "beacon/turntable-power-01/ota/state",
  ]
}
