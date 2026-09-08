# The EdS tenant.
#
# Hosts the services behind the LP jacket stand: the palette extractor and
# lightd. It does NOT host the MQTT broker - see the README for why that is a
# forced choice rather than a preference.
#
# Unlike the reference pattern in ADR-0006, where the repository *is* the
# tenant, this tenant lives inside the EdS monorepo so the application and the
# infrastructure it runs on stay in one place. Everything else follows the
# pattern: one allocated index, no per-tenant decisions, no substrate commit.

terraform {
  required_version = ">= 1.5"

  # The substrate's state store (ADR-0007). The credential is scoped by the
  # server to this tenant's own prefix, and use_lockfile is what this buys over
  # state in a repository: two machines applying at once are refused rather
  # than silently overwriting one another.
  backend "s3" {
    bucket       = "tf-state"
    key          = "tenants/eds/terraform.tfstate"
    region       = "us-east-1"
    endpoints    = { s3 = "http://tfstate.mobile.deevnet.net:9000" }
    use_lockfile = true

    # MinIO, not AWS.
    skip_credentials_validation = true
    skip_region_validation      = true
    skip_requesting_account_id  = true
    skip_metadata_api_check     = true
    skip_s3_checksum            = true
    use_path_style              = true
  }

  required_providers {
    proxmox = {
      source  = "bpg/proxmox"
      version = "~> 0.101"
    }
    dns = {
      source  = "hashicorp/dns"
      version = "~> 3.4"
    }
  }
}

provider "proxmox" {
  endpoint  = replace(var.proxmox_url, "/api2/json", "")
  api_token = "${var.proxmox_token_id}=${var.proxmox_token_secret}"
  insecure  = true
}

# Publication credentials (ADR-0004). The key is scoped by the server to this
# tenant's zones, so an update aimed elsewhere comes back REFUSED.
provider "dns" {
  update {
    server        = var.dns_update_server
    key_name      = "${var.tsig_key_name}."
    key_algorithm = "hmac-sha256"
    key_secret    = var.tsig_key_secret
  }
}

locals {
  # Allocated once in the factory's TENANTS.md. Index 1 was released when tdemo
  # was destroyed on 2026-09-05.
  tenant_index = 1

  # ADR-0002's derivation, repeated here rather than read from the module's
  # outputs. Using module.tenant.subnet to build dns_extra_records would make
  # an input depend on an output of the same module - a cycle Terraform
  # refuses. The formula is stable and documented, so restating it is cheaper
  # than restructuring around the cycle.
  tenant_subnet = "10.${var.site_octet}.${128 + local.tenant_index}.0/24"

  # Workloads are addressed from .10 upward (there is no DHCP on an EVPN zone),
  # so the first VM - which runs both services - is .10.
  service_host = cidrhost(local.tenant_subnet, 10)
}

# The module is consumed by tag, not by path (ADR-0006). `terraform init`
# vendors it and neither plan nor apply re-fetches, so moving to a newer tag
# requires an explicit `terraform init -upgrade` - which is a feature and a
# trap in equal measure, since a repository that never re-inits stays on its
# old module indefinitely and quietly.
#
# Pinned to v1.1.0 rather than the v1.0.0 tdemo used. The only difference is a
# validation block rejecting a tenant_index outside 1-63, which is a guard rail
# with no interface change - there is no reason for a new tenant to start on
# the older tag.
module "tenant" {
  source = "git::ssh://git@github.com/deevnet/deevnet-tenant-factory.git//modules/tenant?ref=tenant-module-v1.1.0"

  tenant_name  = "eds"
  tenant_index = local.tenant_index

  # Issued by the substrate at onboarding, in fabric.auto.tfvars (ADR-0006).
  controller_id = var.controller_id
  node          = var.node

  # Site selectors are passed explicitly rather than left to the module's
  # defaults. EdS is a *home* audio platform being built on the mobile
  # substrate because that is the only fabric that exists; stating these here
  # makes a move to the home site a tfvars change instead of an edit to this
  # file.
  site_octet    = var.site_octet
  vrf_vni_base  = var.vrf_vni_base
  vnet_vni_base = var.vnet_vni_base
  dns_substrate = var.dns_substrate

  template_vm_id = var.template_vm_id
  ssh_keys       = var.ssh_keys

  # One VM runs both services. palette is a small FastAPI process and lightd is
  # a static Go binary holding one MQTT connection; neither justifies its own
  # machine, and splitting them would only add a network hop between two
  # things that always talk to each other.
  vm_count  = 1
  vm_cores  = var.vm_cores
  vm_memory = var.vm_memory

  # Service names, which is the half of the tenant contract the Proxmox IPAM
  # hook could never have provided: it registers an address at allocation time
  # and has no concept of a service name.
  dns_publish = true
  dns_extra_records = {
    palette = local.service_host
    lightd  = local.service_host
  }
}

output "subnet" {
  value = module.tenant.subnet
}

output "gateway" {
  value = module.tenant.gateway
}

output "vrf_vni" {
  value = module.tenant.vrf_vni
}

output "vnet_ids" {
  value = module.tenant.vnet_ids
}

output "vm_names" {
  value = module.tenant.vm_names
}

output "dns_zone" {
  value = module.tenant.dns_zone
}

output "dns_names" {
  value = module.tenant.dns_names
}

output "service_host" {
  description = "Address the palette and lightd services answer on."
  value       = local.service_host
}
