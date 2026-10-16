# Host tests for the hardware-independent half of the firmware.
#
# What a touch means, what is drawn over the cover, where a touch is and the
# event lines are pure C, so the parts where a bug shows up only as "it did
# the wrong thing when I touched it" can be tested with a compiler and no
# hardware. The ESP-IDF glue needs a device.
#
# `make pictures` draws the screens to BMP files, with a made-up cover, so the
# look of the buttons can be judged without flashing anything.
CC      ?= gcc
CFLAGS  ?= -std=c11 -Wall -Wextra -Werror -O2 -g
SRCDIR  := ../../main
INCLUDE := -I$(SRCDIR)/app/ui -I$(SRCDIR)/app/events -I$(SRCDIR)/app/ota
SOURCES := test_cover_screen.c \
           $(SRCDIR)/app/ui/ui.c \
           $(SRCDIR)/app/ui/draw.c \
           $(SRCDIR)/app/ui/touch_map.c \
           $(SRCDIR)/app/events/event_format.c \
           $(SRCDIR)/app/ota/ota_proto.c
PICTURES ?= pictures

.PHONY: test pictures clean

test: run-tests
	./run-tests

pictures: run-tests
	mkdir -p $(PICTURES) && ./run-tests --pictures $(PICTURES)

run-tests: $(SOURCES) $(SRCDIR)/app/ui/ui.h $(SRCDIR)/app/ui/draw.h $(SRCDIR)/app/ui/touch_map.h
	$(CC) $(CFLAGS) $(INCLUDE) -o $@ $(SOURCES) -lm

clean:
	rm -rf run-tests run-tests.dSYM $(PICTURES)
