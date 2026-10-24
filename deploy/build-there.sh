#!/bin/bash
# build-there.sh HOST NAME DIR SOURCE [CONTAINERFILE]
#
# Build an image on the workload itself, from its sources sent as they are,
# on a base the workload already has. It is how scribe's code reaches the
# workload: 60 KB of sources, where the image they make is 500 MB, nearly all
# of it the base (INC-0002). The build there fetches nothing: --pull=never.
#
# Like load.sh, it does nothing when the workload's image already carries the
# label eds.source with SOURCE, the hash of what it was built from.
set -euo pipefail

HOST="${1:?usage: build-there.sh HOST NAME DIR SOURCE [CONTAINERFILE]}"
NAME="${2:?}"; DIR="${3:?}"; SOURCE="${4:?}"; FILE="${5:-Containerfile}"
image="localhost/eds-$NAME:latest"
SSH=(ssh -o BatchMode=yes "$HOST")

have=$("${SSH[@]}" "sudo podman image inspect --format '{{index .Labels \"eds.source\"}}' $image 2>/dev/null" || true)
if [[ "$have" == "$SOURCE" ]]; then
  echo "$NAME: unchanged ($SOURCE), not built there"
  exit 0
fi

echo "$NAME: sources ${have:-unknown} -> $SOURCE; building there, $(tar -C "$DIR" --exclude .venv --exclude __pycache__ --exclude .pytest_cache -czf - . | wc -c | numfmt --to=iec) sent"
tar -C "$DIR" --exclude .venv --exclude __pycache__ --exclude .pytest_cache -czf - . |
  "${SSH[@]}" "d=\$(mktemp -d); tar -xzf - -C \$d \
    && sudo podman build -q --pull=never --label eds.deploy=1 --label eds.source=$SOURCE -t $image -f \$d/$FILE \$d >/dev/null; \
    rc=\$?; rm -rf \$d; exit \$rc"
# What it replaced, untagged now: the workload has room for a few of these,
# not one every deploy.
"${SSH[@]}" "sudo podman image prune -f --filter label=eds.deploy=1 >/dev/null"
echo "$NAME: built there"
