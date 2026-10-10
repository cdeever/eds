# The state store the substrate issued this tenant (Deevnet ADR-0007), written by
# `make state-backend`. Not secret: the credentials are in .backend.env.
terraform {
  backend "s3" {
    bucket           = "tf-state"
    key              = "tenants/eds/terraform.tfstate"
    region           = "us-east-1"
    endpoints        = { s3 = "https://tfstate.mobile.deevnet.net:9000" }
    custom_ca_bundle = "deevnet-root-ca.pem"
    use_lockfile     = true

    # MinIO, not AWS.
    skip_credentials_validation = true
    skip_region_validation      = true
    skip_requesting_account_id  = true
    skip_metadata_api_check     = true
    skip_s3_checksum            = true
    use_path_style              = true
  }
}
