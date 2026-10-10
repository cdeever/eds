#!/bin/bash
# card.sh - write a flashed relay card's settings to its boot partition.
#
# The image carries no secret and no name. This writes four files into
# eds-agent/ on the card's boot partition; the Pi imports them at its next
# boot and deletes them from the card.
#
#   hostname             what the card calls itself
#   wifi.conf            the home network and its key, asked for here: the
#                        second leg, which is what makes this card a relay
#   npagent.env          the agent's broker account, from the tenant's state
#   deevnet-root-ca.pem  the CA the broker's certificate is verified against
#
# Nothing is kept on this machine: the key is never written anywhere but the
# card, and the broker account stays in the tenant's state where it lives.
set -euo pipefail

INFRA="${INFRA:?set INFRA to the tenant directory}"
BOOT="${BOOT:-/Volumes/bootfs}"
SOURCE_ID="${SOURCE_ID:-itunes}"
PI_HOSTNAME="${PI_HOSTNAME:?set PI_HOSTNAME to what the card should call itself}"

if [[ ! -f "$BOOT/eds-agent.txt" ]]; then
    echo "No agent card at $BOOT (no eds-agent.txt there)." >&2
    echo "Flash the image, leave the card in, and check where its boot partition mounted:" >&2
    echo "  make card BOOT=/Volumes/<name>" >&2
    exit 2
fi
if [[ ! -f "$INFRA/deevnet-root-ca.pem" ]]; then
    echo "No $INFRA/deevnet-root-ca.pem." >&2
    exit 2
fi

read -r -p "Home Wi-Fi network [DVNT-CORE]: " ssid
ssid="${ssid:-DVNT-CORE}"
read -r -s -p "Key for $ssid: " psk; echo
if (( ${#psk} < 8 || ${#psk} > 63 )); then
    echo "A WPA key is 8 to 63 characters; that was ${#psk}." >&2
    exit 2
fi
read -r -p "Wi-Fi country code [US]: " country
country="${country:-US}"

drop="$BOOT/eds-agent"
mkdir -p "$drop"

printf '%s\n' "$PI_HOSTNAME" > "$drop/hostname"

# printf, not a heredoc with expansion: the key is data, whatever is in it.
printf 'SSID=%s\nPSK=%s\nCOUNTRY=%s\n' "$ssid" "$psk" "$country" > "$drop/wifi.conf"
unset psk

cp "$INFRA/deevnet-root-ca.pem" "$drop/deevnet-root-ca.pem"

# The agent's account, straight from the tenant's state to the card.
( set -a; . "$INFRA/.backend.env"; set +a
  terraform -chdir="$INFRA" output -json ) | SOURCE_ID="$SOURCE_ID" python3 -c '
import json, os, sys
out = {k: v["value"] for k, v in json.load(sys.stdin).items()}
for need in ("broker", "np_relay_broker"):
    if need not in out:
        sys.exit("the tenant has no output %r: apply infra/deevnet-tenant-eds first" % need)
broker, account, source = out["broker"], out["np_relay_broker"], os.environ["SOURCE_ID"]

# The broker drops a publish to a topic that was not granted, and says
# nothing. Better to refuse here than to boot an agent that reports to no one.
wanted = ["eds/nowplaying/source/%s/%s" % (source, kind) for kind in ("state", "status", "art")]
missing = [t for t in wanted if t not in account["publish"]]
if missing:
    sys.exit("the relay account was not granted: %s\ngranted: %s" % (missing, account["publish"]))

print("NP_MQTT_URL=tls://%s:%d" % (broker["host"], int(broker["port"])))
print("NP_MQTT_USERNAME=%s" % account["username"])
print("NP_MQTT_PASSWORD=%s" % account["password"])
print("NP_MQTT_CA_FILE=/etc/eds/deevnet-root-ca.pem")
print("NP_MQTT_CLIENT_ID=%s" % account["username"])
print("NP_TOPIC_PREFIX=eds")
print("NP_SOURCE_ID=%s" % source)
print("NP_PLAYER=itunes")
' > "$drop/npagent.env"

# macOS leaves its own files beside anything it writes to FAT.
rm -f "$drop"/._* 2>/dev/null || true
sync

echo
echo "Written to $drop, for $PI_HOSTNAME:"
ls -1 "$drop" | sed 's/^/  /'
echo
echo "Eject the card, put it in the Pi, connect its Ethernet, and power it on."
echo "Then: make status"
