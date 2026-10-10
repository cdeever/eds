# Host tests for the hardware-independent half of the firmware.
#
# Scene validation, effects, gamma and event formatting are pure C, so the parts where a bug is
# subtle and shows up only as "the light looks wrong" can be tested with a
# compiler and no hardware. The ESP-IDF glue needs a device.
CC      ?= gcc
CFLAGS  ?= -std=c11 -Wall -Wextra -Werror -O2 -g
SRCDIR  := ../../main
INCLUDE := -I$(SRCDIR)/app/scene -I$(SRCDIR)/app/effects -I$(SRCDIR)/app/events \
           -I$(SRCDIR)/hardware
SOURCES := test_lp_stand.c \
           $(SRCDIR)/app/scene/scene.c \
           $(SRCDIR)/app/effects/effects.c \
           $(SRCDIR)/app/events/event_format.c \
           $(SRCDIR)/hardware/gamma.c

.PHONY: test clean

test: run-tests
	./run-tests

run-tests: $(SOURCES)
	$(CC) $(CFLAGS) $(INCLUDE) -o $@ $(SOURCES) -lm

clean:
	rm -f run-tests
