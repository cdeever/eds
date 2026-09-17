# EdS declares what its services run on, and nothing about the substrate.
#
# The index, subnet, gateway, VMID, MAC and address are the API's to issue
# (ADR-0015), so the site selectors this file used to carry - site_octet, VNI
# bases, the DNS substrate - are gone: they are the API's, and moving EdS to
# another site means pointing at that site's API.

variable "vm_cores" {
  type        = number
  default     = 2
  description = <<-EOT
    Cores for the service VM. palette clusters a downscaled image - 160px on
    the long edge, so tens of milliseconds - and lightd is idle between
    records. Two is generous; the first thing that changes it is the album
    cover resolver arriving.
  EOT
}

variable "vm_memory_mb" {
  type        = number
  default     = 2048
  description = "Memory for the service VM, in MB. numpy and Pillow are the floor here."
}

variable "ssh_keys" {
  type        = list(string)
  default     = []
  description = "Public keys for the workload's cloud-init account. Public halves only."
}
