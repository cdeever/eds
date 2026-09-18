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
      version = "~> 0.1"
    }
  }

  # The substrate's state store (ADR-0007). Its credentials are outputs of the
  # tenant below, so it is configured after the first apply:
  # `make state-backend`, then `terraform init -migrate-state`.
  #
  backend "s3" {
    bucket       = "tf-state"
    key          = "tenants/eds/terraform.tfstate"
    region       = "us-east-1"
    endpoints    = { s3 = "http://tfstate.mobile.deevnet.net:9000" }
    use_lockfile = true

    skip_credentials_validation = true
    skip_region_validation      = true
    skip_requesting_account_id  = true
    skip_metadata_api_check     = true
    skip_s3_checksum            = true
    use_path_style              = true
  }
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
resource "deevnet_iot_wifi_key" "devices" {
  tenant      = deevnet_tenant.eds.name
  name        = "devices"
  trust_class = "iot"
}
