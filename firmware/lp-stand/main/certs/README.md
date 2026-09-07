# certs

`mqtt_ca.pem` goes here when `LP_MQTT_USE_TLS` is enabled — the CA that signed
the broker's certificate, so the stand can verify `mqtt01` before handing over
credentials.

It is deliberately not committed: it is installation-specific, and a repository
is the wrong place to imply one particular CA is *the* CA. The build embeds
whatever is in this directory.

Per-device client certificates (mutual TLS) are the intended end state — see the
roadmap in the firmware README. This directory is where that material would
land too.
