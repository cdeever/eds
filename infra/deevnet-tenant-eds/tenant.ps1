# EdS tenant, from Windows PowerShell 5.1 or 7: the Makefile's targets, with
# its guards, for a computer with no make, bash or python. README.md for the
# steps; `.\tenant.ps1 help` for the commands. On macOS, Linux and WSL2, use
# the Makefile. Keep the two alike, and both like deevnet-tenant-tdemo's,
# which this is a copy of (less `new`: this tenant is eds).
#
#   powershell -ExecutionPolicy Bypass -File tenant.ps1 <command> [-Auto]
#
# A tenant holds one credential: its Deevnet API token. There is no vault to
# read, no Proxmox credential to render, and no index to allocate.
#
# Plain ASCII on purpose: Windows PowerShell 5.1 reads a script with no BOM
# as the local code page.

param(
  [Parameter(Position = 0)][string]$Command = 'help',
  [switch]$Auto
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2
Set-Location $PSScriptRoot

# The API and the certificate it is served with, as the Makefile names them.
if (-not $env:DEEVNET_API_ENDPOINT) { $env:DEEVNET_API_ENDPOINT = 'https://api.mobile.deevnet.net:8080' }
if (-not $env:DEEVNET_API_CACERT)   { $env:DEEVNET_API_CACERT   = Join-Path $PSScriptRoot 'deevnet-root-ca.pem' }

# The state store's credentials, once state-backend has written them. Secret,
# gitignored, and needed by every command once state lives in the store. Set
# for this run only, as the Makefile's -include does.
function Import-BackendEnv {
  if (-not (Test-Path .backend.env)) { return }
  foreach ($line in Get-Content .backend.env) {
    if ($line -match '^\s*([A-Za-z_][A-Za-z0-9_]*)=(.*)$') {
      Set-Item -Path ("env:" + $Matches[1]) -Value $Matches[2].TrimEnd("`r")
    }
  }
}
Import-BackendEnv

$Tenant = 'eds'

function Fail([string[]]$Lines) {
  foreach ($l in $Lines) { [Console]::Error.WriteLine($l) }
  exit 2
}

function Invoke-Terraform {
  & terraform @args
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}

# Terraform reads UTF-8; Windows PowerShell 5.1's "UTF8" writes a BOM. LF, so
# the Makefile can read .backend.env too, under WSL2.
function Write-Plain([string]$Path, [string]$Text) {
  [IO.File]::WriteAllText((Join-Path $PSScriptRoot $Path), $Text.Replace("`r`n", "`n"), (New-Object Text.UTF8Encoding $false))
}

function Show-Help {
  Write-Host 'eds - tenant (inside the EdS monorepo)'
  Write-Host ''
  Write-Host '  init           terraform init'
  Write-Host '  plan           terraform plan'
  Write-Host '  apply          terraform apply    (-Auto to skip approval)'
  Write-Host '  destroy        terraform destroy  (-Auto to skip approval)'
  Write-Host '  state-backend  move state into the state store: writes backend.tf and'
  Write-Host '                 .backend.env from this tenant''s outputs, then migrates'
  Write-Host '  validate       fmt check + validate'
  Write-Host ''
  Write-Host 'Needs DEEVNET_API_TOKEN: the enrollment token on the first apply, then'
  Write-Host '  $env:DEEVNET_API_TOKEN = terraform output -raw api_token'
}

# The one credential. It is never stored here: a tenant token belongs in the
# state, and the enrollment token is single use.
function Assert-Token {
  if (-not $env:DEEVNET_API_TOKEN) {
    Fail @('DEEVNET_API_TOKEN is not set.',
           '  First apply:  $env:DEEVNET_API_TOKEN = ''<the enrollment token from the admission>''',
           '  After that:   $env:DEEVNET_API_TOKEN = terraform output -raw api_token')
  }
}

# Once backend.tf exists, every command reads state from the store, and that
# needs the credentials in .backend.env.
function Assert-Backend {
  if ((Test-Path backend.tf) -and -not (Select-String -Quiet -SimpleMatch "tenants/$Tenant/" backend.tf)) {
    Fail "backend.tf does not name tenants/$Tenant/."
  }
  if ((Test-Path backend.tf) -and -not (Test-Path .backend.env)) {
    Fail @('backend.tf is here but .backend.env is not: copy .backend.env from the computer',
           'that ran state-backend (it holds the state store''s credentials).')
  }
}

# The state store is offered, not mandated (Deevnet ADR-0007). Its credentials are
# outputs of the tenant, so it is set up after the first apply: backend.tf
# (not secret - committed) and .backend.env (secret, gitignored), then the
# local state moves in. scripts/state-backend.py does
# the same for the Makefile.
function Invoke-StateBackend {
  Assert-Backend
  if (Test-Path backend.tf) { Fail 'backend.tf already exists; state is already in the store.' }
  $json = & terraform output -json state_backend
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
  $d = ($json -join "`n") | ConvertFrom-Json

  Write-Plain backend.tf @"
# The state store Deevnet IoTaaS issued this tenant (Deevnet ADR-0007), written by
# ``make state-backend``. Not secret: the credentials are in .backend.env.
terraform {
  backend "s3" {
    bucket           = "$($d.bucket)"
    key              = "$($d.key)"
    region           = "us-east-1"
    endpoints        = { s3 = "$($d.endpoint)" }
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

"@

  # Readable and writable by this user alone: the Windows form of mode 0600.
  New-Item -ItemType File -Force -Path .backend.env | Out-Null
  & icacls .backend.env /inheritance:r /grant:r "${env:USERNAME}:(R,W)" | Out-Null
  if ($LASTEXITCODE -ne 0) { Fail 'could not restrict .backend.env to this user (icacls)' }
  Write-Plain .backend.env "AWS_ACCESS_KEY_ID=$($d.access_key)`nAWS_SECRET_ACCESS_KEY=$($d.secret_key)`n"
  Write-Host 'wrote backend.tf and .backend.env'

  Import-BackendEnv
  Invoke-Terraform init -migrate-state
  Write-Host 'State is in the store. Commit backend.tf; keep .backend.env safe - it is the key to the state.'
}

$approve = @()
if ($Auto) { $approve = @('-auto-approve') }

switch ($Command) {
  'help'          { Show-Help }
  'init'          { Assert-Backend; Invoke-Terraform init }
  'plan'          { Assert-Backend; Assert-Token; Invoke-Terraform plan }
  'apply'         { Assert-Backend; Assert-Token; Invoke-Terraform apply @approve }
  'destroy'       { Assert-Backend; Assert-Token; Invoke-Terraform destroy @approve }
  'state-backend' { Invoke-StateBackend }
  'fmt'           { Invoke-Terraform fmt -recursive }
  'validate'      { Invoke-Terraform fmt -check -recursive; Invoke-Terraform validate }
  default         { Show-Help; exit 2 }
}
