---
title: "CHG-0007: The Console Over TLS"
weight: 7
---

# CHG-0007: The Console Over TLS

| | |
|---|---|
| **Date** | 2026-10-11 |
| **Change type** | Deployment |
| **Status** | {{< status-badge "complete" "Complete" >}} 2026-10-11 |
| **Systems** | the `eds` tenant: a certificate, held in its state; the `services` workload: `eds-console`, and its firewall |
| **How** | `terraform init -upgrade`, `terraform apply`; `make -C deploy deploy WHERE=builder` |
| **Risk** | Low. Most likely to go wrong: the console unable to read its key as the user it runs as, which stops it at start with the reason in its log |
| **Decisions** | [ADR-0018](/docs/architecture/decisions/0018-a-console-of-eds-own/); Deevnet ADR-0037, which gave tenants a CA to ask |
| **Related** | [CHG-0006](/docs/changes/2026/0006-the-console/); Deevnet CHG-0047, which built the Tenant Service CA this is the first use of |

---

## Summary

The console is served in the clear, because when it was built nothing on the
site would certify a name a tenant serves. The substrate now has a Tenant
Service CA (Deevnet ADR-0037), and a tenant can ask it for a server
certificate for a name in its own zone.

After this change the console is at `https://console.eds.mobile.deevnet.net/`,
with a certificate that chains to the Deevnet Root CA. Port 80 only sends
people there.

## Goal

- The tenant's state holds a key made here and a certificate for
  `console.eds.mobile.deevnet.net`, valid a year.
- `https://console.eds.mobile.deevnet.net/` verifies with the Deevnet Root CA
  alone, and shows what the plain page showed.
- `http://console.eds.mobile.deevnet.net/` redirects to it.
- The page still updates by itself.
- A second `terraform plan` shows nothing to change.

## Scope

**In scope:** the provider moved to 0.7; a key, a signing request and a
certificate in the tenant's Terraform; the console rebuilt with TLS; its key
and certificate on the workload; port 443 opened on the workload's firewall.

**Out of scope:** a login; controls; certificates for `lightd` and `palette`,
which nothing off the workload calls; renewing by anything but an apply.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| The provider upgrade changes something else | every resource in the tenant | the plan is read first, and must show three resources added and outputs changed, nothing else |
| The console cannot read its key | the workload | the key is installed owned by the user the container runs as; a console that cannot read it stops at start and says so, and the plain page is what was there before |
| The certificate's chain is incomplete | browsers | the console presents the leaf with what the API returned after it; checked with `openssl` against the root alone |
| The key reaches somewhere it should not | this repository, the Builder | it is written only to `deploy/tls/`, which is ignored, and copied only to the workload. Images are built on the Builder from sources, and the key is not a source |
| The certificate expires unnoticed | in a year | `make plan` shows the renewal from sixty days before; the console warns at every start inside thirty; noted as a follow-up with a date |

## Procedure

### Step 1: The certificate

```bash
terraform -chdir=infra/deevnet-tenant-eds init -upgrade
make -C infra/deevnet-tenant-eds plan
```

Expected: `tls_private_key.console`, `tls_cert_request.console` and
`deevnet_certificate.console` added. Then apply, and plan again.

### Step 2: The console

```bash
make -C deploy deploy WHERE=builder
```

Builds the images, writes the key and certificate from the tenant's state,
opens `https` in the workload's firewall, and restarts the units.

## Verification

- `consoled` logs `serving over TLS` with the name, a chain of three and the
  expiry.
- `openssl s_client` to port 443 verifies with `deevnet-root-ca.pem` alone.
- `curl -I http://console.eds.mobile.deevnet.net/` answers `308` to the
  `https` address.
- The page, in a browser, with no warning.

## Undo

- Remove `CONSOLE_TLS_CERT_FILE` and `CONSOLE_TLS_KEY_FILE` from
  `deploy/console.env` and run `make -C deploy units`: the console is back on
  port 80 in the clear.
- The certificate cannot be withdrawn. It stays valid for its year; its key
  is in this tenant's state and on its workload and nowhere else.

## Outcome

| When (UTC) | Step | What happened |
|---|---|---|
| 18:49 | 1 | Provider 0.7.0 installed from its release, its checksum checked. `init -upgrade` took it and `hashicorp/tls` 4.4.1. |
| 18:50 | 1 | The plan showed three resources added and the outputs changed, and nothing else from the upgrade. Applied. A second plan showed no changes. |
| 18:50 | — | The certificate read back: subject `O=Deevnet, OU=Mobile Tenant Services, CN=console.eds.mobile.deevnet.net`, issued by the Deevnet Mobile Tenant Service CA, one DNS name, server authentication only, not a CA, to 2027-10-11. Its chain is the Tenant Service CA and the Mobile Site CA. It verified for the name with the root alone. |
| 18:52 | 2 | Deployed. The key and certificate are on the workload, owned by the container's user and readable by no one else. `http` and `https` are open in the workload's firewall. Four units active. `consoled` logged `serving over TLS name=console.eds.mobile.deevnet.net chain=3 expires=2027-10-11 days_left=364`. |
| 18:53 | — | From the desk: TLS 1.3, verified with the root alone and with the desk's own trust store. The plain address answered `308` to the secure one. The picture and the event stream arrive over it. |

Every check under Verification holds, but the last was made with `curl`
against the desk's trust store, not by a person looking at a browser.

### Departures from the plan

- None in the procedure.
- This is the first certificate the site's Tenant Service CA issued to a
  tenant, a few minutes after the Deevnet API that asks for them was deployed
  (Deevnet CHG-0047).

## Follow-ups

- [ ] **Renew before 2027-10-11.** `make -C infra/deevnet-tenant-eds plan`
  shows it from 2027-08-12; apply, then `make -C deploy units`.
- [ ] A person opens it in a browser
  ([ADR-0018](/docs/architecture/decisions/0018-a-console-of-eds-own/), *What
  would settle it*).
- [ ] `lightd` and `palette` are still served in the clear, on the workload
  only. Nothing off the workload calls them.
