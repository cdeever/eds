variable "proxmox_url" {
  type = string
}

variable "proxmox_token_id" {
  type = string
}

variable "proxmox_token_secret" {
  type      = string
  sensitive = true
}

variable "template_vm_id" {
  type        = number
  description = <<-EOT
    VMID of the Fedora template to clone. This changes every time the image
    factory rebuilds the template - Proxmox assigns the next free ID rather
    than reusing one - so it is a variable rather than a constant, and it needs
    updating after a rebuild.
  EOT
  default     = 100
}

variable "ssh_keys" {
  type        = list(string)
  description = "Public keys injected by cloud-init."
  default     = []
}

# --- Workload sizing ---------------------------------------------------------

variable "vm_cores" {
  type        = number
  description = <<-EOT
    Cores for the service VM. palette clusters a downscaled image - 160px on
    the long edge, so tens of milliseconds - and lightd is idle between
    records. Two is generous; it is here as a variable because the first thing
    that changes this is the album cover resolver arriving.
  EOT
  default     = 2
}

variable "vm_memory" {
  type        = number
  description = "Megabytes for the service VM. numpy and Pillow are the floor here."
  default     = 2048
}

# --- Site selection ----------------------------------------------------------
# Passed explicitly rather than left to the module's defaults, so relocating
# EdS from the mobile substrate to the home one is a tfvars change.
#
# This matters for EdS specifically: ADR-0008 records that only the mobile site
# has hosts, and addressing.md describes a "mobile co-located with home" WAN
# mode - the mobile rack travels. A turntable does not, so the lights stop when
# the rack leaves. Accepted for now; mobile is the only fabric that exists.

variable "site_octet" {
  type        = number
  description = "Second octet of the site block: 20 = dvntm (mobile), 10 = dvnt (home)."
  default     = 20
}

variable "vrf_vni_base" {
  type        = number
  description = "Site base for VRF VNIs (ADR-0002): 10000 = dvntm, 11000 = dvnt."
  default     = 10000
}

variable "vnet_vni_base" {
  type        = number
  description = "Site base for VNet VNIs (ADR-0002): 20000 = dvntm, 21000 = dvnt."
  default     = 20000
}

variable "dns_substrate" {
  type        = string
  description = "Site label in the zone name: mobile or home."
  default     = "mobile"
}

# --- DNS publication (ADR-0004) ----------------------------------------------

variable "tsig_key_name" {
  type        = string
  description = <<-EOT
    Name of this tenant's TSIG key. The substrate issued it at onboarding and
    bound it to this tenant's zones, so it is accepted for those and refused
    for every other tenant's. By convention it is the tenant name.
  EOT
  default     = "eds"
}

variable "tsig_key_secret" {
  type        = string
  sensitive   = true
  description = <<-EOT
    The shared secret for tsig_key_name, from the substrate vault
    (vault_tenant_tsig_keys). Never commit it: pass it as TF_VAR_tsig_key_secret
    like the Proxmox credentials, which is why there is no default.
  EOT
}

variable "dns_update_server" {
  type        = string
  description = <<-EOT
    Authoritative server that accepts the dynamic updates. This is the tenant
    DNS service in the substrate's identity VM, not the substrate resolver:
    workloads still *resolve* through the core router, which forwards these
    zones here. A name, so the service can move hosts without a tenant edit -
    the old 10.20.99.30 default broke when CHG-0008 moved it to Platform.
  EOT
  default     = "tdns.mobile.deevnet.net"
}

# --- Fabric attachment -------------------------------------------------------
# Issued by the substrate at onboarding and delivered as fabric.auto.tfvars:
#   make tenant-attachment TENANT=<path to this directory>
#
# No defaults on purpose. A tenant never invents these; if the file is missing,
# terraform should ask rather than guess (ADR-0006).

variable "controller_id" {
  type        = string
  description = "EVPN controller the tenant's zone attaches to. Issued by the substrate."
}

variable "node" {
  type        = string
  description = "Proxmox node the tenant's workloads land on. Issued by the substrate."
}
