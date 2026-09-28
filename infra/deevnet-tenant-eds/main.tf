# The EdS tenant.
#
# Hosts the services behind the LP jacket stand: the palette extractor and
# lightd. It does NOT host the MQTT broker - see the README for why that is a
# forced choice rather than a preference.
#
# Everything here goes through the Deevnet API (ADR-0015): the tenant's
# network, its workload and its names. EdS holds one credential, its Deevnet
# token, and no substrate credential at all.
#
# Unlike the pattern in ADR-0006, where the repository *is* the tenant, this
# tenant lives inside the EdS monorepo so the application and the
# infrastructure it runs on stay in one place. Nothing else about the pattern
# changes.

terraform {
  required_version = ">= 1.5"

  required_providers {
    deevnet = {
      source  = "deevnet/deevnet"
      version = "~> 0.5"
    }
  }

  # No backend here. The first apply keeps state on this machine; after it,
  # `make state-backend` writes backend.tf for the state store the substrate
  # issued this tenant (ADR-0007), from this tenant's own outputs.
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
# it (ADR-0015 §13).
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

# The Wi-Fi credential EdS's devices are flashed with (ADR-0012 §3, CHG-0013).
#
# One key serves every device: the LP stand, and any later one. The substrate
# does not know those devices individually, and a MAC binding would buy no
# enforcement while making a first flash wait on a substrate registration.
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
# developer's computer on DVNTM-TD (ADR-0029). It came with the admission and
# the API adopts it when the tenant is created, so it is not declared here.
resource "deevnet_iot_wifi_key" "devices" {
  tenant      = deevnet_tenant.eds.name
  name        = "devices"
  trust_class = "iot"
}

# The LP jacket stand, in the tenant's device registry (ADR-0012 §3, CHG-0014).
#
# The entry is an identity and nothing else: it carries no credential and
# grants no access. What it is FOR is the pairing rule below - a broker account
# for a device may only be issued against a device in trust class `iot`, which
# is the class for devices whose firmware its owner controls. That is the stand.
#
# The MAC is deliberately not set. It is a label for the owner's own inventory
# and the substrate enforces nothing with it, so recording one here would only
# make a first flash wait on a registration.
resource "deevnet_iot_device" "lp_stand_01" {
  tenant      = deevnet_tenant.eds.name
  name        = "lp-stand-01"
  trust_class = "iot"
}

# lightd's own broker account (ADR-0012 §3, §10; CHG-0016).
#
# A WORKLOAD account: no device, because lightd runs on the services VM and
# reaches the broker over tenant_transit -> iot_backend, not from VLAN 30.
#
# Topic patterns are RELATIVE to the tenant. The API writes the "eds/" prefix
# itself, which is what confines this tenant to its own topics - so write
# "lightstand/+/scene", never "eds/lightstand/+/scene". They come back absolute
# in granted_publish / granted_subscribe, which is what the broker enforces.
#
# These are the permissions that lived in the substrate's inventory as
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
    "lightstand/+/status", # a stand's presence
    "lightstand/+/state",  # and what it is currently showing
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
# restored (ADR-0012 §5).
resource "deevnet_iot_broker_account" "lp_stand_01" {
  tenant = deevnet_tenant.eds.name
  name   = "lp-stand-01"
  device = deevnet_iot_device.lp_stand_01.name

  publish = [
    "lightstand/lp-stand-01/status",
    "lightstand/lp-stand-01/state",
    "log/lp-stand-01", # its own log lines, into the tenant's log store (CHG-0021)
  ]
  subscribe = [
    "lightstand/lp-stand-01/scene",
  ]
}
