#!/bin/bash
# build.sh - build an EdS Raspberry Pi image.
#
#   build.sh <image>          e.g. build.sh agent
#
# An EdS image is Deevnet IoTaaS's base image - Raspberry Pi OS with the site's
# automation user - with one of this directory's playbooks applied to it
# offline (ADR-0015). Deevnet IoTaaS's image factory builds the base; nothing of
# EdS's is in the factory, and nothing of the factory's is copied here.
#
# One image has a different base. `backend` is the whole of EdS on one Pi
# (ADR-0033), and it starts from the take-home image Deevnet IoTaaS publishes:
# the file and its checksum, as offered, and nothing from the factory's
# checkout.
#
# It needs Linux, sudo, loop devices, podman, qemu-aarch64-static and ansible,
# which is to say it runs on the Builder. `make -C deploy/relay image` copies
# this directory there and runs it.
#
#   FACTORY   the image factory checkout (default /srv/dvnt/deevnet-image-factory)
#   BASE      "fresh" to rebuild the cached base image
#   PI_BACKEND  the published take-home image, for `backend`
#             (default /srv/deevnet-http/tenant/pi/raspios-bookworm-mobile-pi-backend.img.xz)
#   SOURCE    what to record as the image's source (default: unknown)
set -euo pipefail

IMAGE="${1:?usage: build.sh <image>, e.g. agent}"
HERE="$(cd "$(dirname "$0")" && pwd)"
FACTORY="${FACTORY:-/srv/dvnt/deevnet-image-factory}"
PI_BACKEND="${PI_BACKEND:-/srv/deevnet-http/tenant/pi/raspios-bookworm-mobile-pi-backend.img.xz}"
OUT="$HERE/out"
INPUTS="$HERE/inputs"
NAME="eds-pi-$IMAGE"
IMG="$OUT/$NAME.img"
BASE_IMG="$OUT/base.img"
MNT="/mnt/eds-image-build"
PLAYBOOK="$HERE/$IMAGE/config.yml"

say()  { printf '\n==> %s\n' "$*"; }
fail() { printf 'build.sh: %s\n' "$*" >&2; exit 1; }

[[ -f "$PLAYBOOK" ]] || fail "no image called '$IMAGE' (no $PLAYBOOK)"
if [[ "$IMAGE" == "backend" ]]; then
    [[ -f "$PI_BACKEND" && -f "$PI_BACKEND.sha256" ]] ||
        fail "no published take-home image and checksum at $PI_BACKEND (set PI_BACKEND)"
else
    [[ -d "$FACTORY/packer/pi" ]] || fail "no image factory at $FACTORY (set FACTORY)"
fi
for tool in podman ansible-playbook growpart losetup xz nft file; do
    command -v "$tool" >/dev/null || fail "$tool is not installed"
done
[[ -x /usr/bin/qemu-aarch64-static ]] || fail "qemu-aarch64-static is not installed"
mkdir -p "$OUT"

# --- inputs --------------------------------------------------------------------
# What an image installs that this directory does not hold. For the agent that
# is the agent itself, cross-compiled from services/nowplaying.
if [[ "$IMAGE" == "agent" ]]; then
    [[ -f "$INPUTS/npagent" ]] || fail "no $INPUTS/npagent: stage it first (make -C deploy/relay image does)"
    file -b "$INPUTS/npagent" | grep -q 'ARM aarch64' || fail "$INPUTS/npagent is not an arm64 binary"
fi
# For the backend it is the agent and the three Go services, palette as a
# wheel, and the services' settings that are not secret, as deploy/ has them.
if [[ "$IMAGE" == "backend" ]]; then
    for binary in npagent lightd nowplayd consoled; do
        [[ -f "$INPUTS/$binary" ]] || fail "no $INPUTS/$binary: stage it first (make -C deploy/backend image does)"
        file -b "$INPUTS/$binary" | grep -q 'ARM aarch64' || fail "$INPUTS/$binary is not an arm64 binary"
    done
    compgen -G "$INPUTS/palette/eds_palette-*.whl" >/dev/null || fail "no palette wheel in $INPUTS/palette"
    for settings in lightd nowplayd console; do
        [[ -f "$INPUTS/settings/$settings.env" ]] || fail "no $INPUTS/settings/$settings.env"
    done
fi

# --- the base image ------------------------------------------------------------
# Built by the factory's own definition, under a name of ours, and kept: it
# changes when Raspberry Pi OS or the automation key does, not when EdS does.
if [[ "$IMAGE" == "backend" ]]; then
    : # its base is taken below, already built
elif [[ "${BASE:-}" == "fresh" || ! -f "$BASE_IMG" ]]; then
    say "Building the base image with the factory's definition"
    make -s -C "$FACTORY" "$FACTORY/packer/pi/raspios-bookworm-base.zip" "$FACTORY/build/keys/a_autoprov_rsa.pub"
    sudo podman run --rm --privileged --network=host --security-opt label=disable \
        -v /dev:/dev -v "$FACTORY":/build:rw \
        docker.io/mkaczanowski/packer-builder-arm:latest build \
        -var "ssh_pubkey_local_path=/build/build/keys/a_autoprov_rsa.pub" \
        -var "image_name=eds-pi-base" \
        packer/pi/sdr-bookworm.pkr.hcl
    sudo mv "$FACTORY/eds-pi-base.img" "$BASE_IMG"
    sudo rm -f "$FACTORY/eds-pi-base-manifest.json"
else
    say "Using the cached base image ($(date -r "$BASE_IMG" +%F)); BASE=fresh rebuilds it"
fi

# --- this image ----------------------------------------------------------------
sudo rm -f "$IMG" "$IMG.xz" "$IMG.xz.sha256"
if [[ "$IMAGE" == "backend" ]]; then
    # Deevnet IoTaaS's own, as it is offered: checked against the checksum
    # published beside it, and already 8G with room in it.
    say "Taking the published take-home image ($(date -r "$PI_BACKEND" +%F))"
    ( cd "$(dirname "$PI_BACKEND")" && sha256sum --check --quiet "$(basename "$PI_BACKEND").sha256" ) ||
        fail "$PI_BACKEND does not match its checksum"
    xz -dc -T0 "$PI_BACKEND" | sudo dd of="$IMG" bs=4M conv=sparse status=none
else
    say "Copying the base and growing it to 8G"
    sudo cp --sparse=always "$BASE_IMG" "$IMG"
    sudo truncate -s 8G "$IMG"
fi

LOOP=""
cleanup() {
    sudo umount -R "$MNT" 2>/dev/null || true
    [[ -n "$LOOP" ]] && sudo losetup -d "$LOOP" 2>/dev/null || true
}
trap cleanup EXIT

LOOP="$(sudo losetup --find --partscan --show "$IMG")"
sudo partprobe "$LOOP"; sudo udevadm settle
for _ in {1..20}; do [[ -b "${LOOP}p1" && -b "${LOOP}p2" ]] && break; sleep 0.2; done
[[ -b "${LOOP}p2" ]] || fail "no partitions appeared on $LOOP"
if [[ "$IMAGE" != "backend" ]]; then
    sudo growpart "$LOOP" 2
    sudo e2fsck -f -y "${LOOP}p2" >/dev/null || true
    sudo resize2fs "${LOOP}p2" >/dev/null
fi

say "Applying $IMAGE/config.yml offline"
sudo umount -R "$MNT" 2>/dev/null || true
sudo mkdir -p "$MNT"
sudo mount "${LOOP}p2" "$MNT"
sudo mkdir -p "$MNT/boot"
sudo mount "${LOOP}p1" "$MNT/boot"
sudo mount --bind /dev "$MNT/dev"
sudo mount -t proc proc "$MNT/proc"
sudo mount -t sysfs sys "$MNT/sys"
sudo install -m 0755 /usr/bin/qemu-aarch64-static "$MNT/usr/bin/qemu-aarch64-static"

sudo ansible-playbook -i localhost, -c local "$PLAYBOOK" \
    --extra-vars "chroot_root=$MNT pi_eds_agent_inputs=$INPUTS pi_eds_agent_source=${SOURCE:-unknown}"

# The emulator was put there for the playbook, and the agent image has always
# shipped with it. The take-home image's own build removes it, so this one
# does too.
[[ "$IMAGE" == "backend" ]] && sudo rm -f "$MNT/usr/bin/qemu-aarch64-static"

cleanup
LOOP=""

say "Compressing"
sudo xz -f -k -6 -T0 "$IMG"
sudo chmod 0644 "$IMG.xz"
( cd "$OUT" && sha256sum "$NAME.img.xz" | sudo tee "$NAME.img.xz.sha256" >/dev/null )
sudo rm -f "$IMG"

say "Done: $IMG.xz"
cat "$OUT/$NAME.img.xz.sha256"
