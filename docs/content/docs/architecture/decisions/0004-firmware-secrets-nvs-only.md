---
title: "ADR-0004: Firmware Secrets Are NVS-Only"
weight: 4
---

# ADR-0004: Firmware Secrets Are NVS-Only, and the Broker's CA Is Provisioned

| | |
|---|---|
| **Status** | {{< adr-status "Accepted" >}} |
| **Date** | 2026-10-10 |
| **Scope** | Where a device's WiFi key, broker password and broker CA come from. |
| **Related** | [CHG-0001](/docs/changes/2026/0001-lp-stand-joins-its-tenant/), [Provisioning a Stand](/docs/guide/provisioning/) |

---

## Context

The stand needs a WiFi key and a broker password, both issued by the EdS
tenant, and the CA that the broker's certificate is verified against. The
repository is public, and the firmware image gets built, cached and copied.

The first firmware took all of these from Kconfig, which meant `sdkconfig`
held live secrets and the image carried them. An interim step read them from
a dedicated `creds` NVS partition but kept the Kconfig options as fallbacks,
and embedded the CA in the image at build time.

## Options considered

**Kconfig only.** Simple; the secret is in `sdkconfig` and in every image.

**NVS first, Kconfig as fallback.** A provisioned stand carries nothing in
its image, but an option still exists to compile a password in, so the
guarantee depends on nobody using it.

**NVS only for secrets.** No option exists for either password. The Ma Bell
gateway already works this way.

For the CA: **embedded at build time**, or **provisioned beside the
credentials**. The substrate re-rooted its PKI twice in the weeks before this
decision; with an embedded CA each re-root is a rebuild and reflash of every
stand.

## Decision

- The **WiFi key and the broker password have no Kconfig option**. They are
  read from the `creds` partition and from nowhere else.
- The **broker's CA is provisioned** into the same partition. A certificate
  embedded under `LP_MQTT_USE_TLS` remains only as a fallback.
- What is not secret — the SSID, the broker URI, the account name — keeps a
  Kconfig default.
- `make provision` writes the partition from the tenant's Terraform outputs,
  so nothing is typed. `make wifi-config` writes it by hand for a stand that
  is not on the substrate.

## Consequences

- A firmware image carries no secret, and `sdkconfig` cannot hold one
  however it is edited.
- One image serves every stand. A stand is reprovisioned without being
  rebuilt, and a re-rooted PKI costs a reprovision.
- An unprovisioned stand still boots and lights its strip, with no keys. It
  says `not provisioned` on the console and cannot associate. Refusing to
  boot would be harder to diagnose.
- The secrets sit in plaintext flash. Per-device client certificates remain
  the intended end state.
- `make provision` checks that the account was granted the topics the
  firmware uses, because the broker would not say so.
