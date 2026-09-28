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
  description = <<-EOT
    Public keys that may log in as the workload's login_user (`tenant`).
    Public halves only. Keys are written when the VM is built, so changing
    this list means `terraform apply -replace=deevnet_workload.services`.

    EdS is the operator's own tenant, so the substrate's a_autoprov key is on
    this list deliberately: it lets the Builder's automation reach the VM. A
    tenant someone else owns would never carry it (ADR-0028).
  EOT
}

variable "broker_host" {
  type        = string
  default     = "mqtt.mobile.deevnet.net"
  description = <<-EOT
    The substrate's MQTT broker. A variable rather than a hardcoded string
    because it is a SUBSTRATE name, not EdS's - moving EdS to another site
    means pointing at that site's broker.

    It is not an attribute of the tenant because the API does not issue one
    today; every other substrate value EdS uses (the DNS server, the state
    endpoint, the Wi-Fi SSID) comes back from the API, and this is the one
    that still has to be named here.

    NOT mqt01 or mqtt01: those names do not resolve. The broker is a VerneMQ
    container on the messaging VM (CHG-0015) and its certificate is valid for
    this name and for the host's own.
  EOT
}
