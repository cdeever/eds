#!/bin/bash
# new-key.sh - make the owner's age key for secrets/ (ADR-0037), once.
#
# The private half goes into the Mac's Keychain and nowhere else on disk; it
# reaches `security` on its standard input, never on a command line. The
# public half is written into secrets/.sops.yaml, which is committed.
set -euo pipefail

SERVICE=eds-sops-age
HERE="$(cd "$(dirname "$0")" && pwd)"
CONF="$HERE/.sops.yaml"

for tool in age-keygen security sops; do
    command -v "$tool" >/dev/null || { echo "$tool is not installed: brew install sops age" >&2; exit 2; }
done

if security find-generic-password -s "$SERVICE" -a "$USER" >/dev/null 2>&1; then
    pub="$(security find-generic-password -s "$SERVICE" -a "$USER" -w | age-keygen -y)"
    echo "The Keychain already has an age key ($SERVICE): $pub"
else
    key="$(age-keygen 2>/dev/null | grep '^AGE-SECRET-KEY-')"
    [[ -n "$key" ]] || { echo "age-keygen made no key." >&2; exit 1; }
    printf 'add-generic-password -s %s -a %s -w %s\n' "$SERVICE" "$USER" "$key" | security -i >/dev/null
    pub="$(printf '%s\n' "$key" | age-keygen -y)"
    unset key
    echo "Made an age key, and put its private half in the Keychain ($SERVICE)."
fi

if grep -q "AGE_PUBLIC_KEY_NOT_MADE_YET" "$CONF"; then
    sed -i '' "s/AGE_PUBLIC_KEY_NOT_MADE_YET/$pub/" "$CONF"
    echo "Wrote the public half into $CONF: commit it."
elif ! grep -q "$pub" "$CONF"; then
    echo "$CONF names another recipient. Add $pub to it by hand, then: (cd secrets && sops updatekeys *.sops.*)" >&2
fi

cat <<NEXT

Public key: $pub

Now copy the private half to paper, and keep it with your other papers:
  security find-generic-password -s $SERVICE -a $USER -w
NEXT
