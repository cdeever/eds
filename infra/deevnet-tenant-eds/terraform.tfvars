# Site: dvntm (mobile). These are the module's defaults too, but EdS states
# them so that moving to the home site is a change here rather than an edit to
# main.tf. Home would be: site_octet 10, vrf_vni_base 11000,
# vnet_vni_base 21000, dns_substrate "home".
site_octet    = 20
vrf_vni_base  = 10000
vnet_vni_base = 20000
dns_substrate = "mobile"

# Public key injected into tenant workloads by cloud-init. Public half only.
ssh_keys = []
