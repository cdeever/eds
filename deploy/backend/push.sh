#!/bin/bash
# push.sh - put this tree's services, agent, units and tools on the running
# backend Pi.
#
# For changing a Pi that is already in service without reflashing it. What it
# installs is what the image would have had; rebuild the image afterward
# (`make image`) or the next card flashed will be older than the Pi it
# replaces.
#
# Nothing secret travels. The Pi's accounts, settings, pairing and
# authorization are not touched; eds-backend-setup is run at the end, and adds
# only an account that is new to the list.
#
# Not pushed: the firewall, the kernel settings and the broker's drop-in.
# Those change with an image.
set -euo pipefail

SSH="${SSH:?}"; REPO="${REPO:?}"; STAGE="${STAGE:?}"; VERSION="${VERSION:?}"
FILES="$REPO/images/backend/files"
AGENT_FILES="$REPO/images/agent/files"

stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT

mkdir -p "$stage/settings" "$stage/palette"
cp "$STAGE"/{npagent,lightd,nowplayd,consoled} "$stage/"
cp "$STAGE"/palette/eds_palette-*.whl "$stage/palette/"
cp "$STAGE"/settings/{lightd,nowplayd,console}.env "$stage/settings/"
cp "$FILES/console-pi.settings" "$stage/settings/console-pi.env"
cp "$FILES"/{accounts.conf,eds-backend-setup,eds-backend-status,eds-backend-palette-install} "$stage/"
cp "$FILES"/{eds-backend-setup,eds-palette,eds-lightd,eds-nowplayd,eds-console}.service "$FILES/npagent-backend.conf" "$stage/"
cp "$AGENT_FILES"/{npagent.service,eds-agent-pair,eds-agent-spotify} "$stage/"
cp "$AGENT_FILES"/{eds-recognize,eds-recognize-requirements.txt,eds-agent-turntable-install} "$stage/"
printf '%s\n' "$VERSION" > "$stage/source"

# Runs on the Pi, as root, from the directory the rest was unpacked into.
cat > "$stage/install.sh" <<'INSTALL'
#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
install -o root -g root -m 0755 npagent lightd nowplayd consoled /usr/local/bin/
install -o root -g root -m 0755 eds-backend-status eds-agent-pair eds-agent-spotify /usr/local/bin/
install -o root -g root -m 0755 eds-backend-setup eds-backend-palette-install eds-agent-turntable-install /usr/local/sbin/
install -o root -g root -m 0644 settings/*.env /etc/eds/settings/
install -o root -g root -m 0644 accounts.conf /etc/eds/accounts.conf
install -o root -g root -m 0644 ./*.service /etc/systemd/system/
install -o root -g root -m 0644 npagent-backend.conf /etc/systemd/system/npagent.service.d/eds-backend.conf
# Both come from PyPI, which the Pi reaches by itself.
/usr/local/sbin/eds-backend-palette-install "$PWD/palette"
/usr/local/sbin/eds-agent-turntable-install "$PWD"
printf 'pushed: %s on %s\n' "$(cat source)" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> /etc/eds-backend-release
systemctl daemon-reload
/usr/local/sbin/eds-backend-setup
systemctl restart eds-palette.service eds-lightd.service eds-nowplayd.service eds-console.service
# 78 is "no driver is set up", which is a fair state for a Pi to be in.
systemctl restart npagent.service || true
echo "installed $(cat source)"
for unit in eds-palette eds-lightd eds-nowplayd eds-console npagent; do
    echo "  $unit is $(systemctl is-active "$unit" || true)"
done
INSTALL

# macOS tar adds a ._ file beside each one unless told not to.
COPYFILE_DISABLE=1 tar -C "$stage" -cf - . |
    ssh "$SSH" 'd=$(mktemp -d) && tar -xf - -C $d && sudo bash $d/install.sh; rc=$?; rm -rf $d; exit $rc'
