# Ears: the reSpeaker Lite

The microphone EdS is asked for albums through ([ADR-0043](../../docs/content/docs/architecture/decisions/0043-the-respeaker-runs-firmware-of-edss-own.md)).

The reSpeaker Lite is two chips. An **XMOS XU316** takes its two
microphones and gives one cleaned-up voice over I2S: echo cancelled, noise
suppressed, beam steered. Its firmware is Seeed's I2S build, 1.0.8, and is
left alone. A **XIAO ESP32-S3** (8 MB PSRAM, 8 MB flash, MAC
`68:ee:8f:50:45:b0`) runs this.

While the XIAO's button is held, what is said is kept as 16 kHz mono, up to
fifteen seconds. On release it goes as one WAV to
`eds/voice/respeaker-01/utterance`, where voiced makes it a request for an
album ([ADR-0042](../../docs/content/docs/architecture/decisions/0042-a-request-is-heard-on-the-workload.md)).
What voiced made of it comes back on `…/heard`. A wake word comes next.

```bash
make test                      # host tests for the sound and the events, gcc only
. ~/.espressif/tools/activate_idf_v6.1.sh
make build flash provision monitor   # PORT defaults to /dev/cu.usbmodem2101
```

`flash` and `provision` check the board's MAC first (`MAC=` in the
Makefile). The board came with a Seeed test sketch, saved by `make backup`
in `backup/` before it was first flashed.

## The light

The XIAO's yellow LED:

| | |
|---|---|
| two quick flashes | connected to the broker |
| lit | listening, while the button is held |
| one long flash | voiced asked for an album |
| three short flashes | not understood, not sent, or no broker |
| five fast flashes | the button was pressed with no usable sound to listen to |

## Bringing it up

The pins (BCLK 8, WS 7, DIN 44), the rate and the voice's channel are
Seeed's examples' and Kconfig's defaults, not yet measured. The console
prints, every two seconds, the rate the XMOS chip sends and the level of
each channel:

```
mic: 16000 Hz; left -48.2 dB, right -71.9 dB; the voice is taken from the left
```

- **"no clock on BCLK GPIO8"**: the pins are wrong, or the XMOS chip has its
  USB firmware rather than its I2S one.
- **A rate**: 16 or 48 kHz both work.
- **Speak, and watch which channel rises**: that is `EA_VOICE_CHANNEL`.

The rate and the levels also go to the log store when first known, and in
each five-minute health report.

## What is copied

The credentials (`config/ea_credentials.*`), WiFi, the event log and
`tools/provision_tenant.py` are the cover screen's, renamed: change one,
change the other. The partition table has two OTA slots and the bootloader
has rollback on from the first flash (ADR-0035), so updates through the
broker can come later without a cable.

## Not yet

- The wake word: microWakeWord, with a stock model and then "Hey Ed".
- Updates through the broker (`make ota`) and the `ota` account's grant.
- The reSpeaker's own RGB LED and speaker.
