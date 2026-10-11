#!/bin/bash
# push.sh - put this tree's agent, its unit and its tools on the running Pi.
#
# For changing a Pi that is already in service without reflashing it. What it
# installs is what the image would have had; rebuild the image afterwards
# (`make image`) or the next card flashed will be older than the Pi it
# replaces.
#
# Nothing secret travels: the agent, a unit file and four scripts. The Pi's
# settings, pairing and authorization are not touched.
set -euo pipefail

BUILDER="${BUILDER:?}"; PI="${PI:?}"; REPO="${REPO:?}"
FILES="$REPO/images/agent/files"
AGENT="$REPO/services/nowplaying/bin/npagent-linux-arm64"

stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT

cp "$AGENT" "$stage/npagent"
cp "$FILES"/{npagent.service,eds-agent-status,eds-agent-pair,eds-agent-spotify,eds-agent-import} "$stage/"
git -C "$REPO" describe --always --dirty > "$stage/source"

# Runs on the Pi, as root, from the directory the rest was unpacked into.
cat > "$stage/install.sh" <<'INSTALL'
#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
install -o root -g root -m 0755 npagent /usr/local/bin/npagent
install -o root -g root -m 0755 eds-agent-status eds-agent-pair eds-agent-spotify /usr/local/bin/
install -o root -g root -m 0755 eds-agent-import /usr/local/sbin/eds-agent-import
install -o root -g root -m 0644 npagent.service /etc/systemd/system/npagent.service
printf 'pushed: %s on %s\n' "$(cat source)" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> /etc/eds-agent-release
systemctl daemon-reload
# 78 is "no driver is set up", which is a fair state for a Pi to be in.
systemctl restart npagent.service || true
echo "installed $(cat source); npagent is $(systemctl is-active npagent.service || true)"
INSTALL

# macOS tar adds a ._ file beside each one unless told not to.
COPYFILE_DISABLE=1 tar -C "$stage" -cf - . |
    ssh "$BUILDER" "ssh a_autoprov@$PI 'd=\$(mktemp -d) && tar -xf - -C \$d && sudo bash \$d/install.sh; rc=\$?; rm -rf \$d; exit \$rc'"
