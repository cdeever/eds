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
  description = "For publishing names directly over RFC 2136, which EdS may still do (Deevnet ADR-0004)."
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
    ca   = "deevnet-root-ca.pem"
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

output "nowplayd_broker" {
  description = "nowplayd's MQTT credential. deploy/ writes it to the workload beside kit.env."
  sensitive   = true
  value = {
    username  = deevnet_iot_broker_account.nowplayd.username
    password  = deevnet_iot_broker_account.nowplayd.password
    publish   = deevnet_iot_broker_account.nowplayd.granted_publish
    subscribe = deevnet_iot_broker_account.nowplayd.granted_subscribe
  }
}

output "voiced_broker" {
  description = "voiced's MQTT credential (ADR-0042). deploy/ writes it to the workload beside kit.env."
  sensitive   = true
  value = {
    username  = deevnet_iot_broker_account.voiced.username
    password  = deevnet_iot_broker_account.voiced.password
    publish   = deevnet_iot_broker_account.voiced.granted_publish
    subscribe = deevnet_iot_broker_account.voiced.granted_subscribe
  }
}

output "console_broker" {
  description = "The console's MQTT credential. deploy/ writes it to the workload beside kit.env."
  sensitive   = true
  value = {
    username  = deevnet_iot_broker_account.console.username
    password  = deevnet_iot_broker_account.console.password
    publish   = deevnet_iot_broker_account.console.granted_publish
    subscribe = deevnet_iot_broker_account.console.granted_subscribe
  }
}

output "ota_broker" {
  description = "eds-ota's MQTT credential (ADR-0035). deploy/ writes it to the workload beside kit.env."
  sensitive   = true
  value = {
    username  = deevnet_iot_broker_account.ota.username
    password  = deevnet_iot_broker_account.ota.password
    publish   = deevnet_iot_broker_account.ota.granted_publish
    subscribe = deevnet_iot_broker_account.ota.granted_subscribe
  }
}

output "console_url" {
  description = "Where the console is, from the site's trusted networks."
  value       = "https://${deevnet_certificate.console.fqdn}/"
}

output "console_tls" {
  description = "The console's key and certificate. deploy/ writes them to the workload. The key is the secret."
  sensitive   = true
  value = {
    key       = tls_private_key.console.private_key_pem
    fullchain = deevnet_certificate.console.fullchain_pem
  }
}

output "console_certificate" {
  description = "When the console's certificate expires, and when a plan begins to renew it."
  value = {
    names       = deevnet_certificate.console.dns_names
    serial      = deevnet_certificate.console.serial
    not_after   = deevnet_certificate.console.not_after
    renew_after = deevnet_certificate.console.renew_after
  }
}

output "np_relay_broker" {
  description = "The relay agent's MQTT credential. deploy/relay writes it to a flashed card."
  sensitive   = true
  value = {
    username  = deevnet_iot_broker_account.np_relay.username
    password  = deevnet_iot_broker_account.np_relay.password
    publish   = deevnet_iot_broker_account.np_relay.granted_publish
    subscribe = deevnet_iot_broker_account.np_relay.granted_subscribe
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

output "respeaker_01_broker" {
  description = "The reSpeaker's MQTT credential (ADR-0042), for its provisioning."
  sensitive   = true
  value = {
    username  = deevnet_iot_broker_account.respeaker_01.username
    password  = deevnet_iot_broker_account.respeaker_01.password
    publish   = deevnet_iot_broker_account.respeaker_01.granted_publish
    subscribe = deevnet_iot_broker_account.respeaker_01.granted_subscribe
  }
}

output "cover_screen_01_broker" {
  description = "What the cover screen's firmware is provisioned with, beside the Wi-Fi key above (ADR-0020)."
  sensitive   = true
  value = {
    username  = deevnet_iot_broker_account.cover_screen_01.username
    password  = deevnet_iot_broker_account.cover_screen_01.password
    publish   = deevnet_iot_broker_account.cover_screen_01.granted_publish
    subscribe = deevnet_iot_broker_account.cover_screen_01.granted_subscribe
  }
}

output "turntable_power_01_broker" {
  description = "What the turntable's power beacon is provisioned with, beside the Wi-Fi key above (ADR-0023)."
  sensitive   = true
  value = {
    username  = deevnet_iot_broker_account.turntable_power_01.username
    password  = deevnet_iot_broker_account.turntable_power_01.password
    publish   = deevnet_iot_broker_account.turntable_power_01.granted_publish
    subscribe = deevnet_iot_broker_account.turntable_power_01.granted_subscribe
  }
}

# EdS's own log store credentials (Deevnet ADR-0027, Deevnet CHG-0020).
#
# The stand does NOT use these: it publishes its events to MQTT, and
# Deevnet IoTaaS's bridge carries them into this tenant's device partition. These
# are for reading them back - firmware/lp-stand/tools/stand_logs.py does - and
# they are the same values kit_env hands the services for their own logs.
output "log_store" {
  description = "Reading this tenant's logs, and writing from anything it runs itself."
  sensitive   = true
  value = {
    endpoint      = deevnet_tenant.eds.log_endpoint
    account_id    = deevnet_tenant.eds.log_account_id
    select_header = deevnet_tenant.eds.log_select_header
    ingest_token  = deevnet_tenant.eds.log_ingest_token
    read_token    = deevnet_tenant.eds.log_read_token
  }
}

output "dashboard" {
  description = "Where the stands' dashboard is. The password is in kit_env's GRAFANA_AUTH."
  value = {
    url      = deevnet_tenant.eds.dashboard_url
    org_id   = deevnet_tenant.eds.dashboard_org_id
    username = deevnet_tenant.eds.dashboard_username
    path     = "/d/eds-lp-stand"
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
    MQTT_CA_FILE=deevnet-root-ca.pem
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
    GRAFANA_CA_CERT=deevnet-root-ca.pem
  EOT
}
