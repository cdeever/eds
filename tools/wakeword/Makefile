# eds-wakeword - trains the ears' "Hey DJ Ed" with microWakeWord (ADR-0054).
#
# Plain recipes, one shell per line: macOS ships GNU make 3.81.
# WORK holds everything downloaded, generated and trained; it is never in the
# repository and may be on an external disk. Its path may have spaces in it.
SHELL := /bin/bash

WORK    ?=
PYTHON  ?= /opt/homebrew/bin/python3.12
VENV    ?= .venv
PY      := $(VENV)/bin/python

# Pinned: microWakeWord's training as the notebook used it on this date
# (2026-11-11), and the generator's macOS branch the notebook names.
MWW_REPO     := https://github.com/kahrendt/microWakeWord
MWW_COMMIT   := 4665173cd35f1cff9a61e06fc427f124766c488e
PIPER_REPO   := https://github.com/kahrendt/piper-sample-generator
PIPER_COMMIT := b5b36e266576693ea47db129fbfe6bbc06164762
PIPER_MODEL  := https://github.com/rhasspy/piper-sample-generator/releases/download/v2.0.0/en_US-libritts_r-medium.pt
# The voice is a pickled model, which Torch loads only when told to; this is
# the file fetched on 2026-11-11 (204,089,915 bytes), and no other is loaded.
PIPER_SHA256 := e95ee53770bf598c354a6e6dbfc95ccb259aeeb501d35a86be8a767429ab0ff6
METADATA_FORK := git+https://github.com/whatsnowplaying/audio-metadata@d4ebb238e6a401bb1a5aaaac60c9e2b3cb30929f

SAMPLES      ?= 2000
NEAR_SAMPLES ?= 300
MUSIC        ?=
MAX_FAPH     ?= 0.05

MODELS := ../../firmware/ears/main/models

.PHONY: help venv work listen generate fetch music features config train manifest install tidy clean-venv

help:
	@echo "eds-wakeword - trains \"Hey DJ Ed\" for the ears (ADR-0054)"
	@echo
	@echo "  venv        Python $(PYTHON), microWakeWord, the generator ($(VENV) and .src, on this disk)"
	@echo "  work        WORK=... the generator and its voice, in WORK"
	@echo "  listen      one clip of each spelling in phrases.txt, to hear before the rest"
	@echo "  generate    SAMPLES=$(SAMPLES) of each spelling, NEAR_SAMPLES=$(NEAR_SAMPLES) of each near miss"
	@echo "  fetch       room sounds, noise, music and the ready-made negatives (~7 GB down, ~26 GB unpacked)"
	@echo "  music       MUSIC=dir: the room's own music, made 16 kHz WAV in WORK (optional)"
	@echo "  features    augments the clips and makes their features"
	@echo "  config      writes training_parameters.yaml"
	@echo "  train       trains, quantizes and tests the streaming model (hours)"
	@echo "  manifest    picks the cutoff at MAX_FAPH=$(MAX_FAPH) false wakes an hour"
	@echo "  install     copies the model and manifest to firmware/ears/main/models"
	@echo
	@echo "Every target after venv needs WORK=/path/outside/the/repository."

need-work:
	@if [[ -z "$(WORK)" ]]; then echo "WORK=/path is needed: somewhere outside the repository, with ~35 GB" >&2; exit 1; fi
	@mkdir -p "$(WORK)"

venv:
	$(PYTHON) -m venv $(VENV)
	$(PY) -m pip install -q --upgrade pip
	@# pymicro-features 2 builds from source on macOS; the notebook's macOS fork
	@# of it is older than this microWakeWord and lacks process_samples.
	$(PY) -m pip install -q 'pymicro-features==2.0.2' '$(METADATA_FORK)'
	@# Editable from a clone, as the notebook installs it: microwakeword/audio
	@# and microwakeword/layers have no __init__.py, and a plain install drops them.
	@if [[ ! -d .src/microWakeWord ]]; then git clone -q $(MWW_REPO) .src/microWakeWord; fi
	git -C .src/microWakeWord checkout -q $(MWW_COMMIT)
	$(PY) -m pip install -q -e .src/microWakeWord
	$(PY) -m pip install -q torch torchaudio piper-phonemize-cross==1.2.1 webrtcvad-wheels scipy tqdm soundfile pyarrow huggingface_hub tensorboard 'datasets<4'
	@$(PY) -c 'import microwakeword.audio.augmentation, microwakeword.layers.modes, torch, tensorflow as tf; print("microWakeWord, torch", torch.__version__, "tensorflow", tf.__version__)'

work: need-work
	@if [[ ! -d "$(WORK)/piper-sample-generator" ]]; then git clone -q $(PIPER_REPO) "$(WORK)/piper-sample-generator"; fi
	git -C "$(WORK)/piper-sample-generator" checkout -q $(PIPER_COMMIT)
	@if [[ ! -s "$(WORK)/piper-sample-generator/models/en_US-libritts_r-medium.pt" ]]; then curl -fL --progress-bar -o "$(WORK)/piper-sample-generator/models/en_US-libritts_r-medium.pt" $(PIPER_MODEL); fi
	@echo "$(PIPER_SHA256)  $(WORK)/piper-sample-generator/models/en_US-libritts_r-medium.pt" | shasum -a 256 -c --status || { echo "the voice model is not the one pinned: delete it and run make work again" >&2; exit 1; }

listen: work
	$(PY) scripts/generate.py --work "$(WORK)" --listen phrases.txt
	@echo "Listen: open \"$(WORK)/listen\""

generate: work
	$(PY) scripts/generate.py --work "$(WORK)" --samples $(SAMPLES) phrases.txt positive
	$(PY) scripts/generate.py --work "$(WORK)" --samples $(NEAR_SAMPLES) near-misses.txt negative

fetch: need-work
	$(PY) scripts/fetch.py --work "$(WORK)"

music: need-work
	@if [[ -z "$(MUSIC)" ]]; then echo "MUSIC=/dir/of/songs is needed" >&2; exit 1; fi
	scripts/music.sh "$(MUSIC)" "$(WORK)/music_16k"

features: need-work
	$(PY) scripts/features.py --work "$(WORK)"

config: need-work
	$(PY) scripts/config.py --work "$(WORK)"

train: need-work
	cd "$(WORK)" && "$(CURDIR)/$(PY)" -m microwakeword.model_train_eval \
	  --training_config=training_parameters.yaml \
	  --train 1 --restore_checkpoint 1 \
	  --test_tf_nonstreaming 0 --test_tflite_nonstreaming 0 \
	  --test_tflite_nonstreaming_quantized 0 --test_tflite_streaming 0 \
	  --test_tflite_streaming_quantized 1 \
	  --use_weights best_weights \
	  mixednet \
	  --pointwise_filters "64,64,64,64" --repeat_in_block "1, 1, 1, 1" \
	  --mixconv_kernel_sizes '[5], [7,11], [9,15], [23]' \
	  --residual_connection "0,0,0,0" \
	  --first_conv_filters 32 --first_conv_kernel_size 5 --stride 3

manifest: need-work
	$(PY) scripts/manifest.py --work "$(WORK)" --max-faph $(MAX_FAPH)

install: need-work
	@# install, not cp: files on exFAT come as rwx------, with macOS attributes.
	install -m 644 "$(WORK)/out/hey_dj_ed.tflite" "$(WORK)/out/hey_dj_ed.json" $(MODELS)/
	@echo "Copied to $(MODELS). The firmware still embeds hey_jarvis until it is switched."

# exFAT and other foreign disks collect macOS's ._ files beside every file
# written; the training's file scans would read them as audio.
tidy: need-work
	find "$(WORK)" -name '._*' -type f -delete

clean-venv:
	rm -rf $(VENV) .src
