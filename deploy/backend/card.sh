#!/bin/bash
# card.sh - write a flashed backend card's settings to its boot partition.
#
# The image carries no secret and no name. This writes, on the card's boot
# partition:
#
#   deevnet-kit.txt   the tenant and the card's name, which deevnet-kit
#                     reads once, on the first boot. No Wi-Fi: the Pi is on
#                     a cable, and the image starts with its radio off
#                     (CHG-0046)
#   userconf.txt      your login, which Raspberry Pi OS makes on the first boot
#   ssh               turns SSH on
#   eds/              your public key, your Discogs settings if you have
#                     them, and the iTunes pairing kept for this card's name
#                     in secrets/ (ADR-0052); taken and removed by
#                     eds-backend-setup
#
# Nothing is kept on this machine. The card's broker accounts, its CA and its
# tokens are not written here at all: they are made on the card.
set -euo pipefail

BOOT="${BOOT:-/Volumes/bootfs}"
PI_HOSTNAME="${PI_HOSTNAME:?set PI_HOSTNAME to what the card should call itself}"
PI_USER="${PI_USER:?set PI_USER to your login on the card}"
TENANT="${TENANT:?}"; INDEX="${INDEX:?}"
DISCOGS="${DISCOGS:-}"
NAMES="${NAMES:-}"

if [[ ! -f "$BOOT/eds-backend.txt" || ! -f "$BOOT/deevnet-kit.txt" ]]; then
    echo "No backend card at $BOOT (no eds-backend.txt and deevnet-kit.txt there)." >&2
    echo "Flash the image, leave the card in, and check where its boot partition mounted:" >&2
    echo "  make card BOOT=/Volumes/<name>" >&2
    exit 2
fi
if [[ ! "$PI_HOSTNAME" =~ ^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$ ]]; then
    echo "PI_HOSTNAME must be lowercase letters, digits and dashes; got '$PI_HOSTNAME'." >&2
    exit 2
fi
if [[ ! "$PI_USER" =~ ^[a-z_][a-z0-9_-]{0,31}$ ]]; then
    echo "PI_USER must be a login name; got '$PI_USER'." >&2
    exit 2
fi

# The login's password is stored as a SHA-512 crypt hash, which macOS's own
# openssl cannot make. Homebrew's can.
hasher=""
for candidate in openssl /opt/homebrew/opt/openssl/bin/openssl /usr/local/opt/openssl/bin/openssl; do
    if command -v "$candidate" >/dev/null 2>&1 && "$candidate" passwd -6 -stdin <<<probe >/dev/null 2>&1; then
        hasher="$candidate"
        break
    fi
done
if [[ -z "$hasher" ]]; then
    echo "No openssl here can make the password's hash (passwd -6). brew install openssl" >&2
    exit 2
fi

# A cable, always: the image starts with its radio off, so a network named
# here would never be joined.
ssid=""; psk=""; country=""

read -r -s -p "A password for $PI_USER on the card: " password; echo
read -r -s -p "Again: " again; echo
if [[ -z "$password" || "$password" != "$again" ]]; then
    echo "The two did not match, or were empty." >&2
    exit 2
fi
hash="$(printf '%s\n' "$password" | "$hasher" passwd -6 -stdin)"
unset password again

# printf, not a heredoc with expansion: the key is data, whatever is in it.
{
    printf '# Written by EdS, deploy/backend (make card), on %s.\n' "$(date -u +%Y-%m-%d)"
    printf '# README.txt, beside this file, says what each line is. Read once, on first boot.\n'
    printf 'tenant=%s\nindex=%s\nhostname=%s\n' "$TENANT" "$INDEX" "$PI_HOSTNAME"
    printf 'wifi_ssid=%s\nwifi_psk=%s\nwifi_country=%s\n' "$ssid" "$psk" "$country"
    # Only when given: a kit older than 0.12.0 stops on a key it does not know.
    if [[ -n "$NAMES" ]]; then
        printf 'names=%s\n' "$NAMES"
    fi
} > "$BOOT/deevnet-kit.txt"
unset psk

printf '%s:%s\n' "$PI_USER" "$hash" > "$BOOT/userconf.txt"
: > "$BOOT/ssh"

drop="$BOOT/eds"
mkdir -p "$drop"

key="${SSH_KEY:-}"
if [[ -z "$key" ]]; then
    for candidate in "$HOME/.ssh/id_ed25519.pub" "$HOME/.ssh/id_ecdsa.pub" "$HOME/.ssh/id_rsa.pub"; do
        [[ -f "$candidate" ]] && { key="$candidate"; break; }
    done
fi
if [[ -n "$key" && -f "$key" ]]; then
    printf '%s\n' "$PI_USER" > "$drop/login"
    cp "$key" "$drop/authorized_keys"
    keyed="your key $(basename "$key")"
else
    keyed="no key (none found; SSH_KEY=<file.pub> names one), so the password each time"
fi

if [[ -n "$DISCOGS" && -f "$DISCOGS" ]]; then
    grep -E '^NP_DISCOGS_(USER|TOKEN)=' "$DISCOGS" > "$drop/discogs.env"
    discogs="your Discogs settings"
else
    discogs="no Discogs settings (none at ${DISCOGS:-DISCOGS}): records are named by what was heard"
fi

# The pairing kept for this card's name, decrypted straight onto the card.
itunes="$(PI_HOSTNAME="$PI_HOSTNAME" "$(dirname "$0")/../itunes-pairing.sh" card "$drop")"

# macOS leaves its own files beside anything it writes to FAT.
rm -f "$BOOT"/._* "$drop"/._* 2>/dev/null || true
rmdir "$drop" 2>/dev/null || true
sync

echo
echo "Written to $BOOT, for $PI_HOSTNAME (tenant $TENANT, index $INDEX):"
echo "  network   a cable (the radio is off)"
echo "  login     $PI_USER, with $keyed"
echo "  also      $discogs"
echo "  iTunes    $itunes"
echo
echo "Eject the card, put it in the Pi, and power it on. The first boot makes"
echo "your login, restarts once, and then makes the card's CA and its accounts:"
echo "allow a few minutes. Then: make status"
