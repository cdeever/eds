#!/bin/bash
# load.sh HOST NAME... - send the workload each image it does not already have.
#
# Run where the images were built: the Builder, or a machine with podman.
#
# An image is sent only when its sources changed: each carries the label
# eds.source, a hash of what it was built from (the Makefile's images
# target), and one the workload already has under the same label stays where
# it is. A build is not reproducible - the same sources make a new image id
# every time - so the label is compared, not the id.
#
# What is sent goes as a file, at a capped rate. Every deploy used to stream
# every image, scribe's 767 MB and palette's 256 MB among them, through the
# core router between the Builder's segment and the workload's, as fast as
# it would go; on 2026-10-24 the router hung under it twice (CHG-0037).
set -euo pipefail

HOST="${1:?usage: load.sh HOST NAME...}"
shift
# Kbit/s, as scp takes it: 40000 is 5 MB/s, scribe in under three minutes.
LIMIT_KBIT="${LIMIT_KBIT:-40000}"
CONTAINER="${CONTAINER:-podman}"
SSH=(ssh -o BatchMode=yes "$HOST")

for name in "$@"; do
  image="localhost/eds-$name:latest"
  want=$("$CONTAINER" image inspect --format '{{index .Labels "eds.source"}}' "$image")
  have=$("${SSH[@]}" "sudo podman image inspect --format '{{index .Labels \"eds.source\"}}' $image 2>/dev/null" || true)
  if [[ -n "$want" && "$want" == "$have" ]]; then
    echo "$name: unchanged ($want), not sent"
    continue
  fi

  archive="$(mktemp -d)/eds-$name.tar"
  "$CONTAINER" save -q -o "$archive" "$image"
  size=$(du -h "$archive" | cut -f1)
  echo "$name: sources ${have:-unknown} -> $want; sending $size at $((LIMIT_KBIT / 8000)) MB/s"
  scp -q -o BatchMode=yes -l "$LIMIT_KBIT" "$archive" "$HOST:/var/tmp/eds-$name.tar"
  "${SSH[@]}" "sudo podman load -q -i /var/tmp/eds-$name.tar; rm -f /var/tmp/eds-$name.tar"
  rm -rf "$(dirname "$archive")"
done
