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

output "broker" {
  description = "Where MQTT clients dial, and what the broker's certificate is verified against."
  value = {
    host = var.broker_host
    port = 8883
    ca   = "site-ca.pem"
  }
}

output "lightd_broker" {
  description = "lightd's MQTT credential. The service config reads these."
  sensitive   = true
  value = {
    username  = deevnet_iot_broker_account.lightd.username
    password  = deevnet_iot_broker_account.lightd.password
    publish   = deevnet_iot_broker_account.lightd.granted_publish
    subscribe = deevnet_iot_broker_account.lightd.granted_subscribe
  }
}

output "lp_stand_01_broker" {
  description = "What the LP stand's firmware is flashed with, beside the Wi-Fi key above."
  sensitive   = true
  value = {
    username  = deevnet_iot_broker_account.lp_stand_01.username
    password  = deevnet_iot_broker_account.lp_stand_01.password
    publish   = deevnet_iot_broker_account.lp_stand_01.granted_publish
    subscribe = deevnet_iot_broker_account.lp_stand_01.granted_subscribe
  }
}

output "login" {
  description = "How to log in to the services VM, with a key in ssh_keys."
  value       = "ssh ${deevnet_workload.services.login_user}@${deevnet_workload.services.fqdn}"
}

# The services' whole environment, in the shape deevnet-kit and the Deploy
# Your App page expect: `terraform output -raw kit_env > kit.env`. The app's
# login is lightd's broker account.
output "kit_env" {
  sensitive = true
  value     = <<-EOT
    DEEVNET_TENANT=${deevnet_tenant.eds.name}
    MQTT_HOST=${var.broker_host}
    MQTT_PORT=8883
    MQTT_CA_FILE=site-ca.pem
    MQTT_USERNAME=${deevnet_iot_broker_account.lightd.username}
    MQTT_PASSWORD=${deevnet_iot_broker_account.lightd.password}
    LOG_ENDPOINT=${deevnet_tenant.eds.log_endpoint}
    LOG_INGEST_TOKEN=${deevnet_tenant.eds.log_ingest_token}
    LOG_READ_TOKEN=${deevnet_tenant.eds.log_read_token}
    LOG_SELECT_HEADER=${deevnet_tenant.eds.log_select_header}
    LOG_DEVICE_PARTITION=${deevnet_tenant.eds.index}-2
    GRAFANA_URL=${deevnet_tenant.eds.dashboard_url}
    GRAFANA_AUTH=${deevnet_tenant.eds.dashboard_username}:${deevnet_tenant.eds.dashboard_password}
    GRAFANA_ORG_ID=${deevnet_tenant.eds.dashboard_org_id}
    TF_VAR_grafana_org_id=${deevnet_tenant.eds.dashboard_org_id}
    GRAFANA_CA_CERT=site-ca.pem
  EOT
}
