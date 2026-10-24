# scribe - a short spoken request in, its words out (ADR-0042).
#
# Plain recipes, one shell per line: macOS ships GNU make 3.81.
SHELL := /bin/bash

PYTHON ?= python3
VENV   ?= $(CURDIR)/.venv
PY     := $(VENV)/bin/python
PORT   ?= 8732
WAVS   ?= $(CURDIR)/testdata

.PHONY: help venv test serve scribe clean

help:
	@echo "scribe - available targets"
	@echo
	@echo "  venv     create the virtualenv and install (editable, with dev extras)"
	@echo "  test     run the test suite (no model, no network)"
	@echo "  serve    run the API on PORT=$(PORT)"
	@echo "  scribe   write down every WAV in WAVS=$(WAVS)"
	@echo "  clean    remove the virtualenv and caches"

$(PY):
	$(PYTHON) -m venv "$(VENV)"
	"$(VENV)/bin/pip" install --quiet --upgrade pip
	"$(VENV)/bin/pip" install --quiet -e ".[dev]"

venv: $(PY)

test: $(PY)
	"$(VENV)/bin/pytest" -q

serve: $(PY)
	"$(VENV)/bin/uvicorn" eds_scribe.api:app --host 0.0.0.0 --port $(PORT)

scribe: $(PY)
	"$(VENV)/bin/eds-scribe" "$(WAVS)"/*.wav

clean:
	rm -rf "$(VENV)" .pytest_cache
	find . -name __pycache__ -type d -prune -exec rm -rf {} +
