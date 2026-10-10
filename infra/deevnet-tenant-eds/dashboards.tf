# The stands' dashboard, in EdS's Grafana organization (Deevnet ADR-0024).
#
# A stand publishes its key events to its MQTT log topic, the substrate's
# bridge carries them into this tenant's device log partition, and the
# organization the tenant was issued already has that partition wired in as
# the data source `deevnet-logs-devices`. What is declared here is only the
# dashboard that reads it.
#
# Kept in code because the platform treats Grafana's own database as
# rebuildable: a dashboard built by clicking does not come back, and this one
# does on the next apply.
#
# The login is the tenant's own, and the state holds it - EdS was readmitted
# after dashboards existed, so the API returned the password rather than the
# operator handing one over.

provider "grafana" {
  url     = deevnet_tenant.eds.dashboard_url
  auth    = "${deevnet_tenant.eds.dashboard_username}:${deevnet_tenant.eds.dashboard_password}"
  ca_cert = "${path.module}/deevnet-root-ca.pem"
}

# org_id is on every resource on purpose: under a username and password the
# provider ignores its own org_id setting and sends organization 1, which this
# tenant is not a member of.
resource "grafana_folder" "eds" {
  org_id = deevnet_tenant.eds.dashboard_org_id
  title  = "EdS"
}

resource "grafana_dashboard" "lp_stand" {
  org_id      = deevnet_tenant.eds.dashboard_org_id
  folder      = grafana_folder.eds.uid
  config_json = file("${path.module}/dashboards/lp-stand.json")
}
