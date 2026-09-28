#!/usr/bin/env python3
"""Write backend.tf and .backend.env from `terraform output -json state_backend` on stdin.

backend.tf is not secret: commit it in your own repository. .backend.env holds
the state store's credentials: mode 0600, gitignored, and needed by every
terraform command once the state lives in the store.
"""
import json
import os
import sys

d = json.load(sys.stdin)

with open("backend.tf", "w") as f:
    f.write(f'''# The state store the substrate issued this tenant (ADR-0007), written by
# `make state-backend`. Not secret: the credentials are in .backend.env.
terraform {{
  backend "s3" {{
    bucket           = "{d["bucket"]}"
    key              = "{d["key"]}"
    region           = "us-east-1"
    endpoints        = {{ s3 = "{d["endpoint"]}" }}
    custom_ca_bundle = "site-ca.pem"
    use_lockfile     = true

    # MinIO, not AWS.
    skip_credentials_validation = true
    skip_region_validation      = true
    skip_requesting_account_id  = true
    skip_metadata_api_check     = true
    skip_s3_checksum            = true
    use_path_style              = true
  }}
}}
''')

fd = os.open(".backend.env", os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
with os.fdopen(fd, "w") as f:
    f.write(f"AWS_ACCESS_KEY_ID={d['access_key']}\nAWS_SECRET_ACCESS_KEY={d['secret_key']}\n")
print("wrote backend.tf and .backend.env")
