# Host tests for the hardware-independent half of the firmware: the sound
# and the event lines. The ESP-IDF glue needs a device.
CC      ?= gcc
CFLAGS  ?= -std=c11 -Wall -Wextra -Werror -O2 -g -D_DEFAULT_SOURCE
SRCDIR  := ../../main
INCLUDE := -I$(SRCDIR)/app/audio -I$(SRCDIR)/app/events
SOURCES := test_ears.c $(SRCDIR)/app/audio/pcm.c $(SRCDIR)/app/audio/endpoint.c $(SRCDIR)/app/events/event_format.c

.PHONY: test clean

test: run-tests
	./run-tests

run-tests: $(SOURCES) $(SRCDIR)/app/audio/pcm.h $(SRCDIR)/app/audio/endpoint.h $(SRCDIR)/app/events/event_format.h
	$(CC) $(CFLAGS) $(INCLUDE) -o $@ $(SOURCES) -lm

clean:
	rm -rf run-tests run-tests.dSYM
