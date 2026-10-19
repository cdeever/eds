#!/bin/bash
# spotify.sh - authorize the agent for Spotify here, and hand it to the Pi.
#
# deploy/relay/spotify.sh, for a Pi that is reached directly and not through
# the Builder (ADR-0033). The two are to be one script once the backend Pi is
# proven. It reads the same settings file, deploy/relay/spotify.conf: the app
# is the same app.
#
# The authorization needs a browser, which this machine has and the Pi does
# not. So it is made here and then MOVED to the Pi: Spotify may replace the
# refresh token on any use, so two holders of one authorization would each
# sooner or later break the other. For the same reason the relay Pi's
# authorization is never copied to this Pi: this makes one of its own.
set -euo pipefail

SSH="${SSH:?}"; NPAGENT="${NPAGENT:?}"
CONF="$(dirname "$0")/../relay/spotify.conf"

if [[ ! -f "$CONF" ]]; then
    echo "No $CONF. Run make -C deploy/relay spotify once to have it written, and fill it in." >&2
    exit 2
fi

# Read for two named values; nothing in the file is run.
value() { grep -m1 -E "^$1=" "$CONF" | cut -d= -f2- | sed 's/[[:space:]]*$//'; }
client_id="$(value SPOTIFY_CLIENT_ID)"
devices="$(value SPOTIFY_DEVICES)"
if [[ ! "$client_id" =~ ^[0-9a-f]{32}$ ]]; then
    echo "SPOTIFY_CLIENT_ID in $CONF is not a client id (32 hex characters)." >&2
    exit 2
fi

# A directory of its own, so this never touches an agent being run on this
# machine, and so what is left behind afterward is nothing.
state="$(mktemp -d)"
trap 'rm -rf "$state"' EXIT
export NP_STATE_DIR="$state"
token="$state/spotify-token.json"

remote() { ssh "$SSH" "$1"; }

if [[ "${AGAIN:-}" == 1 ]] || ! remote "sudo test -s /var/lib/eds-npagent/spotify-token.json"; then
    NP_SPOTIFY_CLIENT_ID="$client_id" "$NPAGENT" spotify-auth
    remote "eds-agent-spotify token" < "$token"
    rm -f "$token"
else
    echo "The Pi already holds an authorization (AGAIN=1 to make a new one)."
fi

if [[ -n "$devices" ]]; then
    printf '%s\n' "$devices" | remote "eds-agent-spotify devices"
else
    # Asked of the Pi, which is now the only holder of the authorization.
    echo
    remote "eds-agent-spotify list" || true
    echo
    echo "No devices are named yet, so nothing counts and the driver will not start." >&2
    echo "Put the ones that count in SPOTIFY_DEVICES in $CONF and run this again." >&2
    exit 2
fi
