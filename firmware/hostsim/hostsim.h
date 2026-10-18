// Host simulation of the core firmware: the state the mock Arduino headers share with hostsim.cpp.
#ifndef HOSTSIM_H
#define HOSTSIM_H

#include <stdint.h>

extern uint32_t hs_now_ms;        // virtual clock, advanced by the driver (and by slow EEPROM writes)
extern bool hs_button_down;       // the simulated button
extern bool hs_pin_set;           // is the update PIN page (0x7780) written?
extern bool hs_unlocked;          // development device (lock bits not final)?
extern uint8_t hs_eeprom[1024];

void hs_advance(uint32_t ms);     // time passes inside a blocking firmware call
void hs_snapshot();               // record the display if it changed

void hs_display_tile(uint8_t x, uint8_t y, const uint8_t *tile);
void hs_display_contrast(uint8_t value);
void hs_display_power(bool on);
void hs_display_invert(bool on);
void hs_display_flip(bool on);

void hs_serial_out(char c);
bool hs_serial_avail();
char hs_serial_in();

#endif
