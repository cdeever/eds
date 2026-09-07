// Gamma correction for the LED strip.
//
// This is the piece lightd deliberately does not do. Gamma is a property of
// the specific driver and diffuser, so it belongs next to the hardware it
// corrects - and keeping it off the wire means `mosquitto_sub` shows the
// colours actually intended rather than pre-distorted ones.
//
// WS2812/SK6812 PWM is linear in duty cycle while perception is not, so an
// uncorrected fade looks like it rushes through the dark end and then stalls.
#pragma once

#include <stdint.h>

// lp_gamma8 maps a linear 0..255 level to the driver value that looks like it.
uint8_t lp_gamma8(uint8_t value);
