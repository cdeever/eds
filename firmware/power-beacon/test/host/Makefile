# Host tests for the hardware-independent half of the firmware.
#
# The event lines are pure C, so the one part of the beacon that can be wrong
# without the board being wrong is tested with a compiler and no hardware.
# The ESP-IDF glue needs a device.
CC      ?= gcc
CFLAGS  ?= -std=c11 -Wall -Wextra -Werror -O2 -g
SRCDIR  := ../../main
INCLUDE := -I$(SRCDIR)/app/events
SOURCES := test_power_beacon.c \
           $(SRCDIR)/app/events/event_format.c

.PHONY: test clean

test: run-tests
	./run-tests

run-tests: $(SOURCES) $(SRCDIR)/app/events/event_format.h
	$(CC) $(CFLAGS) $(INCLUDE) -o $@ $(SOURCES)

clean:
	rm -rf run-tests run-tests.dSYM
