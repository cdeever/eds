# scribe

A short spoken request in, its words out ([ADR-0042](../../docs/content/docs/architecture/decisions/0042-a-request-is-heard-on-the-workload.md)).

scribe knows nothing about music, MQTT or albums. `voiced` hears a request on
the bus and asks scribe what was said; scribe answers with words. That
boundary is what lets it be tuned against a folder of WAVs with no
infrastructure at all, as palette is against a folder of covers.

```bash
make venv PYTHON=python3.12   # faster-whisper has no wheels for 3.15 yet
make test                     # no model, no network
make serve                    # on :8733; the model loads on the first request
make scribe WAVS=~/requests   # write down every WAV in a folder
```

## The contract

`POST /v1/transcribe`, the body a WAV:

- **16 kHz, 16-bit, mono PCM, 0.3 to 15 seconds.** Anything else is refused
  with 422, not converted: there is one sender, and a different recording
  means something is wrong with it. A body over a megabyte is refused with
  413.
- The answer is `{"v":1,"text":"Play Van Halen's first album.","seconds":2.5,"took_ms":900}`.
  `text` is empty when nothing was said.

`GET /healthz` says it is running.

## The model

faster-whisper, `base.en`, int8, on the CPU (`SCRIBE_MODEL`,
`SCRIBE_THREADS`, default 2). The workload has two cores and two gigabytes;
`base.en` uses a few hundred megabytes of it. The image fetches the model
when it is built and runs offline. Requests are written down one at a time,
because two at once on two cores are each slower than both in turn.

The model is prompted with a sentence like the requests it will hear, which
makes it likelier to write names as names.
