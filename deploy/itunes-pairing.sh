#!/bin/bash
# itunes-pairing.sh - keep a Pi's iTunes pairing in secrets/, encrypted, and
# put it back (ADR-0052).
#
#   itunes-pairing.sh keep          the Pi's pairing, into secrets/
#   itunes-pairing.sh restore       secrets/ to the running Pi, and restart its agent
#   itunes-pairing.sh card DIR      secrets/ onto a card's boot partition, in DIR
#
# The pairing is one small JSON file whose GUID is the whole credential: iTunes
# remembers the GUID, not the card, so a card that starts with it is paired.
# It is only ever plain in a pipe, or on the Pi, or on a card until its first
# boot takes it.
#
# Which Pi: PI_HOSTNAME names the file, secrets/itunes-<PI_HOSTNAME>.sops.json.
# How to reach it: SSH=user@host directly (the backend Pi), or BUILDER= and
# PI= for a Pi reached through the Builder as a_autoprov (the relay).
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
PI_HOSTNAME="${PI_HOSTNAME:?set PI_HOSTNAME to the name of the card}"
FILE="$REPO/secrets/itunes-$PI_HOSTNAME.sops.json"
ON_PI=/var/lib/eds-npagent/itunes-pairing.json

# The age key's private half is in the Keychain (secrets/README.md).
export SOPS_AGE_KEY_CMD="${SOPS_AGE_KEY_CMD:-security find-generic-password -s eds-sops-age -a $USER -w}"

on_pi() {
    local quoted
    if [[ -z "${BUILDER:-}" && -z "${SSH:-}" ]]; then
        echo "set SSH=user@host, or BUILDER= and PI=" >&2
        return 2
    fi
    if [[ -n "${BUILDER:-}" ]]; then
        # Two shells: the Builder's, then the Pi's.
        quoted="$(printf '%q' "$1")"
        ssh "$BUILDER" "ssh a_autoprov@$PI $quoted"
    else
        ssh "$SSH" "$1"
    fi
}

ready() {
    command -v sops >/dev/null || { echo "sops is not installed: brew install sops age" >&2; return 1; }
    if grep -q AGE_PUBLIC_KEY_NOT_MADE_YET "$REPO/secrets/.sops.yaml"; then
        echo "No age key yet: run secrets/new-key.sh once (secrets/README.md)." >&2
        return 1
    fi
}

# valid reads a pairing on standard input, and passes it on only if it is one,
# laid out as the agent writes it. Nothing of it is printed.
valid() {
    python3 -c '
import json, re, sys
p = json.load(sys.stdin)
if not re.fullmatch(r"[0-9A-F]{16}", str(p.get("guid", ""))) or not p.get("host"):
    sys.exit("not a pairing: it needs a guid of sixteen hex digits and a host")
json.dump(p, sys.stdout, indent=2)
print()
'
}

case "${1:-}" in
keep)
    ready
    tmp="$(mktemp "$FILE.XXXXXX")"
    trap 'rm -f "$tmp"' EXIT
    on_pi "sudo cat $ON_PI" | valid |
        sops --config "$REPO/secrets/.sops.yaml" --encrypt --input-type json --output-type json \
            --filename-override "$FILE" /dev/stdin > "$tmp"
    if [[ -f "$FILE" ]]; then
        echo "Replacing the pairing kept for $PI_HOSTNAME. iTunes still remembers the old one until it is forgotten there."
    fi
    mv "$tmp" "$FILE"
    trap - EXIT
    echo "Kept: ${FILE#"$REPO"/} (encrypted). Commit it."
    ;;
restore)
    ready
    [[ -f "$FILE" ]] || { echo "No pairing kept for $PI_HOSTNAME (${FILE#"$REPO"/}): make pair." >&2; exit 2; }
    sops --decrypt "$FILE" | valid |
        on_pi "sudo tee $ON_PI.new >/dev/null && sudo chown npagent:npagent $ON_PI.new && sudo chmod 600 $ON_PI.new && sudo mv $ON_PI.new $ON_PI && sudo systemctl restart npagent && systemctl is-active npagent"
    echo "Restored the pairing kept for $PI_HOSTNAME."
    ;;
card)
    drop="${2:?card DIR: the drop directory on the card}"
    if [[ ! -f "$FILE" ]]; then
        echo "no iTunes pairing (none kept for $PI_HOSTNAME): make pair after it boots"
        exit 0
    fi
    # Optional on a card: a pairing that cannot be read is said, and the
    # card is written without it.
    if ! ready 2>/dev/null; then
        echo "no iTunes pairing (sops or the age key is missing here): make pairing after it boots"
        exit 0
    fi
    umask 077
    if sops --decrypt "$FILE" | valid > "$drop/itunes-pairing.json"; then
        echo "its iTunes pairing"
    else
        rm -f "$drop/itunes-pairing.json"
        echo "no iTunes pairing (${FILE#"$REPO"/} would not decrypt): make pairing after it boots"
    fi
    ;;
*)
    echo "usage: $0 keep | restore | card DIR" >&2
    exit 2
    ;;
esac
