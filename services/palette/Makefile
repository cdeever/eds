# palette - album cover art in, ranked colour swatches out.
.ONESHELL:
SHELL := /usr/bin/bash
.SHELLFLAGS := -euo pipefail -c

VENV   ?= $(CURDIR)/.venv
PY     := $(VENV)/bin/python
PORT   ?= 8731
COVERS ?= $(CURDIR)/testdata

.PHONY: help venv test serve palette sheet clean

help:
	@echo "palette - available targets"
	@echo
	@echo "  venv     create the virtualenv and install (editable, with dev extras)"
	@echo "  test     run the test suite"
	@echo "  serve    run the API on PORT=$(PORT)"
	@echo "  palette  report palettes for COVERS=$(COVERS)"
	@echo "  sheet    render a PNG contact sheet for COVERS"
	@echo "  clean    remove the virtualenv and caches"

$(PY):
	python3 -m venv "$(VENV)"
	"$(VENV)/bin/pip" install --upgrade pip
	"$(VENV)/bin/pip" install -e ".[dev]"

venv: $(PY)

test: $(PY)
	"$(VENV)/bin/pytest" -q

serve: $(PY)
	"$(VENV)/bin/uvicorn" eds_palette.api:app --host 0.0.0.0 --port $(PORT)

palette: $(PY)
	"$(VENV)/bin/eds-palette" "$(COVERS)"

sheet: $(PY)
	"$(VENV)/bin/eds-palette" "$(COVERS)" --contact-sheet "$(CURDIR)/sheet.png"

clean:
	rm -rf "$(VENV)" .pytest_cache sheet.png
	find . -name __pycache__ -type d -prune -exec rm -rf {} +
