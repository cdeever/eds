#!/bin/bash
# music.sh SRC DEST - the room's own music, as the training hears it.
#
# Every song under SRC that macOS can read (m4a, mp3, aiff, wav, flac) is
# made 16 kHz mono 16-bit WAV in DEST by afconvert. A song already made is
# skipped; one that cannot be read (a protected .m4p) is said and skipped.
# The songs are the owner's and are never committed (ADR-0054).
set -euo pipefail
src=$1
dest=$2
mkdir -p "$dest"
made=0 skipped=0
while IFS= read -r -d '' song; do
  name=$(basename "${song%.*}" | tr -c 'A-Za-z0-9_.-\n' '_')
  out="$dest/$name.wav"
  [[ -s "$out" ]] && continue
  if afconvert -f WAVE -d LEI16@16000 -c 1 "$song" "$out" 2>/dev/null; then
    made=$((made + 1))
  else
    rm -f "$out"
    echo "could not read: $song" >&2
    skipped=$((skipped + 1))
  fi
done < <(find "$src" -type f \( -iname '*.m4a' -o -iname '*.mp3' -o -iname '*.aif' -o -iname '*.aiff' -o -iname '*.wav' -o -iname '*.flac' \) ! -name '._*' -print0)
find "$dest" -name '._*' -type f -delete
echo "music: $made made, $skipped unreadable, $(find "$dest" -name '*.wav' ! -name '._*' | wc -l | tr -d ' ') in $dest"
