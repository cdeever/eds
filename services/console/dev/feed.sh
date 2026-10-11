#!/bin/bash
# feed.sh - put a sample of the bus on the development broker, so the console
# has something to show with nothing else running.
#
#   dev/feed.sh [itunes|spotify] [cover.jpg]
#
# "itunes" is a track with its cover. "spotify" is a track reported without
# one, over a stand still lit from the last cover (ADR-0016). With no cover
# file the cover topic is left as it is.
set -euo pipefail

PORT="${BROKER_PORT:-21883}"
PREFIX="${PREFIX:-edsdev}"
what="${1:-itunes}"
cover="${2:-}"

pub() { mosquitto_pub -h 127.0.0.1 -p "$PORT" -r -q 1 -t "$PREFIX/$1" "${@:2}"; }

art=""
if [[ -n "$cover" ]]; then
    pub nowplaying/current/art -f "$cover"
    sha="$(shasum -a 256 "$cover" | cut -d' ' -f1)"
    art=",\"art\":{\"sha256\":\"$sha\",\"from\":\"player\"}"
fi

for part in nowplaying/status lightd/status nowplaying/source/itunes/status nowplaying/source/spotify/status \
            lightstand/lp-stand-01/status; do
    pub "$part" -m online
done

house='"track":{"title":"(Ain'"'"'t Nothin'"'"' but A) House Party","artist":"The J. Geils Band","album":"Bloodshot","duration_ms":298000}'
reaper='"track":{"title":"(Don'"'"'t Fear) The Reaper","artist":"Blue Öyster Cult","album":"Agents Of Fortune","duration_ms":308000}'

case "$what" in
itunes)
    pub nowplaying/source/itunes/state -m "{\"v\":1,\"source\":\"itunes\",\"player\":\"itunes\",\"state\":\"playing\",$house}"
    pub nowplaying/source/spotify/state -m '{"v":1,"source":"spotify","player":"spotify","state":"stopped"}'
    pub nowplaying/current -m "{\"v\":1,\"source\":\"itunes\",\"player\":\"itunes\",\"state\":\"playing\",$house$art}"
    ;;
spotify)
    pub nowplaying/source/itunes/state -m "{\"v\":1,\"source\":\"itunes\",\"player\":\"itunes\",\"state\":\"paused\",$house}"
    pub nowplaying/source/spotify/state -m "{\"v\":1,\"source\":\"spotify\",\"player\":\"spotify\",\"state\":\"playing\",$reaper}"
    pub nowplaying/current -m "{\"v\":1,\"source\":\"spotify\",\"player\":\"spotify\",\"state\":\"playing\",$reaper}"
    ;;
*)
    echo "usage: $0 [itunes|spotify] [cover.jpg]" >&2
    exit 2
    ;;
esac

effect="${EFFECT:-breathe}"
pub lightstand/lp-stand-01/state -m "{\"v\":1,\"effect\":\"$effect\",\"colors\":4,\"brightness\":0.80}"
pub lightstand/lp-stand-01/scene -m "{\"v\":1,\"effect\":\"$effect\",\"speed\":0.4,\"brightness\":0.8,\"palette\":[{\"rgb\":[201,0,39],\"weight\":0.65},{\"rgb\":[172,0,48],\"weight\":0.13},{\"rgb\":[36,0,12],\"weight\":0.13},{\"rgb\":[76,0,23],\"weight\":0.09}]}"
echo "fed $PREFIX/ on port $PORT: $what, $effect"
