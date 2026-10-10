---
title: "Services: palette and lightd"
weight: 2
---

# Services

## palette

```bash
cd services/palette
make venv                      # .venv with editable install + dev extras
make test                      # pytest, no network
make serve PORT=8731
```

Run a single test:

```bash
.venv/bin/pytest tests/test_extract.py::test_name -q
```

### Tuning by eye

The thresholds are judgement calls that no test can settle — where a colour
stops being a colour, how much black field to discard. The CLI exists for that
loop:

```bash
make palette COVERS=~/covers
make sheet   COVERS=~/covers      # PNG contact sheet
.venv/bin/eds-palette ~/covers --min-chroma 0.05 --min-lightness 0.08
```

It prints truecolor swatches in the terminal and can render a sheet of
thumbnails beside their weighted palettes, so many sleeves can be compared at
once.

### Test fixtures

Real cover art cannot be committed, so `tests/conftest.py` builds synthetic
sleeves that exhibit the specific pathologies the algorithm exists to handle:
two-tone, heavy black field, blown-out white field, and genuinely monochrome
art. Add fixtures there rather than committing images.

## lightd

```bash
cd services/lightd
make check                     # gofmt check + go vet + go test ./...
make broker                    # dev mosquitto in podman on tcp://127.0.0.1:21883
make test-integration          # broker + go test -race ./...
make run                       # needs palette on :8731
make broker-watch              # tail every eds/# topic
```

Run a single test:

```bash
go test ./internal/scene -run TestName -v
```

Integration tests are gated on `LIGHTD_TEST_BROKER`; without it they skip. The
retained-scene behaviour is a *broker* behaviour rather than a client one, so
the tests that matter most need a real broker.

### Exercising the whole vertical

```bash
# Cover image in, scene published.
curl -F image=@jacket.jpg http://localhost:8732/v1/cover

# Publish a scene by hand - how the stand gets exercised before any image path
# exists, and how a specific effect gets tested without hunting for cover art.
curl -X POST http://localhost:8732/v1/scene \
  -d '{"effect":"solid","brightness":1,"palette":[{"rgb":[255,0,0],"weight":1}]}'

curl 'http://localhost:8732/v1/scene?stand=lp-stand-01'   # what was last sent
```

`POST /v1/cover` was built as the interface an album cover resolver would
call. It arrived as a topic instead: with `LIGHTD_COVER_TOPIC` set, lightd
subscribes and runs each image it receives down the same path
([ADR-0009](/docs/architecture/decisions/0009-lightd-takes-covers-from-a-topic/)).
It still sees only image bytes.
