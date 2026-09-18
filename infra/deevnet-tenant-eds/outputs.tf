output "subnet" {
  description = "EdS's overlay subnet, issued with its index."
  value       = deevnet_tenant.eds.subnet
}

output "service_host" {
  description = "Address palette and lightd answer on."
  value       = deevnet_workload.services.address
}

output "names" {
  value = [
    deevnet_workload.services.fqdn,
    deevnet_dns_record.palette.fqdn,
    deevnet_dns_record.lightd.fqdn,
  ]
}

output "state_backend" {
  description = "Values for the backend block, and the credentials it needs."
  sensitive   = true
  value = {
    endpoint   = deevnet_tenant.eds.state_endpoint
    bucket     = deevnet_tenant.eds.state_bucket
    key        = "${deevnet_tenant.eds.state_key_prefix}terraform.tfstate"
    access_key = deevnet_tenant.eds.state_access_key
    secret_key = deevnet_tenant.eds.state_secret_key
  }
}

output "dns_publication" {
  description = "For publishing names directly over RFC 2136, which EdS may still do (ADR-0004)."
  sensitive   = true
  value = {
    server        = deevnet_tenant.eds.dns_update_server
    zone          = deevnet_tenant.eds.dns_zone
    reverse_zone  = deevnet_tenant.eds.dns_reverse_zone
    key_name      = deevnet_tenant.eds.tsig_key_name
    key_algorithm = deevnet_tenant.eds.tsig_algorithm
    key_secret    = deevnet_tenant.eds.tsig_secret
  }
}

output "device_wifi" {
  description = "What EdS's devices are flashed with. Do not hardcode the SSID - it differs by site."
  sensitive   = true
  value = {
    ssid = deevnet_iot_wifi_key.devices.ssid
    psk  = deevnet_iot_wifi_key.devices.psk
    vlan = deevnet_iot_wifi_key.devices.vlan
  }
}

output "api_token" {
  description = "EdS's own API token. Every apply after the first uses it."
  sensitive   = true
  value       = deevnet_tenant.eds.api_token
}
