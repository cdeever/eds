#!/bin/bash
# devices.sh - fetch what the stand, the screen and the beacon are provisioned
# with, from a backend Pi.
#
# On Deevnet IoTaaS a device's Wi-Fi key, its broker account and the broker's
# CA come from the tenant's Terraform outputs. On a backend Pi the accounts
# and the CA were made on the card (ADR-0033), and the Wi-Fi is the house's.
# This writes the same shape the firmware's provisioning tools already read
# in place of those outputs:
#
#   devices.json          the broker, each device's account, and the Wi-Fi
#   deevnet-kit-ca.pem    the card's CA
#
# Both are gitignored. devices.json holds passwords and the Wi-Fi key.
#
# Provisioning a device moves it from the tenant to this Pi. Nothing here
# does that; it only fetches what doing it needs.
set -euo pipefail

SSH="${SSH:?}"; REPO="${REPO:?}"
HERE="$(cd "$(dirname "$0")" && pwd)"
umask 077

fetched="$(mktemp)"
trap 'rm -f "$fetched"' EXIT
ssh "$SSH" 'sudo eds-backend-setup devices' > "$fetched"
ssh "$SSH" 'cat /etc/deevnet-kit/tls/ca.pem' > "$HERE/deevnet-kit-ca.pem"
grep -q 'BEGIN CERTIFICATE' "$HERE/deevnet-kit-ca.pem" || { echo "The Pi's CA did not arrive." >&2; exit 1; }

read -r -p "Wi-Fi network the devices join [DVNT-CORE]: " ssid
ssid="${ssid:-DVNT-CORE}"
read -r -s -p "Key for $ssid: " psk; echo
if (( ${#psk} < 8 || ${#psk} > 63 )); then
    echo "A WPA key is 8 to 63 characters; that was ${#psk}." >&2
    exit 2
fi

# The devices have no mDNS, so they are given an address, and the card's
# certificate has to name it. BROKER_HOST names something else to dial: the
# router's name for the Pi, say, which the certificate also carries.
SSID="$ssid" PSK="$psk" BROKER_HOST="${BROKER_HOST:-}" python3 - "$fetched" "$HERE/devices.json" <<'MERGE'
import json, os, sys
with open(sys.argv[1]) as f:
    out = json.load(f)
if os.environ["BROKER_HOST"]:
    out["broker"]["value"]["host"] = os.environ["BROKER_HOST"]
out["device_wifi"] = {"value": {"ssid": os.environ["SSID"], "psk": os.environ["PSK"]}}
with open(sys.argv[2], "w") as f:
    json.dump(out, f, indent=2)
    f.write("\n")
print()
print("Broker: %s:%s" % (out["broker"]["value"]["host"], out["broker"]["value"]["port"]))
print("Accounts: %s" % ", ".join(sorted(v["value"]["username"] for k, v in out.items() if k.endswith("_broker"))))
MERGE
unset psk

cat <<NEXT

Written: $HERE/devices.json and deevnet-kit-ca.pem.

The broker's address must stay what it is: reserve it for the Pi on the
router. If it changes, on the Pi: sudo deevnet-kit regen-certs, then
sudo systemctl restart eds-console, and provision the devices again.

To move a device to this Pi (it leaves the tenant when you do), with ESP-IDF
activated and the device on its cable, in its firmware directory:

  python3 tools/provision_tenant.py -p <port> \\
      --outputs-json $HERE/devices.json --ca $HERE/deevnet-kit-ca.pem

The beacon first: nothing is attached to it.
NEXT
